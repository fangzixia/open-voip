package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"open-call/internal/errs"
	"open-call/internal/httpapi"
	"open-call/internal/integration/switchapi"
	"open-call/internal/ports/dto"
)

// serveSwitchBFF 将已鉴权、已校验的 BFF 请求转为 switchapi 服务端调用（非任意路径反向代理）。
func serveSwitchBFF(w http.ResponseWriter, r *http.Request, client *switchapi.Client) error {
	path := r.URL.Path
	callID := switchCallID(path)

	switch {
	case path == "/switch/v1/calls":
		items, err := client.ListOpenCalls(r.Context())
		if err != nil {
			return err
		}
		httpapi.Write(w, http.StatusOK, map[string]any{"items": items})
		return nil
	case path == "/switch/v1/calls/outbound":
		var req dto.OutboundRequest
		if err := decodeBFFBody(r, &req); err != nil {
			return err
		}
		id, err := client.Outbound(r.Context(), req)
		if err != nil {
			return err
		}
		view, err := client.GetCall(r.Context(), id)
		if err != nil {
			return err
		}
		httpapi.Write(w, http.StatusOK, view)
		return nil
	case path == "/switch/v1/calls/voice-notifications":
		var req dto.VoiceNotificationRequest
		if err := decodeBFFBody(r, &req); err != nil {
			return err
		}
		id, err := client.VoiceNotification(r.Context(), req)
		if err != nil {
			return err
		}
		view, err := client.GetCall(r.Context(), id)
		if err != nil {
			return err
		}
		httpapi.Write(w, http.StatusCreated, view)
		return nil
	case callID != "" && path == "/switch/v1/calls/"+callID && r.Method == http.MethodGet:
		view, err := client.GetCall(r.Context(), callID)
		if err != nil {
			return err
		}
		httpapi.Write(w, http.StatusOK, view)
		return nil
	case callID != "" && (path == "/switch/v1/calls/"+callID+"/bridges" || path == "/switch/v1/calls/"+callID+"/bridge"):
		var body struct {
			LegIDs []string `json:"leg_ids"`
			LegA   string   `json:"leg_a"`
			LegB   string   `json:"leg_b"`
		}
		if err := decodeBFFBody(r, &body); err != nil {
			return err
		}
		a, b := body.LegA, body.LegB
		if len(body.LegIDs) >= 2 {
			a, b = body.LegIDs[0], body.LegIDs[1]
		}
		if err := client.BridgeCall(r.Context(), callID, a, b); err != nil {
			return err
		}
		httpapi.Write(w, http.StatusOK, nil)
		return nil
	case callID != "" && strings.HasSuffix(path, "/hangup"):
		var body struct {
			Reason string `json:"reason"`
		}
		_ = decodeBFFBody(r, &body)
		reason := dto.HangupReasonNormal
		if body.Reason != "" {
			reason = dto.HangupReason(body.Reason)
		}
		if err := client.Hangup(r.Context(), callID, reason); err != nil {
			return err
		}
		httpapi.Write(w, http.StatusOK, nil)
		return nil
	case callID != "" && strings.HasSuffix(path, "/answer") && !strings.Contains(path, "/legs/"):
		var body struct {
			AgentID string `json:"agent_id"`
		}
		if err := decodeBFFBody(r, &body); err != nil {
			return err
		}
		if err := client.Answer(r.Context(), callID, body.AgentID); err != nil {
			return err
		}
		view, err := client.GetCall(r.Context(), callID)
		if err != nil {
			return err
		}
		httpapi.Write(w, http.StatusOK, view)
		return nil
	case callID != "" && strings.HasSuffix(path, "/decline"):
		var body struct {
			AgentID string `json:"agent_id"`
		}
		if err := decodeBFFBody(r, &body); err != nil {
			return err
		}
		if err := client.Decline(r.Context(), callID, body.AgentID); err != nil {
			return err
		}
		httpapi.Write(w, http.StatusOK, nil)
		return nil
	case callID != "" && strings.HasSuffix(path, "/hold"):
		var body struct {
			On bool `json:"on"`
		}
		if err := decodeBFFBody(r, &body); err != nil {
			return err
		}
		if err := client.Hold(r.Context(), callID, body.On); err != nil {
			return err
		}
		httpapi.Write(w, http.StatusOK, nil)
		return nil
	case callID != "" && strings.HasSuffix(path, "/transfer"):
		var req dto.TransferRequest
		if err := decodeBFFBody(r, &req); err != nil {
			return err
		}
		if err := client.Transfer(r.Context(), callID, req); err != nil {
			return err
		}
		httpapi.Write(w, http.StatusOK, nil)
		return nil
	case callID != "" && strings.HasSuffix(path, "/transfer/complete"):
		if err := client.CompleteTransfer(r.Context(), callID); err != nil {
			return err
		}
		httpapi.Write(w, http.StatusOK, nil)
		return nil
	case callID != "" && strings.HasSuffix(path, "/video/request"):
		var body struct {
			FromLegID string `json:"from_leg_id"`
		}
		if err := decodeBFFBody(r, &body); err != nil {
			return err
		}
		if err := client.RequestVideo(r.Context(), callID, body.FromLegID); err != nil {
			return err
		}
		httpapi.Write(w, http.StatusOK, nil)
		return nil
	case callID != "" && strings.HasSuffix(path, "/video/respond"):
		var body struct {
			Accept bool `json:"accept"`
		}
		if err := decodeBFFBody(r, &body); err != nil {
			return err
		}
		if err := client.RespondVideo(r.Context(), callID, body.Accept); err != nil {
			return err
		}
		httpapi.Write(w, http.StatusOK, nil)
		return nil
	case callID != "" && strings.HasSuffix(path, "/video/downgrade"):
		if err := client.DowngradeVideo(r.Context(), callID); err != nil {
			return err
		}
		httpapi.Write(w, http.StatusOK, nil)
		return nil
	case callID != "" && strings.HasSuffix(path, "/screen-share"):
		body, err := readCommandBody(r)
		if err != nil {
			return err
		}
		on, _ := body["on"].(bool)
		legID, _ := body["leg_id"].(string)
		if err := client.ScreenShare(r.Context(), callID, legID, on); err != nil {
			return err
		}
		httpapi.Write(w, http.StatusOK, nil)
		return nil
	case callID != "" && strings.HasSuffix(path, "/conference"):
		var body struct {
			AgentID string `json:"agent_id"`
		}
		if err := decodeBFFBody(r, &body); err != nil {
			return err
		}
		if err := client.ConferenceInvite(r.Context(), callID, body.AgentID); err != nil {
			return err
		}
		httpapi.Write(w, http.StatusOK, nil)
		return nil
	case callID != "" && strings.HasSuffix(path, "/dtmf"):
		body, err := readCommandBody(r)
		if err != nil {
			return err
		}
		legID, _ := body["leg_id"].(string)
		digit, _ := body["digit"].(string)
		if err := client.SendDTMF(r.Context(), callID, legID, digit); err != nil {
			return err
		}
		httpapi.Write(w, http.StatusOK, nil)
		return nil
	case callID != "" && strings.HasSuffix(path, "/turn-credentials"):
		subject := r.URL.Query().Get("subject")
		cfg, err := client.TURNCredentials(r.Context(), callID, subject)
		if err != nil {
			return err
		}
		httpapi.Write(w, http.StatusOK, cfg)
		return nil
	}

	if callID != "" && strings.Contains(path, "/legs/") {
		parts := strings.Split(strings.TrimPrefix(path, "/switch/v1/calls/"), "/")
		if len(parts) >= 3 && parts[1] == "legs" {
			legID := parts[2]
			switch parts[3] {
			case "offer":
				out, err := client.JoinWebRTCOffer(r.Context(), callID, legID)
				if err != nil {
					return err
				}
				httpapi.Write(w, http.StatusOK, out)
				return nil
			case "answer":
				var body struct {
					SDP  string `json:"sdp"`
					Type string `json:"type"`
				}
				if err := decodeBFFBody(r, &body); err != nil {
					return err
				}
				if err := client.AcceptLegAnswer(r.Context(), callID, legID, body.SDP, body.Type); err != nil {
					return err
				}
				httpapi.Write(w, http.StatusOK, map[string]string{"ok": "true"})
				return nil
			case "ice":
				body, err := readCommandBody(r)
				if err != nil {
					return err
				}
				if err := client.TrickleLegICE(r.Context(), callID, legID, body); err != nil {
					return err
				}
				httpapi.Write(w, http.StatusOK, nil)
				return nil
			case "mute":
				body, err := readCommandBody(r)
				if err != nil {
					return err
				}
				if err := client.MuteLeg(r.Context(), callID, legID, body); err != nil {
					return err
				}
				httpapi.Write(w, http.StatusOK, nil)
				return nil
			case "hold":
				var body struct {
					On bool `json:"on"`
				}
				if err := decodeBFFBody(r, &body); err != nil {
					return err
				}
				if err := client.HoldLeg(r.Context(), callID, legID, body.On); err != nil {
					return err
				}
				httpapi.Write(w, http.StatusOK, nil)
				return nil
			case "reject":
				var body struct {
					Reason string `json:"reason"`
				}
				_ = decodeBFFBody(r, &body)
				if err := client.RejectLeg(r.Context(), callID, legID, body.Reason); err != nil {
					return err
				}
				httpapi.Write(w, http.StatusOK, nil)
				return nil
			case "playbacks":
				if r.Method != http.MethodPost {
					return errs.InvalidRequest("方法不允许")
				}
				var body struct {
					AssetID string `json:"asset_id"`
				}
				if err := decodeBFFBody(r, &body); err != nil {
					return err
				}
				id, err := client.StartLegPlayback(r.Context(), callID, legID, body.AssetID)
				if err != nil {
					return err
				}
				httpapi.Write(w, http.StatusCreated, map[string]string{"playback_id": id})
				return nil
			}
			if len(parts) >= 5 && parts[3] == "playbacks" && r.Method == http.MethodDelete {
				playbackID := parts[4]
				if err := client.StopLegPlayback(r.Context(), callID, legID, playbackID); err != nil {
					return err
				}
				httpapi.Write(w, http.StatusOK, nil)
				return nil
			}
		}
	}

	if callID != "" && strings.Contains(path, "/bridges/") && r.Method == http.MethodPut {
		bridgeID := strings.Split(strings.TrimPrefix(path, "/switch/v1/calls/"+callID+"/bridges/"), "/")[0]
		var body struct {
			LegIDs []string `json:"leg_ids"`
			LegA   string   `json:"leg_a"`
			LegB   string   `json:"leg_b"`
		}
		if err := decodeBFFBody(r, &body); err != nil {
			return err
		}
		a, b := body.LegA, body.LegB
		if len(body.LegIDs) >= 2 {
			a, b = body.LegIDs[0], body.LegIDs[1]
		}
		if err := client.ReplaceBridge(r.Context(), callID, bridgeID, a, b); err != nil {
			return err
		}
		httpapi.Write(w, http.StatusOK, nil)
		return nil
	}
	if callID != "" && strings.Contains(path, "/bridges/") && r.Method == http.MethodDelete {
		bridgeID := strings.Split(strings.TrimPrefix(path, "/switch/v1/calls/"+callID+"/bridges/"), "/")[0]
		if err := client.EndBridge(r.Context(), callID, bridgeID); err != nil {
			return err
		}
		httpapi.Write(w, http.StatusOK, nil)
		return nil
	}

	if strings.HasPrefix(path, "/switch/v1/supervisor/calls/") && strings.HasSuffix(path, "/listen") {
		var body struct {
			AgentID string `json:"agent_id"`
		}
		if err := decodeBFFBody(r, &body); err != nil {
			return err
		}
		legID, err := client.SupervisorListen(r.Context(), callID, body.AgentID)
		if err != nil {
			return err
		}
		httpapi.Write(w, http.StatusOK, map[string]string{"leg_id": legID})
		return nil
	}
	if strings.HasPrefix(path, "/switch/v1/supervisor/agents/") && strings.HasSuffix(path, "/force-check-out") {
		agentID := strings.TrimSuffix(strings.TrimPrefix(path, "/switch/v1/supervisor/agents/"), "/force-check-out")
		var body struct {
			Policy string `json:"policy"`
		}
		_ = decodeBFFBody(r, &body)
		if err := client.ForceReleaseAgent(r.Context(), agentID, body.Policy); err != nil {
			return err
		}
		httpapi.Write(w, http.StatusOK, nil)
		return nil
	}

	return errs.InvalidRequest("不支持的 Switch 操作")
}

func decodeBFFBody(r *http.Request, out any) error {
	if r.Body == nil {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, (64<<10)+1))
	_ = r.Body.Close()
	if err != nil || len(raw) > 64<<10 {
		return errs.InvalidRequest("请求体过大")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if json.Unmarshal(raw, out) != nil {
		return errs.InvalidRequest("JSON 无法解析")
	}
	return nil
}
