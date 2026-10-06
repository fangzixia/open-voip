package media

import (
	"testing"
	"time"
)

func TestRecordPromptToMixAddsAudio(t *testing.T) {
	started := time.Now()
	rec := &recorder{
		started: started,
		pcm:     newPCMMix(8000, started),
	}
	frame := mulawToneFrame()
	recordPromptToMix(rec, frame)
	if rec.pcm == nil || len(rec.pcm.samples) < len(frame) {
		t.Fatalf("no samples mixed into recording")
	}
	peak := 0
	for _, s := range rec.pcm.samples[:len(frame)] {
		v := int(s)
		if v < 0 {
			v = -v
		}
		if v > peak {
			peak = v
		}
	}
	if peak < 200 {
		t.Fatalf("mixed prompt too quiet: peak=%d", peak)
	}
}

func TestPlayToneToRoomMixesIntoRecorder(t *testing.T) {
	callID := "rec-tone"
	started := time.Now()
	rec := &recorder{started: started, pcm: newPCMMix(8000, started)}
	r := &room{
		rec:    rec,
		peers:  map[string]*peer{},
		sipRTP: map[*sipRTP]struct{}{},
	}
	s := &Service{rooms: map[string]*room{callID: r}}
	seq := r.promptSeq.Add(1)
	s.playToneToRoom(callID, "", 45*time.Millisecond, seq, false)
	peak := 0
	for _, sample := range rec.pcm.samples {
		v := int(sample)
		if v < 0 {
			v = -v
		}
		if v > peak {
			peak = v
		}
	}
	if peak < 200 {
		t.Fatalf("playToneToRoom did not mix into recorder: peak=%d", peak)
	}
}
