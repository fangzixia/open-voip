package http

import (
	"encoding/json"
	"net/http"

	"gorm.io/gorm"
)

// StatusProvider 提供运行状态摘要字段。
type StatusProvider struct {
	// DB 数据库连接，可为 nil（仅 health 场景）。
	DB *gorm.DB
	// ActiveCalls 活跃通话数。
	ActiveCalls func() int
	// WSConnections WebSocket 连接数。
	WSConnections func() int
}

type statusResponse struct {
	DbOK           bool `json:"db_ok"`
	ActiveCalls    int  `json:"active_calls"`
	WSConnections  int  `json:"ws_connections"`
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (s StatusProvider) handleStatus(w http.ResponseWriter, r *http.Request) {
	resp := statusResponse{
		ActiveCalls:   0,
		WSConnections: 0,
	}
	if s.ActiveCalls != nil {
		resp.ActiveCalls = s.ActiveCalls()
	}
	if s.WSConnections != nil {
		resp.WSConnections = s.WSConnections()
	}
	if s.DB != nil {
		sqlDB, err := s.DB.DB()
		if err == nil {
			resp.DbOK = sqlDB.Ping() == nil
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
