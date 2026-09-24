package http

import (
	"open-switch/internal/config"
	"open-switch/internal/ports"
)

// RouterDeps Switch API 处理器依赖（仅 L3 通话控制）。
type RouterDeps struct {
	Config      config.Config
	CallControl ports.CallControlPort
	Signaling   ports.SignalingPort
}
