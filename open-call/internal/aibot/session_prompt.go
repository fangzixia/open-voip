package aibot

import (
	"context"
	"log/slog"
	"time"

	"open-call/internal/ports/dto"
)

// promptRTPFlush 播完后继续送静音的时长，让末帧走完 WebRTC→SIP，并避免 RTP 空窗。
// 不得再叠加素材时长：PlayWAVToPeer 已按实时时钟播完。
const (
	promptRTPFlush      = 600 * time.Millisecond
	promptHangupTimeout = 8 * time.Second
)

func hangupPrompt(api MediaAPI, callID string, reason dto.HangupReason) error {
	hctx, cancel := context.WithTimeout(context.Background(), promptHangupTimeout)
	defer cancel()
	return api.Hangup(hctx, callID, reason)
}

// RunPromptOutbound 语音通知：JoinWebRTC、桥接 PSTN、播放素材并挂断。
func RunPromptOutbound(ctx context.Context, sig *Signaling, callID, mediaAgentID, assetRef string, log *slog.Logger) error {
	view, err := sig.api.GetCall(ctx, callID)
	if err != nil {
		return err
	}
	agentLeg, ok := findAgentLeg(view, mediaAgentID)
	if !ok {
		agentLeg, ok = findAgentLeg(view, "")
	}
	if !ok {
		return errNoAgentLeg
	}
	pstnLeg, ok := findPSTNLeg(view)
	if !ok {
		return errNoPSTNLeg
	}
	peer, err := sig.Connect(ctx, callID, agentLeg, mediaAgentID, false)
	if err != nil {
		return err
	}
	defer peer.Close()
	waitCtx, waitCancel := context.WithTimeout(ctx, 15*time.Second)
	defer waitCancel()
	if err := peer.WaitConnected(waitCtx); err != nil {
		_ = hangupPrompt(sig.api, callID, dto.HangupReasonError)
		return err
	}
	if err := bridgeCallWithRetry(ctx, sig.api, callID, agentLeg, pstnLeg); err != nil {
		_ = hangupPrompt(sig.api, callID, dto.HangupReasonError)
		return err
	}
	wav, err := sig.api.DownloadIVRAsset(ctx, assetRef)
	if err != nil {
		_ = hangupPrompt(sig.api, callID, dto.HangupReasonError)
		return err
	}
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	dur, err := PlayWAVToPeer(sctx, peer, wav)
	if err != nil {
		_ = hangupPrompt(sig.api, callID, dto.HangupReasonError)
		return err
	}
	if log != nil {
		log.Info("语音通知播放完成", "call_id", callID, "duration", dur)
	}
	flushCtx, flushCancel := context.WithTimeout(ctx, promptRTPFlush+100*time.Millisecond)
	_ = PlaySilenceToPeer(flushCtx, peer, promptRTPFlush)
	flushCancel()
	return hangupPrompt(sig.api, callID, dto.HangupReasonNormal)
}

func bridgeCallWithRetry(ctx context.Context, api MediaAPI, callID, legA, legB string) error {
	var last error
	for attempt := 0; attempt < 40; attempt++ {
		err := api.BridgeCall(ctx, callID, legA, legB)
		if err == nil {
			return nil
		}
		last = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return last
}
