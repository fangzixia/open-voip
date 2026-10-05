package http

import (
	"net/http"
	"strings"

	"open-call/internal/observability"
)

func isIVRAssetPath(path string) bool {
	if path == "/api/v1/ivr-assets/tts-options" || path == "/api/v1/ivr-assets/synthesize" {
		return false
	}
	return path == "/api/v1/ivr-assets" || strings.HasPrefix(path, "/api/v1/ivr-assets/")
}

func switchPermission(method, path string) string {
	if strings.HasPrefix(path, "/switch/v1/ivr-assets") {
		if method == http.MethodGet {
			return "ivr.read"
		}
		return "ivr.write"
	}
	if strings.Contains(path, "/supervisor/agents/") {
		return "agents.force_checkout"
	}
	if strings.Contains(path, "/supervisor/calls/") {
		return "calls.listen"
	}
	if method == http.MethodGet {
		return "calls.read"
	}
	return "calls.operate"
}

func switchCallID(path string) string {
	for _, prefix := range []string{"/switch/v1/calls/", "/switch/v1/supervisor/calls/"} {
		if strings.HasPrefix(path, prefix) {
			id := strings.Split(strings.TrimPrefix(path, prefix), "/")[0]
			if id == "outbound" || id == "inbound" {
				return ""
			}
			return observability.NormalizeID(id)
		}
	}
	return ""
}

func mapSwitchPath(path string) string {
	switch {
	case path == "/api/v1/calls/outbound":
		return "/switch/v1/calls/outbound"
	case path == "/api/v1/calls":
		return "/switch/v1/calls"
	case strings.HasPrefix(path, "/api/v1/calls/"):
		return strings.Replace(path, "/api/v1/calls/", "/switch/v1/calls/", 1)
	case strings.HasPrefix(path, "/api/v1/supervisor/calls/"):
		return strings.Replace(path, "/api/v1/supervisor/calls/", "/switch/v1/supervisor/calls/", 1)
	case strings.HasPrefix(path, "/api/v1/supervisor/agents/"):
		return strings.Replace(path, "/api/v1/supervisor/agents/", "/switch/v1/supervisor/agents/", 1)
	default:
		return path
	}
}

func shouldProxyToSwitch(path string) bool {
	if isIVRAssetPath(path) {
		return false
	}
	if path == "/api/v1/calls/outbound" || path == "/api/v1/calls" {
		return true
	}
	if strings.HasPrefix(path, "/api/v1/calls/") {
		parts := strings.Split(strings.TrimPrefix(path, "/api/v1/calls/"), "/")
		if len(parts) == 1 && parts[0] != "" && parts[0] != "inbound" && parts[0] != "outbound" {
			return true
		}
		suffix := strings.Join(parts[1:], "/")
		switch suffix {
		case "answer", "decline", "hangup", "hold", "transfer", "transfer/complete", "video/request", "video/respond", "video/downgrade", "screen-share", "conference", "dtmf", "turn-credentials", "bridges", "bridge":
			return true
		}
		if len(parts) >= 4 && parts[1] == "legs" {
			switch parts[3] {
			case "offer", "answer", "ice", "mute", "hold", "reject", "playbacks":
				return true
			}
			if len(parts) >= 5 && parts[3] == "playbacks" {
				return true
			}
		}
		if len(parts) >= 3 && parts[1] == "bridges" {
			return true
		}
		return false
	}
	if strings.HasPrefix(path, "/api/v1/supervisor/calls/") {
		parts := strings.Split(strings.TrimPrefix(path, "/api/v1/supervisor/calls/"), "/")
		return len(parts) == 2 && parts[1] == "listen"
	}
	if strings.HasPrefix(path, "/api/v1/supervisor/agents/") {
		parts := strings.Split(strings.TrimPrefix(path, "/api/v1/supervisor/agents/"), "/")
		return len(parts) == 2 && parts[1] == "force-check-out"
	}
	return false
}
