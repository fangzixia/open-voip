package http

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"path/filepath"
	"runtime"

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
	// RecordingsDir 录音目录，供运维判断磁盘挂载。
	RecordingsDir string
}

type statusResponse struct {
	DbOK            bool   `json:"db_ok"`
	ActiveCalls     int    `json:"active_calls"`
	WSConnections   int    `json:"ws_connections"`
	Goroutines      int    `json:"goroutines"`
	HeapAlloc       uint64 `json:"heap_alloc_bytes"`
	SysBytes        uint64 `json:"sys_bytes"`
	NumCPU          int    `json:"num_cpu"`
	RecordingsDir   string `json:"recordings_dir,omitempty"`
	RecordingsBytes int64  `json:"recordings_dir_bytes"`
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (s StatusProvider) handleStatus(w http.ResponseWriter, r *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	resp := statusResponse{
		ActiveCalls:     0,
		WSConnections:   0,
		Goroutines:      runtime.NumGoroutine(),
		HeapAlloc:       mem.HeapAlloc,
		SysBytes:        mem.Sys,
		NumCPU:          runtime.NumCPU(),
		RecordingsDir:   s.RecordingsDir,
		RecordingsBytes: dirSize(s.RecordingsDir),
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

func dirSize(root string) int64 {
	if root == "" {
		return 0
	}
	var total int64
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}
