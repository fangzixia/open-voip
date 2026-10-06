package aibot

import (
	"context"
	"log/slog"
	"time"

	"open-call/internal/ports/dto"
)

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
	peer, err := sig.Connect(ctx, callID, agentLeg, mediaAgentID)
	if err != nil {
		return err
	}
	defer peer.Close()
	if err := sig.api.BridgeCall(ctx, callID, agentLeg, pstnLeg); err != nil {
		return err
	}
	wav, err := sig.api.DownloadIVRAsset(ctx, assetRef)
	if err != nil {
		return err
	}
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	dur, err := PlayWAVToPeer(sctx, peer, wav)
	if err != nil {
		return err
	}
	if log != nil {
		log.Info("语音通知播放完成", "call_id", callID, "duration", dur)
	}
	time.Sleep(dur + 600*time.Millisecond)
	return sig.api.Hangup(ctx, callID, dto.HangupReasonNormal)
}
