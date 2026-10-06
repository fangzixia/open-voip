package app

import (
	"encoding/json"
	"testing"
)

// 契约：call.ended 载荷字段与 docs/call-events.md 一致。
func TestCallEndedEventPayloadShape(t *testing.T) {
	raw := map[string]any{
		"type": "call.ended",
		"payload": map[string]any{
			"call_id":    "c-1",
			"reason":     "error",
			"result":     "failed",
			"message":    "SIP 网关模组未注册",
			"error_code": "SIP_DISABLED",
		},
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	p, _ := decoded["payload"].(map[string]any)
	for _, key := range []string{"call_id", "reason", "result", "message", "error_code"} {
		if p[key] == nil || p[key] == "" {
			t.Fatalf("missing payload.%s", key)
		}
	}
}
