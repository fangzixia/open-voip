package cccore

import (
	"context"
	"encoding/json"
	"errors"

	"gorm.io/gorm"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/store/models"
)

// GetActiveConfiguration 返回当前激活的配置包；尚未激活时返回 Conflict。
func (s *Service) GetActiveConfiguration(ctx context.Context) (ports.ActiveConfigurationView, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return ports.ActiveConfigurationView{}, err
	}
	bundle, view, err := s.loadActiveBundle(ctx, appID)
	if err != nil {
		return ports.ActiveConfigurationView{}, err
	}
	return ports.ActiveConfigurationView{Version: view, Bundle: bundle}, nil
}

// GetActiveConfigurationSummary 仅返回激活版本元数据。
func (s *Service) GetActiveConfigurationSummary(ctx context.Context) (ports.ConfigVersionView, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return ports.ConfigVersionView{}, err
	}
	version, err := activeVersion(s.db.WithContext(ctx), appID)
	if err != nil {
		return ports.ConfigVersionView{}, err
	}
	return s.GetConfigVersion(ctx, version)
}

func (s *Service) loadActiveBundle(ctx context.Context, appID string) (ports.ConfigBundle, ports.ConfigVersionView, error) {
	version, err := activeVersion(s.db.WithContext(ctx), appID)
	if err != nil {
		return ports.ConfigBundle{}, ports.ConfigVersionView{}, err
	}
	var row models.ConfigVersion
	if err := s.db.WithContext(ctx).Where("application_id = ? AND version = ?", appID, version).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.ConfigBundle{}, ports.ConfigVersionView{}, errs.NotFound("配置版本不存在")
		}
		return ports.ConfigBundle{}, ports.ConfigVersionView{}, err
	}
	var bundle ports.ConfigBundle
	if err := json.Unmarshal([]byte(row.Payload), &bundle); err != nil {
		return ports.ConfigBundle{}, ports.ConfigVersionView{}, errs.Internal("配置 payload 损坏: " + err.Error())
	}
	bundle.Version = version
	return bundle, configView(row), nil
}

// loadActiveBundleOrEmpty 无激活配置时返回空 bundle（用于首次写入）。
func (s *Service) loadActiveBundleOrEmpty(ctx context.Context, appID string) (ports.ConfigBundle, error) {
	bundle, _, err := s.loadActiveBundle(ctx, appID)
	if err != nil {
		if errors.Is(err, errs.ErrConflict) {
			return ports.ConfigBundle{
				Queues: []ports.QueueConfig{},
				Skills: []ports.SkillConfig{},
				Agents: []ports.AgentConfig{},
				DIDs:   []ports.DIDConfig{},
				IVRs:   []ports.IVRConfig{},
			}, nil
		}
		return ports.ConfigBundle{}, err
	}
	return bundle, nil
}
