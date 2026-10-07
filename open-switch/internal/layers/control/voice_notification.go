package control

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	"open-switch/internal/errs"
	"open-switch/internal/ports/dto"
)

func normalizePromptAsset(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if !strings.HasSuffix(strings.ToLower(ref), ".wav") {
		if _, err := uuid.Parse(ref); err == nil {
			ref += ".wav"
		}
	}
	return ref
}

func promptOutboundMetadata(agentID, asset string) string {
	raw, _ := json.Marshal(map[string]any{
		"outbound_mode":      "prompt_outbound",
		"prompt_asset_id":    asset,
		"initiator_agent_id": agentID,
	})
	return string(raw)
}

// VoiceNotification 坐席触发单通 PSTN 语音通知。
func (s *Service) VoiceNotification(ctx context.Context, req dto.VoiceNotificationRequest) (string, error) {
	if req.AgentID == "" || req.Destination == "" {
		return "", errs.InvalidRequest("agent_id 与 destination 必填")
	}
	asset := normalizePromptAsset(req.PromptAssetID)
	if asset == "" {
		return "", errs.InvalidRequest("prompt_asset_id 必填")
	}
	if !looksPSTN(req.Destination) {
		return "", errs.InvalidRequest("语音通知仅支持 PSTN 号码")
	}
	if _, err := s.deps.Media.PromptDuration(ctx, asset); err != nil {
		return "", err
	}
	info, err := s.deps.Agents.ByID(ctx, req.AgentID)
	if err != nil {
		return "", err
	}
	if info.TerminalType == "sip" {
		return "", errs.Conflict("请从 SIP 话机拨号", "")
	}
	return s.doOutboundWithID(ctx, dto.OutboundRequest{
		AgentID:       req.AgentID,
		Destination:   req.Destination,
		TrunkID:       req.TrunkID,
		PromptAssetID: asset,
	}, uuid.New().String())
}
