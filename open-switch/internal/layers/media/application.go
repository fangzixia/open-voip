package media

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
)

const maxOutputSamples = mixInternalRate / 5

// audioOutput is consumed only by the room's existing 20 ms mixer clock.
// Generation changes invalidate both buffered and subsequently arriving old audio.
type audioOutput struct {
	mu             sync.Mutex
	pcm            []int16
	generation     uint64
	finished       bool
	completed      bool
	drainAt        time.Time
	resampler      *streamingResampler
	rate           int
	pendingSamples float64 // accepted samples retained inside libsoxr
}

func (o *audioOutput) write(g uint64, pcm []int16) error {
	return o.writeRate(g, pcm, mixInternalRate)
}

func (o *audioOutput) writeRate(g uint64, pcm []int16, rate int) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if g != o.generation {
		return ports.ErrStaleGeneration
	}
	if o.finished {
		return errors.New("output already finished")
	}
	if rate <= 0 {
		return errors.New("invalid sample rate")
	}
	expected := float64(len(pcm)) * mixInternalRate / float64(rate)
	if float64(len(o.pcm))+o.pendingSamples+expected > maxOutputSamples {
		return ports.ErrAudioBackpressure
	}
	if o.rate != 0 && o.rate != rate {
		return errors.New("sample rate changed inside generation")
	}
	o.rate = rate
	converted := pcm
	if rate != mixInternalRate {
		if o.resampler == nil {
			r, e := newStreamingResampler(rate, mixInternalRate, false)
			if e != nil {
				return e
			}
			o.resampler = r
		}
		var e error
		converted, e = o.resampler.process(pcm, false)
		if e != nil {
			return e
		}
	}
	o.pcm = append(o.pcm, converted...)
	o.pendingSamples = max(0, o.pendingSamples+expected-float64(len(converted)))
	return nil
}

func (o *audioOutput) clear(g uint64) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if g <= o.generation {
		return errors.New("generation must increase")
	}
	o.generation, o.pcm, o.finished, o.completed, o.drainAt = g, nil, false, false, time.Time{}
	o.resampler.close()
	o.resampler = nil
	o.rate = 0
	o.pendingSamples = 0
	return nil
}

func (o *audioOutput) finish(g uint64) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if g != o.generation {
		return errors.New("stale output generation")
	}
	o.finished = true
	if o.resampler != nil {
		tail, e := o.resampler.process(nil, true)
		o.resampler.close()
		o.resampler = nil
		if e != nil {
			return e
		}
		o.pcm = append(o.pcm, tail...)
		o.pendingSamples = 0
		if len(o.pcm) > maxOutputSamples {
			return errors.New("resampler tail exceeded output limit")
		}
	}
	return nil
}

func (o *audioOutput) pull(now time.Time) ([]int16, uint64, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	var frame []int16
	if len(o.pcm) >= mixFrameSamples || (o.finished && len(o.pcm) > 0) {
		n := min(len(o.pcm), mixFrameSamples)
		frame = make([]int16, mixFrameSamples)
		copy(frame, o.pcm[:n])
		o.pcm = o.pcm[n:]
	}
	if o.finished && len(o.pcm) == 0 && !o.completed {
		if o.drainAt.IsZero() {
			o.drainAt = now.Add(200 * time.Millisecond)
		}
		if !now.Before(o.drainAt) {
			o.completed = true
			return frame, o.generation, true
		}
	}
	return frame, o.generation, false
}

type applicationStream struct {
	inputConverter pcmConverter
	output         audioOutput
	opts           ports.MediaStreamOptions
	input          chan []byte
	events         chan ports.MediaOutputEvent
	done           chan struct{}
	once           sync.Once
	errMu          sync.Mutex
	err            error
	held           bool // guarded by room.mu
	muted          bool
}

func (a *applicationStream) Input() <-chan []byte                  { return a.input }
func (a *applicationStream) Events() <-chan ports.MediaOutputEvent { return a.events }
func (a *applicationStream) Done() <-chan struct{}                 { return a.done }
func (a *applicationStream) Err() error                            { a.errMu.Lock(); defer a.errMu.Unlock(); return a.err }
func (a *applicationStream) fail(err error) {
	a.once.Do(func() {
		a.errMu.Lock()
		a.err = err
		a.errMu.Unlock()
		close(a.done)
		a.inputConverter.close()
		a.output.mu.Lock()
		a.output.resampler.close()
		a.output.resampler = nil
		a.output.pcm = nil
		a.output.finished = true
		a.output.mu.Unlock()
	})
}
func (a *applicationStream) Close() { a.fail(nil) }
func (a *applicationStream) Write(g uint64, raw []byte) error {
	if a.opts.Direction == "recvonly" {
		return errors.New("receive-only media session")
	}
	select {
	case <-a.done:
		return errors.New("media session closed")
	default:
	}
	if len(raw) == 0 || len(raw)%2 != 0 || len(raw) > a.opts.Output.SampleRate*2 {
		return errors.New("PCM block must contain at most one second of whole PCM16 samples")
	}
	pcm := make([]int16, len(raw)/2)
	for i := range pcm {
		pcm[i] = int16(binary.LittleEndian.Uint16(raw[i*2:]))
	}
	return a.output.writeRate(g, pcm, a.opts.Output.SampleRate)
}
func (a *applicationStream) Clear(g uint64) error  { return a.output.clear(g) }
func (a *applicationStream) Finish(g uint64) error { return a.output.finish(g) }
func validRate(r int) bool                         { return r == 8000 || r == 16000 || r == 24000 || r == 48000 }

func (s *Service) OpenApplicationStream(ctx context.Context, callID, legID string, opts ports.MediaStreamOptions) (ports.ApplicationStream, error) {
	if opts.Direction != "duplex" && opts.Direction != "sendonly" && opts.Direction != "recvonly" {
		return nil, errs.InvalidRequest("direction 必须为 duplex、sendonly 或 recvonly")
	}
	if !validRate(opts.Input.SampleRate) || !validRate(opts.Output.SampleRate) {
		return nil, errs.InvalidRequest("PCM 采样率支持 8000、16000、24000、48000；格式固定为单声道 PCM16LE")
	}
	r := s.getRoom(callID)
	if r == nil {
		return nil, errs.NotFound("媒体房间不存在")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, errs.NotFound("媒体房间已关闭")
	}
	if !r.mixAudio || r.mixer == nil {
		return nil, errs.Conflict("应用音频需要混音房间，请在建立直接桥接前接入", "")
	}
	if r.peers[legID] != nil {
		return nil, errs.Conflict("该腿已接入 WebRTC", "")
	}
	if r.streams == nil {
		r.streams = map[string]*applicationStream{}
	}
	if old := r.streams[legID]; old != nil {
		select {
		case <-old.done:
		default:
			return nil, errs.Conflict("该腿已有媒体会话", "")
		}
	}
	a := &applicationStream{opts: opts, input: make(chan []byte, 100), events: make(chan ports.MediaOutputEvent, 16), done: make(chan struct{})}
	a.output.generation = 1
	r.streams[legID] = a
	return a, nil
}

type assetPlayback struct {
	validateTarget bool
	legID          string
	output         audioOutput
	done           func(string)
	once           sync.Once
}

func (p *assetPlayback) complete(state string) {
	p.once.Do(func() {
		if p.done != nil {
			go p.done(state)
		}
	})
}

func (s *Service) StartPlayback(ctx context.Context, callID, legID, id, asset string, done func(string)) error {
	path := s.resolvePrompt(ctx, asset)
	if path == "" {
		return errs.InvalidRequest("素材 ID 无效")
	}
	pcm, rate, err := readPCMWav(path)
	if err != nil {
		return errs.InvalidRequest("素材不存在或 WAV 无效")
	}
	pcm, err = resamplePCM(pcm, rate, mixInternalRate)
	if err != nil {
		return fmt.Errorf("prepare playback: %w", err)
	}
	r := s.getRoom(callID)
	if r == nil {
		return errs.NotFound("媒体房间不存在")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return errs.NotFound("媒体房间已关闭")
	}
	if r.mixer == nil {
		r.mixer = newScheduledRoomMixer()
		go s.runRoomMixLoop(callID, r, r.mixer)
	}
	if !r.hasPlaybackTarget(legID) {
		return errs.Conflict("播放目标媒体尚未连接", "")
	}
	if r.playbacks == nil {
		r.playbacks = map[string]*assetPlayback{}
	}
	if r.playbacks[id] != nil {
		return nil
	}
	// A target has one audio producer; stopping it cannot stop another leg or IVR.
	for _, p := range r.playbacks {
		if p.legID == legID {
			return errs.Conflict("该腿已有播放任务", "")
		}
	}
	p := &assetPlayback{legID: legID, done: done, validateTarget: true}
	p.output = audioOutput{pcm: pcm, generation: 1, finished: true}
	r.playbacks[id] = p
	return nil
}
func (s *Service) StopPlayback(ctx context.Context, callID, legID, id string) error {
	r := s.getRoom(callID)
	if r == nil {
		return errs.NotFound("媒体房间不存在")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if p := r.playbacks[id]; p != nil {
		if p.legID != legID {
			return errs.NotFound("播放不属于该腿")
		}
		delete(r.playbacks, id)
		p.complete("stopped")
	}
	return nil
}

func encodePCM16(pcm []int16) []byte {
	b := make([]byte, 2*len(pcm))
	for i, v := range pcm {
		binary.LittleEndian.PutUint16(b[2*i:], uint16(v))
	}
	return b
}

// applicationFrames is called under room.mu. Input excludes all local output,
// and each output frame enters the same codec/RTP/recording path as other legs.
func (r *room) applicationFrames(frames map[string][]int16, now time.Time) map[string]map[string][]int16 {
	for id, a := range r.streams {
		select {
		case <-a.done:
			delete(r.streams, id)
			continue
		default:
		}
		if a.held {
			continue
		}
		if a.opts.Direction != "sendonly" {
			allowed := map[string][]int16{}
			for from, pcm := range frames {
				if from != id && r.mediaForwardAllowed(from, id) {
					allowed[from] = pcm
				}
			}
			pcm := mixPCMFramesLimited(allowed, id)
			converted, e := a.inputConverter.convert(pcm, mixInternalRate, a.opts.Input.SampleRate)
			if e != nil {
				a.fail(e)
				continue
			}
			if len(converted) == 0 {
				continue
			}
			frameSize := a.opts.Input.SampleRate / 50
			for start := 0; start < len(converted); start += frameSize {
				select {
				case a.input <- encodePCM16(converted[start : start+frameSize]):
				default:
					a.fail(errors.New("application input stalled for 2 seconds"))
				}
			}
		}
	}
	for id, a := range r.streams {
		if a.held {
			continue
		}
		pcm, g, done := a.output.pull(now)
		if len(pcm) > 0 && !a.muted {
			frames[id] = pcm
		}
		if done {
			select {
			case a.events <- ports.MediaOutputEvent{Type: "output.finished", Generation: g}:
			default:
				a.fail(errors.New("media event consumer stalled"))
			}
		}
	}
	targeted := map[string]map[string][]int16{}
	for id, p := range r.playbacks {
		if p.validateTarget && !r.hasPlaybackTarget(p.legID) {
			delete(r.playbacks, id)
			p.complete("failed")
			continue
		}
		if r.playbackTargetHeld(p.legID) {
			continue
		}
		pcm, _, done := p.output.pull(now)
		if len(pcm) > 0 {
			if targeted[p.legID] == nil {
				targeted[p.legID] = map[string][]int16{}
			}
			targeted[p.legID]["asset:"+id] = pcm
		}
		if done {
			delete(r.playbacks, id)
			p.complete("finished")
		}
	}
	return targeted
}

func (r *room) hasPlaybackTarget(id string) bool {
	if p := r.peers[id]; p != nil && p.pc != nil && webrtcLegMediaReady(p.pc) {
		return true
	}
	for rt := range r.sipRTP {
		rt.mu.Lock()
		ready := rt.legID == id && rt.conn != nil && rt.remote != nil
		rt.mu.Unlock()
		if ready {
			return true
		}
	}
	return false
}

func (r *room) playbackTargetHeld(id string) bool {
	if p := r.peers[id]; p != nil && p.held {
		return true
	}
	for rt := range r.sipRTP {
		rt.mu.Lock()
		held := rt.legID == id && rt.held
		rt.mu.Unlock()
		if held {
			return true
		}
	}
	return false
}
