package control

import (
	"context"
	"encoding/json"
	"time"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
)

// 业务 error_code，与 docs/call-events.md 对齐。
const (
	codeIVRHopLimit      = "IVR_HOP_LIMIT"
	codeIVRPromptFailed  = "IVR_PROMPT_FAILED"
	codeIVRActionFailed  = "IVR_ACTION_FAILED"
	codeRouteQueueFailed = "ROUTE_QUEUE_FAILED"
	codeCallState        = "CALL_STATE_ERROR"
	codeCallSuperseded   = "CALL_SUPERSEDED"
	codeSwitchRecover    = "SWITCH_RECOVER"
	codeSIPDevice        = "SIP_DEVICE_UNAVAILABLE"
)

func (s *Service) applyFailureDetail(callID string, err error) {
	if err == nil {
		return
	}
	if api := errs.AsAPIError(err); api != nil {
		s.setCallEndDetail(callID, api.Message, api.Code)
		return
	}
	s.setCallEndDetail(callID, err.Error(), "")
}

func (s *Service) failCall(ctx context.Context, callID string, reason dto.HangupReason, code, message string) error {
	if message != "" || code != "" {
		s.setCallEndDetail(callID, message, code)
	} else if reason == dto.HangupReasonError {
		s.setCallEndDetail(callID, defaultEndMessage(reason), codeCallState)
	}
	return s.Hangup(ctx, callID, reason)
}

func (s *Service) hangupWithFailure(ctx context.Context, callID string, reason dto.HangupReason, err error) error {
	s.applyFailureDetail(callID, err)
	s.ensureEndMessage(callID, reason)
	return s.Hangup(ctx, callID, reason)
}

func (s *Service) ensureEndMessage(callID string, reason dto.HangupReason) {
	s.mu.Lock()
	rt := s.calls[callID]
	if rt == nil || rt.endMessage != "" {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	if reason == dto.HangupReasonError {
		s.setCallEndDetail(callID, defaultEndMessage(reason), codeCallState)
	}
}

func defaultEndMessage(reason dto.HangupReason) string {
	switch reason {
	case dto.HangupReasonError:
		return "通话异常结束"
	case dto.HangupReasonTimeout:
		return "等待超时"
	case dto.HangupReasonAbandon:
		return "呼叫已放弃"
	default:
		return ""
	}
}

func failureFromErr(err error) (message, code string) {
	if err == nil {
		return "", ""
	}
	if api := errs.AsAPIError(err); api != nil {
		return api.Message, api.Code
	}
	return err.Error(), ""
}

func (s *Service) setPstnDialState(callID, state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rt := s.calls[callID]; rt != nil {
		rt.pstnDialState = state
	}
}

func (s *Service) emitCommandFailed(ctx context.Context, callID, code, message, reason string) {
	payload := map[string]any{"call_id": callID, "message": message, "error_code": code}
	if reason != "" {
		payload["reason"] = reason
	}
	s.emitCall(ctx, callID, "command.failed", "", payload)
}

func (s *Service) emitOutboundProgress(ctx context.Context, callID, agentID, phase, message string) {
	payload := map[string]any{"call_id": callID, "phase": phase}
	if message != "" {
		payload["message"] = message
	}
	_ = s.publishCall(ctx, callID, "call.outbound_progress", agentID, payload)
}

func mergeCallEndIntoMetadata(rec *ports.CallRecord, message, code, result string) {
	if message == "" && code == "" && result == "" {
		return
	}
	meta := map[string]any{}
	if rec.Metadata != "" && rec.Metadata != "{}" {
		_ = json.Unmarshal([]byte(rec.Metadata), &meta)
	}
	if message != "" {
		meta["end_message"] = message
	}
	if code != "" {
		meta["error_code"] = code
	}
	if result != "" {
		meta["cdr_result"] = result
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return
	}
	rec.Metadata = string(raw)
}

func endDetailFromMetadata(rec ports.CallRecord) (message, code, result string) {
	if rec.Metadata == "" || rec.Metadata == "{}" {
		return "", "", ""
	}
	var meta map[string]any
	if json.Unmarshal([]byte(rec.Metadata), &meta) != nil {
		return "", "", ""
	}
	if s, ok := meta["end_message"].(string); ok {
		message = s
	}
	if s, ok := meta["error_code"].(string); ok {
		code = s
	}
	if s, ok := meta["cdr_result"].(string); ok {
		result = s
	}
	return message, code, result
}

func enrichCallView(v *ports.CallView, rt *runtimeCall) {
	if rt == nil {
		return
	}
	v.EndMessage = rt.endMessage
	v.ErrorCode = rt.endCode
	v.PstnDialState = rt.pstnDialState
	if rt.promptOutbound {
		v.OutboundMode = "prompt_outbound"
	}
	v.Result = rt.cdrResult
	if v.EndMessage == "" || v.ErrorCode == "" || v.Result == "" {
		msg, code, res := endDetailFromMetadata(rt.rec)
		if v.EndMessage == "" {
			v.EndMessage = msg
		}
		if v.ErrorCode == "" {
			v.ErrorCode = code
		}
		if v.Result == "" {
			v.Result = res
		}
	}
	if v.Result == "" {
		v.Result = inferResult(rt.rec.State, rt.answeredAt)
	}
}

func inferResult(state string, answeredAt *time.Time) string {
	if state != stateEnded {
		return ""
	}
	if answeredAt != nil {
		return "answered"
	}
	return "failed"
}
