// 本文件负责视频录制流程。
package media

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media/ivfwriter"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
)

// 每个 RTP 来源使用独立写入器；将不同 SSRC 合并到同一个 OGG
// 或 IVF 写入器会扰乱时间戳，导致视频无法解码。
type recordedTrack struct {
	legID   string
	kind    string
	path    string
	started time.Time
	packets int
	ogg     *oggwriter.OggWriter
	ivf     *ivfwriter.IVFWriter
}

type videoRecording struct {
	started    time.Time
	outputPath string
	workDir    string
	format     string
	ffmpeg     string
	tracks     map[string]*recordedTrack
	writeErr   error
}

func newVideoRecording(root, callID, recordingID, format, ffmpegPath string, started time.Time) (*videoRecording, error) {
	if format != "webm" && format != "mp4" {
		return nil, fmt.Errorf("不支持的视频录像格式 %q", format)
	}
	bin, err := exec.LookPath(ffmpegPath)
	if err != nil {
		return nil, fmt.Errorf("视频录像需要 FFmpeg: %w", err)
	}
	base := callID + "-" + recordingID
	workDir, err := os.MkdirTemp(root, "."+base+"-parts-")
	if err != nil {
		return nil, err
	}
	return &videoRecording{
		started: started, outputPath: filepath.Join(root, base+"."+format),
		workDir: workDir, format: format, ffmpeg: bin, tracks: make(map[string]*recordedTrack),
	}, nil
}

func (v *videoRecording) writeRTP(legID string, kind webrtc.RTPCodecType, mime string, pkt *rtp.Packet) error {
	if v.writeErr != nil || len(pkt.Payload) == 0 {
		return v.writeErr
	}
	trackKind, ext := "", ""
	switch {
	case kind == webrtc.RTPCodecTypeAudio && strings.EqualFold(mime, webrtc.MimeTypeOpus):
		trackKind, ext = "audio", ".ogg"
	case kind == webrtc.RTPCodecTypeVideo && strings.EqualFold(mime, webrtc.MimeTypeVP8):
		trackKind, ext = "video", ".ivf"
	default:
		return nil
	}
	key := trackKind + ":" + legID + ":" + strconv.FormatUint(uint64(pkt.SSRC), 10)
	track := v.tracks[key]
	if track == nil {
		path := filepath.Join(v.workDir, fmt.Sprintf("%s-%d%s", trackKind, len(v.tracks), ext))
		track = &recordedTrack{legID: legID, kind: trackKind, path: path, started: time.Now().UTC()}
		var err error
		if trackKind == "audio" {
			track.ogg, err = oggwriter.New(path, 48000, 2)
		} else {
			track.ivf, err = ivfwriter.New(path)
		}
		if err != nil {
			return err
		}
		v.tracks[key] = track
	}
	var err error
	if track.ogg != nil {
		err = track.ogg.WriteRTP(pkt)
	} else {
		err = track.ivf.WriteRTP(pkt)
	}
	if err == nil {
		track.packets++
	}
	return err
}

func (v *videoRecording) finish(duration time.Duration) (string, int64, error) {
	var videos, audios []*recordedTrack
	for _, track := range v.tracks {
		var err error
		if track.ogg != nil {
			err = track.ogg.Close()
		} else if track.ivf != nil {
			err = track.ivf.Close()
		}
		if err != nil && v.writeErr == nil {
			v.writeErr = err
		}
		if track.packets == 0 {
			continue
		}
		if track.kind == "audio" {
			audios = append(audios, track)
		} else {
			videos = append(videos, track)
		}
	}
	if v.writeErr != nil {
		return "", 0, fmt.Errorf("录像轨道写入失败: %w", v.writeErr)
	}
	if len(audios) == 0 || len(videos) == 0 {
		return "", 0, fmt.Errorf("录像不完整：音频轨 %d，视频轨 %d；原始轨道保留在 %s", len(audios), len(videos), v.workDir)
	}
	sortTracks(videos)
	sortTracks(audios)
	partial := v.outputPath + ".partial"
	args := v.ffmpegArgs(videos, audios, duration, partial)
	timeout := duration + 2*time.Minute
	if timeout < 2*time.Minute {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, v.ffmpeg, args...).CombinedOutput()
	if err != nil {
		_ = os.Remove(partial)
		message := string(output)
		if len(message) > 2000 {
			message = message[len(message)-2000:]
		}
		return "", 0, fmt.Errorf("合成录像失败: %w: %s；原始轨道保留在 %s", err, message, v.workDir)
	}
	st, err := os.Stat(partial)
	if err != nil || st.Size() == 0 {
		return "", 0, fmt.Errorf("合成录像为空或不存在: %v", err)
	}
	if err := os.Rename(partial, v.outputPath); err != nil {
		return "", 0, err
	}
	for _, track := range v.tracks {
		_ = os.Remove(track.path)
	}
	_ = os.Remove(v.workDir)
	return v.outputPath, st.Size(), nil
}

func sortTracks(tracks []*recordedTrack) {
	sort.Slice(tracks, func(i, j int) bool {
		if tracks[i].started.Equal(tracks[j].started) {
			return tracks[i].legID < tracks[j].legID
		}
		return tracks[i].started.Before(tracks[j].started)
	})
}

func (v *videoRecording) ffmpegArgs(videos, audios []*recordedTrack, duration time.Duration, output string) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y"}
	inputs := append(append([]*recordedTrack{}, videos...), audios...)
	for _, track := range inputs {
		offset := track.started.Sub(v.started).Seconds()
		if offset < 0 {
			offset = 0
		}
		args = append(args, "-itsoffset", fmt.Sprintf("%.3f", offset), "-i", track.path)
	}
	var filters []string
	videoMap := "0:v:0"
	if len(videos) > 1 {
		current := "[0:v]"
		for i := 1; i < len(videos); i++ {
			pip, next := fmt.Sprintf("[pip%d]", i), fmt.Sprintf("[v%d]", i)
			filters = append(filters, fmt.Sprintf("[%d:v]scale=iw/3:ih/3%s", i, pip))
			filters = append(filters, fmt.Sprintf("%s%soverlay=W-w-16:H-h-16:eof_action=pass:repeatlast=0%s", current, pip, next))
			current = next
		}
		videoMap = current
	}
	audioMap := fmt.Sprintf("%d:a:0", len(videos))
	if len(audios) > 1 {
		var sources strings.Builder
		for i := range audios {
			fmt.Fprintf(&sources, "[%d:a]", len(videos)+i)
		}
		filters = append(filters, fmt.Sprintf("%samix=inputs=%d:duration=longest:dropout_transition=0:normalize=1[aout]", sources.String(), len(audios)))
		audioMap = "[aout]"
	}
	if len(filters) > 0 {
		args = append(args, "-filter_complex", strings.Join(filters, ";"))
	}
	args = append(args, "-map", videoMap, "-map", audioMap)
	if v.format == "mp4" {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-pix_fmt", "yuv420p",
			"-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart")
	} else {
		if len(videos) > 1 {
			args = append(args, "-c:v", "libvpx", "-deadline", "good", "-cpu-used", "4", "-b:v", "1M")
		} else {
			args = append(args, "-c:v", "copy")
		}
		if len(audios) > 1 {
			args = append(args, "-c:a", "libopus", "-b:a", "96k")
		} else {
			args = append(args, "-c:a", "copy")
		}
	}
	if duration > 0 {
		args = append(args, "-t", fmt.Sprintf("%.3f", duration.Seconds()))
	}
	args = append(args, "-f", v.format, output)
	return args
}
