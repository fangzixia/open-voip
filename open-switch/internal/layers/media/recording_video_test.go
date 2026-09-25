package media

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVideoRecordingProducesPlayableWebMAndMP4(t *testing.T) {
	ffmpeg := os.Getenv("FFMPEG_TEST_BIN")
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	bin, err := exec.LookPath(ffmpeg)
	if err != nil {
		t.Skip("FFmpeg is not installed")
	}
	probe := filepath.Join(filepath.Dir(bin), "ffprobe")
	if strings.HasSuffix(strings.ToLower(bin), ".exe") {
		probe += ".exe"
	}
	if _, err := os.Stat(probe); err != nil {
		t.Skip("ffprobe is not installed")
	}

	for _, format := range []string{"webm", "mp4"} {
		t.Run(format, func(t *testing.T) {
			dir := t.TempDir()
			parts := filepath.Join(dir, "parts")
			if err := os.Mkdir(parts, 0o700); err != nil {
				t.Fatal(err)
			}
			started := time.Now().UTC().Add(-time.Second)
			v := &videoRecording{
				started: started, outputPath: filepath.Join(dir, "call."+format),
				workDir: parts, format: format, ffmpeg: bin, tracks: make(map[string]*recordedTrack),
			}
			for i := 0; i < 2; i++ {
				video := filepath.Join(parts, "video"+string(rune('0'+i))+".ivf")
				audio := filepath.Join(parts, "audio"+string(rune('0'+i))+".ogg")
				makeMediaFixture(t, bin, video, "-f", "lavfi", "-i", "testsrc=size=320x240:rate=10:duration=1", "-c:v", "libvpx", "-pix_fmt", "yuv420p", "-f", "ivf")
				makeMediaFixture(t, bin, audio, "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:a", "libopus", "-f", "ogg")
				when := started.Add(time.Duration(i*100) * time.Millisecond)
				v.tracks["v"+string(rune('0'+i))] = &recordedTrack{legID: string(rune('0' + i)), kind: "video", path: video, started: when, packets: 10}
				v.tracks["a"+string(rune('0'+i))] = &recordedTrack{legID: string(rune('0' + i)), kind: "audio", path: audio, started: when, packets: 50}
			}
			out, size, err := v.finish(time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if out != v.outputPath || size == 0 {
				t.Fatalf("output=%q size=%d", out, size)
			}
			streams, err := exec.Command(probe, "-v", "error", "-show_entries", "stream=codec_type", "-of", "csv=p=0", out).Output()
			if err != nil || !strings.Contains(string(streams), "video") || !strings.Contains(string(streams), "audio") {
				t.Fatalf("invalid recording: streams=%q err=%v", streams, err)
			}
		})
	}
}

func makeMediaFixture(t *testing.T, bin, output string, args ...string) {
	t.Helper()
	command := append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...)
	command = append(command, output)
	if result, err := exec.Command(bin, command...).CombinedOutput(); err != nil {
		t.Fatalf("FFmpeg fixture failed: %v: %s", err, result)
	}
}
