// 本文件负责录音文件读取与下载。
package http

import (
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http"
	"open-switch/internal/errs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (d SwitchRouterDeps) handleRecordingFile(w http.ResponseWriter, r *http.Request) {
	callID, id := chi.URLParam(r, "callId"), chi.URLParam(r, "recordingId")
	if _, err := uuid.Parse(callID); err != nil {
		writeErr(w, errs.InvalidRequest("call_id 无效"))
		return
	}
	if _, err := uuid.Parse(id); err != nil {
		writeErr(w, errs.InvalidRequest("recording_id 无效"))
		return
	}
	ext := r.URL.Query().Get("ext")
	switch ext {
	case ".wav", ".ogg", ".webm", ".mp4", ".ivf":
	default:
		writeErr(w, errs.InvalidRequest("录音格式无效"))
		return
	}
	root, err := os.OpenRoot(d.Config.Recordings.Dir)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer root.Close()
	name := callID + "-" + id + ext
	if r.Method == http.MethodDelete {
		for _, suffix := range []string{".wav", ".ogg", ".webm", ".mp4", ".ivf"} {
			if err := root.Remove(callID + "-" + id + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
				writeErr(w, err)
				return
			}
		}
		writeJSON(w, http.StatusOK, nil)
		return
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format != "" && format != "webm" && format != "mp4" {
		writeErr(w, errs.InvalidRequest("录像格式仅支持 webm 或 mp4"))
		return
	}
	if format != "" && ext != ".webm" && ext != ".mp4" {
		writeErr(w, errs.InvalidRequest("此录音不是可转换的视频文件"))
		return
	}
	if format != "" && "."+format != ext {
		tempDir, err := os.MkdirTemp(d.Config.Recordings.Dir, ".recording-download-")
		if err != nil {
			writeErr(w, err)
			return
		}
		converted := filepath.Join(tempDir, id+"."+format)
		defer func() {
			_ = os.Remove(converted)
			_ = os.Remove(tempDir)
		}()
		if err := convertRecording(r, d.Config.Recordings.FFmpegPath, filepath.Join(d.Config.Recordings.Dir, name), converted, format); err != nil {
			writeErr(w, err)
			return
		}
		file, err := os.Open(converted)
		if err != nil {
			writeErr(w, err)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			writeErr(w, err)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+id+"."+format+`"`)
		w.Header().Set("Content-Type", "video/"+format)
		http.ServeContent(w, r, id+"."+format, info.ModTime(), file)
		return
	}
	file, err := root.Open(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeErr(w, errs.NotFound("录音文件不存在"))
		} else {
			writeErr(w, err)
		}
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+id+ext+`"`)
	http.ServeContent(w, r, name, info.ModTime(), file)
}

func convertRecording(r *http.Request, ffmpegPath, input, output, format string) error {
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	bin, err := exec.LookPath(ffmpegPath)
	if err != nil {
		return fmt.Errorf("视频格式转换需要 FFmpeg: %w", err)
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", input, "-map", "0:v:0", "-map", "0:a:0"}
	if format == "mp4" {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart")
	} else {
		args = append(args, "-c:v", "libvpx", "-deadline", "good", "-cpu-used", "4", "-b:v", "1M", "-c:a", "libopus", "-b:a", "96k")
	}
	outputBytes, err := exec.CommandContext(r.Context(), bin, append(args, output)...).CombinedOutput()
	if err != nil {
		message := string(outputBytes)
		if len(message) > 1000 {
			message = message[len(message)-1000:]
		}
		return fmt.Errorf("录像格式转换失败: %w: %s", err, message)
	}
	return nil
}
