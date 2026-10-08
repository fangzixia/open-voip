package ports

import (
	"context"
	"errors"
)

var ErrAudioBackpressure = errors.New("audio output queue full")
var ErrStaleGeneration = errors.New("stale output generation")

// PCMFormat describes mono, little-endian signed 16-bit application audio.
type PCMFormat struct {
	SampleRate int `json:"sample_rate"`
}

type MediaStreamOptions struct {
	Input     PCMFormat `json:"input"`     // Switch -> application
	Output    PCMFormat `json:"output"`    // application -> Switch
	Direction string    `json:"direction"` // duplex | sendonly | recvonly
}

type MediaOutputEvent struct {
	Type       string `json:"type"`
	Generation uint64 `json:"generation"`
}

// ApplicationStream owns bounded audio queues. Close never hangs up a call.
type ApplicationStream interface {
	Input() <-chan []byte
	Events() <-chan MediaOutputEvent
	Done() <-chan struct{}
	Err() error
	Write(uint64, []byte) error
	Clear(uint64) error
	Finish(uint64) error
	Close()
}

type PlaybackStatus struct {
	ID      string `json:"playback_id"`
	LegID   string `json:"leg_id"`
	State   string `json:"state"` // playing | finished | stopped | failed
	AssetID string `json:"asset_id"`
	Error   string `json:"error,omitempty"`
}

// ApplicationMediaPort is technical execution, independent of business type.
type ApplicationMediaPort interface {
	OpenApplicationStream(context.Context, string, string, MediaStreamOptions) (ApplicationStream, error)
	StartPlayback(context.Context, string, string, string, string, func(string)) error
	StopPlayback(context.Context, string, string, string) error
}

type ApplicationControlPort interface {
	OpenApplicationStream(context.Context, string, string, MediaStreamOptions) (ApplicationStream, error)
	GetLegPlayback(context.Context, string, string, string) (PlaybackStatus, error)
}
