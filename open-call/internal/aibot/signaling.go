package aibot

import (
	"context"
	"fmt"

	"github.com/pion/webrtc/v4"

	"open-call/internal/integration/switchapi"
	"open-call/internal/ports"
	"open-call/internal/ports/dto"
)

// MediaAPI 虚拟坐席访问 open-switch 的呼叫与 WebRTC 信令。
type MediaAPI interface {
	JoinWebRTCOffer(ctx context.Context, callID, legID string) (map[string]string, error) // 获取 SFU Offer SDP
	AcceptLegAnswer(ctx context.Context, callID, legID, sdp, typ string) error            // 提交本端 Answer
	TrickleLegICE(ctx context.Context, callID, legID string, body map[string]any) error
	TURNCredentials(ctx context.Context, callID, subject string) (dto.TURNConfig, error)
	BridgeCall(ctx context.Context, callID, legA, legB string) error
	GetCall(ctx context.Context, callID string) (ports.CallView, error)
	Hangup(ctx context.Context, callID string, reason dto.HangupReason) error
	Answer(ctx context.Context, callID, agentID string) error // 坐席应答
	CheckIn(ctx context.Context, agentID string, queueIDs []string) error
	DownloadIVRAsset(ctx context.Context, assetRef string) ([]byte, error)
}

// Signaling 封装 WebRTC 与 Switch 的信令交互。
type Signaling struct {
	api MediaAPI
}

func NewSignaling(c *switchapi.Client) *Signaling {
	return &Signaling{api: switchMediaAdapter{client: c}}
}

type switchMediaAdapter struct {
	client *switchapi.Client
}

func (a switchMediaAdapter) JoinWebRTCOffer(ctx context.Context, callID, legID string) (map[string]string, error) {
	return a.client.JoinWebRTCOffer(ctx, callID, legID)
}

func (a switchMediaAdapter) AcceptLegAnswer(ctx context.Context, callID, legID, sdp, typ string) error {
	return a.client.AcceptLegAnswer(ctx, callID, legID, sdp, typ)
}

func (a switchMediaAdapter) TrickleLegICE(ctx context.Context, callID, legID string, body map[string]any) error {
	return a.client.TrickleLegICE(ctx, callID, legID, body)
}

func (a switchMediaAdapter) TURNCredentials(ctx context.Context, callID, subject string) (dto.TURNConfig, error) {
	return a.client.TURNCredentials(ctx, callID, subject)
}

func (a switchMediaAdapter) BridgeCall(ctx context.Context, callID, legA, legB string) error {
	return a.client.BridgeCall(ctx, callID, legA, legB)
}

func (a switchMediaAdapter) GetCall(ctx context.Context, callID string) (ports.CallView, error) {
	return a.client.GetCall(ctx, callID)
}

func (a switchMediaAdapter) Hangup(ctx context.Context, callID string, reason dto.HangupReason) error {
	return a.client.Hangup(ctx, callID, reason)
}

func (a switchMediaAdapter) Answer(ctx context.Context, callID, agentID string) error {
	return a.client.Answer(ctx, callID, agentID)
}

func (a switchMediaAdapter) CheckIn(ctx context.Context, agentID string, queueIDs []string) error {
	_, err := a.client.CheckIn(ctx, agentID, queueIDs)
	return err
}

func (a switchMediaAdapter) DownloadIVRAsset(ctx context.Context, assetRef string) ([]byte, error) {
	return a.client.DownloadIVRAsset(ctx, assetRef)
}

// Connect 拉取 TURN、完成 Switch Offer/Answer 与 ICE 串通，返回可收发媒体的 PeerSession。
func (s *Signaling) Connect(ctx context.Context, callID, legID, subject string, widebandWebRTC bool) (*PeerSession, error) {
	turn, err := s.api.TURNCredentials(ctx, callID, subject)
	if err != nil {
		return nil, err
	}
	offerMap, err := s.api.JoinWebRTCOffer(ctx, callID, legID)
	if err != nil {
		return nil, err
	}
	offerSDP := offerMap["sdp"]
	if offerSDP == "" {
		return nil, fmt.Errorf("switch 未返回 offer sdp")
	}
	peer, err := newPeerSession(turn, widebandWebRTC, func(cand *webrtc.ICECandidate) {
		if cand == nil {
			return
		}
		init := cand.ToJSON()
		body := map[string]any{
			"candidate":     init.Candidate,
			"sdp_mid":       init.SDPMid,
			"sdp_mline_index": init.SDPMLineIndex,
		}
		_ = s.api.TrickleLegICE(context.Background(), callID, legID, body)
	})
	if err != nil {
		return nil, err
	}
	if err := peer.CompleteOffer(offerSDP); err != nil {
		peer.Close()
		return nil, err
	}
	answer := peer.LocalDescription()
	if answer == nil {
		peer.Close()
		return nil, fmt.Errorf("本地 answer 为空")
	}
	if err := s.api.AcceptLegAnswer(ctx, callID, legID, answer.SDP, answer.Type.String()); err != nil {
		peer.Close()
		return nil, err
	}
	return peer, nil
}

func findAgentLeg(view ports.CallView, agentID string) (string, bool) {
	for _, leg := range view.Legs {
		if leg.Role == dto.LegRoleAgent && (agentID == "" || leg.AgentID == agentID) {
			return leg.ID, true
		}
	}
	return "", false
}

func findPSTNLeg(view ports.CallView) (string, bool) {
	for _, leg := range view.Legs {
		if leg.Role == dto.LegRolePSTN {
			return leg.ID, true
		}
	}
	return "", false
}
