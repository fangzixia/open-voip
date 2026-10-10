package media

import (
	"errors"
	"github.com/go-audio/wav"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func readRecording(t *testing.T, path string) []int {
	t.Helper()
	f, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	d := wav.NewDecoder(f)
	b, e := d.FullPCMBuffer()
	if e != nil {
		t.Fatal(e)
	}
	if d.SampleRate != 8000 || d.NumChans != 1 || d.BitDepth != 16 {
		t.Fatalf("WAV format %d/%d/%d", d.SampleRate, d.NumChans, d.BitDepth)
	}
	return b.Data
}
func constantPCM(v int16) []int16 {
	p := make([]int16, 160)
	for i := range p {
		p[i] = v
	}
	return p
}

func TestConversationRecordingCompleteAlignedAndGated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "call.wav")
	c, e := newConversationRecording(path, nil)
	if e != nil {
		t.Fatal(e)
	}
	r := &recorder{conversation: c, gateInboundUntilPrompt: true}
	// The first tick records prompt only; the input track still occupies time.
	r.recordFrameGroup(map[string][]int16{"customer": constantPCM(1000)}, map[string][]int16{"p": constantPCM(3000)}, []string{"customer"})
	r.mu.Lock()
	r.gateInboundUntilPrompt = false
	r.mu.Unlock()
	r.recordFrameGroup(map[string][]int16{"customer": constantPCM(1000), "agent": constantPCM(2000)}, nil, []string{"customer", "agent"})
	r.recordFrameGroup(nil, nil, nil) // departed legs remain zero-filled to the end.
	if e = c.stop(); e != nil {
		t.Fatal(e)
	}
	main := readRecording(t, path)
	customer := readRecording(t, c.paths["customer"])
	agent := readRecording(t, c.paths["agent"])
	if len(main) != 480 || len(customer) != 480 || len(agent) != 480 {
		t.Fatal("recording timelines differ")
	}
	if main[0] != 2550 || main[160] != 1275 {
		t.Fatalf("main omitted/duplicated source: %d %d", main[0], main[160])
	}
	if customer[0] != 0 || customer[160] != 1000 || customer[320] != 0 || agent[0] != 0 || agent[160] != 2000 || agent[320] != 0 {
		t.Fatal("stem alignment or gate failed")
	}
	if c.samples != 480 || c.status != "completed" {
		t.Fatal("metadata duration/status")
	}
}

func TestRecordingQueueOverflowNeverExecutesInline(t *testing.T) {
	c := &conversationRecording{queue: make(chan recordingFrameGroup, 250), status: "recording"}
	start := time.Now()
	for range 251 {
		c.enqueue(recordingFrameGroup{main: constantPCM(1)})
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Fatal("media blocked on writer")
	}
	if len(c.queue) != 250 || c.status != "failed" || c.reason == "" {
		t.Fatal("overflow reported as success")
	}
}

type brokenWAVWriter struct{ *os.File }

func (brokenWAVWriter) Write([]byte) (int, error) { return 0, errors.New("disk full injected") }
func TestRecordingDiskFailureReportsPartialAndContinues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partial.wav")
	failed := make(chan string, 1)
	c, e := newConversationRecording(path, func(reason string) { failed <- reason })
	if e != nil {
		t.Fatal(e)
	}
	c.main.encoder = wav.NewEncoder(brokenWAVWriter{c.main.file}, 8000, 16, 1, 1)
	c.enqueue(recordingFrameGroup{main: constantPCM(1), legs: map[string][]int16{}})
	select {
	case reason := <-failed:
		if !strings.Contains(reason, "disk full") {
			t.Fatal(reason)
		}
	case <-time.After(time.Second):
		t.Fatal("failure not reported")
	}
	if e = c.stop(); e == nil {
		t.Fatal("disk failure was reported as completed")
	}
	if _, e = os.Stat(path); e != nil {
		t.Fatal("partial file deleted")
	}
}

type pausedWAVWriter struct {
	*os.File
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *pausedWAVWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.entered); <-w.release })
	return w.File.Write(p)
}

func TestRecordingSlowDiskStopHasFiveSecondDeadline(t *testing.T) {
	c, e := newConversationRecording(filepath.Join(t.TempDir(), "slow.wav"), nil)
	if e != nil {
		t.Fatal(e)
	}
	w := &pausedWAVWriter{File: c.main.file, entered: make(chan struct{}), release: make(chan struct{})}
	c.main.encoder = wav.NewEncoder(w, 8000, 16, 1, 1)
	c.enqueue(recordingFrameGroup{main: constantPCM(7)})
	select {
	case <-w.entered:
	case <-time.After(time.Second):
		t.Fatal("disk injection not reached")
	}
	start := time.Now()
	e = c.stop()
	elapsed := time.Since(start)
	close(w.release)
	select {
	case <-c.done:
	case <-time.After(time.Second):
		t.Fatal("writer did not release")
	}
	if e == nil || !strings.Contains(e.Error(), "timed out") || elapsed < 5*time.Second || elapsed > 6*time.Second {
		t.Fatalf("stop result %v after %s", e, elapsed)
	}
	if c.status != "failed" {
		t.Fatal("late disk completion erased failure")
	}
}

func TestRecordingFIFOOwnsTimeline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fifo.wav")
	c, e := newConversationRecording(path, nil)
	if e != nil {
		t.Fatal(e)
	}
	r := &recorder{conversation: c}
	for i := 1; i <= 100; i++ {
		pcm := constantPCM(int16(i * 100))
		r.recordFrameGroup(map[string][]int16{"customer": pcm}, nil, []string{"customer"})
		// Producer buffer reuse must not corrupt an already queued frame.
		clear(pcm)
	}
	if e = c.stop(); e != nil {
		t.Fatal(e)
	}
	main, stem := readRecording(t, path), readRecording(t, c.paths["customer"])
	if len(main) != 16000 || len(stem) != 16000 {
		t.Fatal("FIFO shortened timeline")
	}
	for i := range 100 {
		if main[i*160] != (i+1)*85 || stem[i*160] != (i+1)*100 {
			t.Fatalf("group %d reordered or reused", i)
		}
	}
}
