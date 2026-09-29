package store

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"open-call/internal/ports"
	"open-call/internal/store/models"
)

// SeedSwitchDemoConfig 在 Switch 写入演示队列、坐席路由与 DID（业务库仅保留用户/坐席主数据）。
func SeedSwitchDemoConfig(sw ports.SwitchAdminPort, db *gorm.DB, log *slog.Logger) error {
	if sw == nil {
		return nil
	}
	ctx := context.Background()
	queues, err := sw.ListQueueConfigs(ctx)
	if err != nil {
		return fmt.Errorf("查询 Switch 队列: %w", err)
	}
	if len(queues) > 0 {
		return nil
	}
	var agents []models.Agent
	if err := db.Order("extension").Find(&agents).Error; err != nil {
		return err
	}
	if len(agents) == 0 {
		return nil
	}
	users := map[string]models.User{}
	var userRows []models.User
	if err := db.Find(&userRows).Error; err != nil {
		return err
	}
	for _, u := range userRows {
		users[u.ID] = u
	}
	for _, ag := range agents {
		u := users[ag.UserID]
		terminal := ag.TerminalType
		if terminal == "" {
			terminal = "webrtc"
		}
		if _, err := sw.UpsertAgentConfig(ctx, ports.SwitchAgentConfig{
			ID: ag.ID, UserRef: ag.UserID, Extension: ag.Extension, DisplayName: u.DisplayName,
			VideoCapable: ag.VideoCapable, TerminalType: terminal, SIPUsername: ag.SIPUsername, Enabled: !u.Disabled,
		}); err != nil {
			return fmt.Errorf("同步坐席 %s: %w", ag.Extension, err)
		}
	}
	audioID := uuid.NewString()
	videoID := uuid.NewString()
	agentIDs := make([]string, 0, len(agents))
	for _, ag := range agents {
		agentIDs = append(agentIDs, ag.ID)
	}
	audioQ, err := sw.CreateQueueConfig(ctx, ports.SwitchQueueConfig{
		ID: audioID, Name: "语音服务", VideoEnabled: false, MaxWaitSec: 300,
		DispatchStrategy: "longest_idle", RecordingPolicy: "audio", OverflowAction: "hangup",
		AnnounceRecording: true, AgentIDs: agentIDs,
	})
	if err != nil {
		return fmt.Errorf("创建语音队列: %w", err)
	}
	_, err = sw.CreateQueueConfig(ctx, ports.SwitchQueueConfig{
		ID: videoID, Name: "视频服务", VideoEnabled: true, MaxWaitSec: 300,
		DispatchStrategy: "longest_idle", RecordingPolicy: "video_composite", OverflowAction: "queue",
		OverflowQueueID: audioQ.ID, AnnounceRecording: true, AgentIDs: agentIDs,
	})
	if err != nil {
		return fmt.Errorf("创建视频队列: %w", err)
	}
	if _, err := sw.UpsertDIDConfig(ctx, ports.SwitchDIDConfig{
		ID: uuid.NewString(), TrunkID: "*", DID: "8001", TargetType: "queue", TargetID: audioQ.ID,
	}); err != nil {
		return fmt.Errorf("创建 DID 8001: %w", err)
	}
	if log != nil {
		log.Info("已在 Switch 写入演示队列与 DID", "did", "8001", "audio_queue", audioQ.ID)
	}
	return nil
}
