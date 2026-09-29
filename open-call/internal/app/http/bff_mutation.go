package http

import (
	"net/http"
	"strings"

	"open-call/internal/integration/switchapi"
)

func attachSwitchMutation(r *http.Request) *http.Request {
	m := switchapi.Mutation{IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key"))}
	body, err := readCommandBody(r)
	if err == nil {
		m.ExpectedVersion = expectedVersionFromBody(body)
		_ = replaceCommandBody(r, body)
	}
	return r.WithContext(switchapi.WithMutation(r.Context(), m))
}

func expectedVersionFromBody(body map[string]any) int64 {
	if body == nil {
		return 0
	}
	v, ok := body["expected_version"]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}
