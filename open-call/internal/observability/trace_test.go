// 本文件验证请求链路追踪的关键行为。
package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRecorderCorrelatesAndSanitizes(t *testing.T) {
	var output bytes.Buffer
	recorder := NewRecorder(&output)
	ctx := With(context.Background(), Context{
		TraceID: "trace-1", RequestID: "request-1", CallID: "call-1",
		LegID: "leg-1", AgentID: "agent-1", QueueID: "queue-1", ClientSessionID: "client-1",
	})
	err := recorder.Write(ctx, "client.webrtc", map[string]any{
		"Authorization": "Bearer secret",
		"nested": map[string]any{
			"password": "unsafe",
			"sdp":      "v=0\r\na=ice-pwd:very-secret\r\n",
		},
		"message": "jwt eyJabcdefghijk.abcdefghijk.signature",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw := output.String()
	for _, secret := range []string{"very-secret", "unsafe", "Bearer secret", "eyJabcdefghijk"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("secret %q leaked: %s", secret, raw)
		}
	}
	var event Event
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if event.TraceID != "trace-1" || event.CallID != "call-1" || event.ClientSessionID != "client-1" {
		t.Fatalf("missing context: %+v", event)
	}
}

func TestNormalizeID(t *testing.T) {
	if NormalizeID(" ok-id_1 ") != "ok-id_1" {
		t.Fatal("valid ID rejected")
	}
	if NormalizeID("bad\r\nheader") != "" || NormalizeID(strings.Repeat("x", 129)) != "" {
		t.Fatal("unsafe ID accepted")
	}
}
