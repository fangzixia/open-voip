package media

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
)

type recordingFrameGroup struct {
	main []int16
	legs map[string][]int16
}
type recordingWAV struct {
	file    *os.File
	encoder *wav.Encoder
}

func openRecordingWAV(path string) (*recordingWAV, error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0640)
	if e != nil {
		return nil, e
	}
	return &recordingWAV{file: f, encoder: wav.NewEncoder(f, 8000, 16, 1, 1)}, nil
}
func (w *recordingWAV) write(pcm []int16) error {
	data := make([]int, len(pcm))
	for i, v := range pcm {
		data[i] = int(v)
	}
	return w.encoder.Write(&audio.IntBuffer{Format: &audio.Format{NumChannels: 1, SampleRate: 8000}, Data: data, SourceBitDepth: 16})
}
func (w *recordingWAV) close() error {
	e := w.encoder.Close()
	if err := w.file.Sync(); e == nil {
		e = err
	}
	if err := w.file.Close(); e == nil {
		e = err
	}
	return e
}

// Only the FIFO consumer touches encoders and files. The room clock submits
// one owned frame group per tick, never a per-leg or per-destination disk task.
type conversationRecording struct {
	mu             sync.Mutex
	queue          chan recordingFrameGroup
	done           chan struct{}
	closed         bool
	status, reason string
	samples        int64
	fileSize       int64
	writeErrors    uint64
	paths          map[string]string
	path           string
	main           *recordingWAV
	tracks         map[string]*recordingWAV
	onFailure      func(string)
}

func newConversationRecording(path string, onFailure func(string)) (*conversationRecording, error) {
	w, e := openRecordingWAV(path)
	if e != nil {
		return nil, e
	}
	c := &conversationRecording{queue: make(chan recordingFrameGroup, 250), done: make(chan struct{}), status: "recording", paths: map[string]string{}, tracks: map[string]*recordingWAV{}, path: path, main: w, onFailure: onFailure}
	go c.run()
	return c, nil
}
func (c *conversationRecording) fail(reason string) {
	c.mu.Lock()
	if strings.HasPrefix(reason, "create ") || strings.HasPrefix(reason, "align ") || strings.HasPrefix(reason, "write ") || strings.HasPrefix(reason, "close ") {
		c.writeErrors++
	}
	first := c.status != "failed"
	if first {
		c.status = "failed"
		c.reason = reason
	}
	f := c.onFailure
	c.mu.Unlock()
	if first && f != nil {
		go f(reason)
	}
}
func (c *conversationRecording) enqueue(group recordingFrameGroup) {
	c.mu.Lock()
	if c.closed || c.status == "failed" {
		c.mu.Unlock()
		return
	}
	select {
	case c.queue <- group:
		c.mu.Unlock()
	default:
		c.mu.Unlock()
		c.fail("recording queue exceeded 250 frame groups")
	}
}
func (c *conversationRecording) run() {
	defer close(c.done)
	var position int64
	for group := range c.queue {
		c.mu.Lock()
		failed := c.status == "failed"
		c.mu.Unlock()
		if failed {
			continue
		}
		for id := range group.legs {
			if c.tracks[id] != nil {
				continue
			}
			// Leg IDs are validated UUIDs by control; retaining a safe suffix
			// also protects file access for internal integration callers.
			path := strings.TrimSuffix(c.path, filepath.Ext(c.path)) + "-leg-" + sanitizeLegID(id) + ".wav"
			w, e := openRecordingWAV(path)
			if e != nil {
				c.fail(fmt.Sprintf("create leg WAV: %v", e))
				break
			}
			c.tracks[id] = w
			c.mu.Lock()
			c.paths[id] = path
			c.mu.Unlock()
			for n := int64(0); n < position; n += 8000 {
				if e = w.write(make([]int16, min(8000, position-n))); e != nil {
					c.fail(fmt.Sprintf("align leg WAV: %v", e))
					break
				}
			}
		}
		c.mu.Lock()
		failed = c.status == "failed"
		c.mu.Unlock()
		if failed {
			continue
		}
		if e := c.main.write(group.main); e != nil {
			c.fail(fmt.Sprintf("write main WAV: %v", e))
			continue
		}
		for id, w := range c.tracks {
			pcm := group.legs[id]
			if len(pcm) == 0 {
				pcm = make([]int16, mixFrameSamples)
			}
			if e := w.write(pcm); e != nil {
				c.fail(fmt.Sprintf("write leg WAV: %v", e))
				break
			}
		}
		position += int64(len(group.main))
		c.mu.Lock()
		c.samples = position
		c.mu.Unlock()
	}
	if e := c.main.close(); e != nil {
		c.fail(fmt.Sprintf("close main WAV: %v", e))
	}
	// Even metadata disk access belongs to the consumer, so Stop retains its
	// five-second deadline when the filesystem itself is stalled.
	if info, e := os.Stat(c.path); e == nil {
		c.mu.Lock()
		c.fileSize = info.Size()
		c.mu.Unlock()
	}
	for _, w := range c.tracks {
		if e := w.close(); e != nil {
			c.fail(fmt.Sprintf("close leg WAV: %v", e))
		}
	}
	c.mu.Lock()
	if c.status != "failed" {
		c.status = "completed"
	}
	c.mu.Unlock()
}
func (c *conversationRecording) stop() error {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		close(c.queue)
	}
	c.mu.Unlock()
	t := time.NewTimer(5 * time.Second)
	defer t.Stop()
	select {
	case <-c.done:
	case <-t.C:
		c.fail("recording stop timed out after 5 seconds")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.status == "failed" {
		return errors.New(c.reason)
	}
	return nil
}

func (rec *recorder) recordFrameGroup(frames, prompts map[string][]int16, legIDs []string) {
	if rec == nil || rec.conversation == nil {
		return
	}
	rec.mu.Lock()
	gated := rec.gateInboundUntilPrompt
	rec.mu.Unlock()
	group := recordingFrameGroup{legs: map[string][]int16{}}
	sources := map[string][]int16{}
	for _, id := range legIDs {
		group.legs[id] = make([]int16, mixFrameSamples)
	}
	for id, pcm := range frames {
		if !gated {
			group.legs[id] = make([]int16, mixFrameSamples)
			copy(group.legs[id], pcm)
			sources[id] = pcm
		}
	}
	for id, pcm := range prompts {
		sources["prompt:"+id] = pcm
	}
	group.main = mixPCMFramesLimited(sources, "")
	rec.conversation.enqueue(group)
}
