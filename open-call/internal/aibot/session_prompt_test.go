package aibot

import (
	"context"
	"testing"
	"time"

	"open-call/internal/ports"
	"open-call/internal/ports/dto"
)

type hangupProbeAPI struct {
	ctxErr error
	reason dto.HangupReason
}

func (h *hangupProbeAPI) JoinWebRTCOffer(context.Context, string, string) (map[string]string, error) {
	return nil, nil
}
func (h *hangupProbeAPI) AcceptLegAnswer(context.Context, string, string, string, string) error {
	return nil
}
func (h *hangupProbeAPI) TrickleLegICE(context.Context, string, string, map[string]any) error {
	return nil
}
func (h *hangupProbeAPI) TURNCredentials(context.Context, string, string) (dto.TURNConfig, error) {
	return dto.TURNConfig{}, nil
}
func (h *hangupProbeAPI) BridgeCall(context.Context, string, string, string) error { return nil }
func (h *hangupProbeAPI) GetCall(context.Context, string) (ports.CallView, error) {
	return ports.CallView{}, nil
}
func (h *hangupProbeAPI) Hangup(ctx context.Context, _ string, reason dto.HangupReason) error {
	h.ctxErr = ctx.Err()
	h.reason = reason
	return nil
}
func (h *hangupProbeAPI) Answer(context.Context, string, string) error { return nil }
func (h *hangupProbeAPI) CheckIn(context.Context, string, []string) error {
	return nil
}
func (h *hangupProbeAPI) DownloadIVRAsset(context.Context, string) ([]byte, error) {
	return nil, nil
}

func TestHangupPromptIgnoresCallerCancel(t *testing.T) {
	api := &hangupProbeAPI{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if ctx.Err() == nil {
		t.Fatal("setup: caller ctx should be cancelled")
	}
	if err := hangupPrompt(api, "c1", dto.HangupReasonNormal); err != nil {
		t.Fatal(err)
	}
	if api.ctxErr != nil {
		t.Fatalf("BYE 必须用独立超时上下文，否则事件 ctx 取消会导致挂不断: %v", api.ctxErr)
	}
	if api.reason != dto.HangupReasonNormal {
		t.Fatalf("reason=%s", api.reason)
	}
}

func TestPromptRTPFlushDoesNotRepeatPlayback(t *testing.T) {
	if promptRTPFlush >= time.Second {
		t.Fatalf("播完后只应保留尾帧余量，不能再等一整段素材（曾导致手机晚挂断 + 后续 486），got %s", promptRTPFlush)
	}
}
