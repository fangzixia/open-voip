package cccore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"uuid"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/store/models"
)

func (s *Service) ListIVRFlows(ctx context.Context) ([]ports.IVRFlowView, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return nil, err
	}
	var rows []models.IVRFlow
	if err := s.db.WithContext(ctx).Where("application_id = ?", appID).Order("created_at").Find(&rows).Error; err != nil {
		return nil, err
	}
	bundle, _ := s.loadActiveBundleOrEmpty(ctx, appID)
	pub := map[string]int{}
	for _, ivr := range bundle.IVRs {
		if ivr.Version > pub[ivr.FlowID] {
			pub[ivr.FlowID] = ivr.Version
		}
	}
	out := make([]ports.IVRFlowView, 0, len(rows))
	for _, r := range rows {
		out = append(out, ports.IVRFlowView{
			ID: r.ID, Name: r.Name, DraftJSON: r.DraftJSON,
			PublishedVersion: pub[r.ID],
			CreatedAt:        r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}
	return out, nil
}

func (s *Service) CreateIVRFlow(ctx context.Context, name, draftJSON string) (ports.IVRFlowView, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return ports.IVRFlowView{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return ports.IVRFlowView{}, errs.InvalidRequest("流程名称必填")
	}
	if draftJSON == "" {
		draftJSON = `{"start":"end","nodes":{"end":{"type":"hangup"}}}`
	}
	if !json.Valid([]byte(draftJSON)) {
		return ports.IVRFlowView{}, errs.InvalidRequest("草稿 JSON 无效")
	}
	now := time.Now().UTC()
	row := models.IVRFlow{ApplicationID: appID, ID: uuid.New().String(), Name: name, DraftJSON: draftJSON, CreatedAt: now, UpdatedAt: now}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return ports.IVRFlowView{}, err
	}
	return ports.IVRFlowView{ID: row.ID, Name: row.Name, DraftJSON: row.DraftJSON, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}

func (s *Service) GetIVRFlow(ctx context.Context, flowID string) (ports.IVRFlowView, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return ports.IVRFlowView{}, err
	}
	var row models.IVRFlow
	if err := s.db.WithContext(ctx).Where("application_id = ? AND id = ?", appID, flowID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.IVRFlowView{}, errs.NotFound("IVR 流程不存在")
		}
		return ports.IVRFlowView{}, err
	}
	bundle, _ := s.loadActiveBundleOrEmpty(ctx, appID)
	ver := 0
	for _, ivr := range bundle.IVRs {
		if ivr.FlowID == flowID && ivr.Version > ver {
			ver = ivr.Version
		}
	}
	return ports.IVRFlowView{ID: row.ID, Name: row.Name, DraftJSON: row.DraftJSON, PublishedVersion: ver, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}

func (s *Service) UpdateIVRFlow(ctx context.Context, flowID, name, draftJSON string) (ports.IVRFlowView, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return ports.IVRFlowView{}, err
	}
	var row models.IVRFlow
	if err := s.db.WithContext(ctx).Where("application_id = ? AND id = ?", appID, flowID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.IVRFlowView{}, errs.NotFound("IVR 流程不存在")
		}
		return ports.IVRFlowView{}, err
	}
	updates := map[string]any{"updated_at": time.Now().UTC()}
	if strings.TrimSpace(name) != "" {
		updates["name"] = strings.TrimSpace(name)
	}
	if draftJSON != "" {
		if !json.Valid([]byte(draftJSON)) {
			return ports.IVRFlowView{}, errs.InvalidRequest("草稿 JSON 无效")
		}
		updates["draft_json"] = draftJSON
	}
	if err := s.db.WithContext(ctx).Model(&row).Updates(updates).Error; err != nil {
		return ports.IVRFlowView{}, err
	}
	return s.GetIVRFlow(ctx, flowID)
}

func (s *Service) DeleteIVRFlow(ctx context.Context, flowID string) error {
	appID, err := applicationID(ctx)
	if err != nil {
		return err
	}
	res := s.db.WithContext(ctx).Where("application_id = ? AND id = ?", appID, flowID).Delete(&models.IVRFlow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("IVR 流程不存在")
	}
	_, err = s.ApplyConfigMutation(ctx, func(b *ports.ConfigBundle) error {
		out := b.IVRs[:0]
		for _, ivr := range b.IVRs {
			if ivr.FlowID != flowID {
				out = append(out, ivr)
			}
		}
		b.IVRs = out
		for i := range b.Queues {
			if b.Queues[i].IVRFlowID == flowID {
				b.Queues[i].IVRFlowID = ""
			}
		}
		for i := range b.DIDs {
			if b.DIDs[i].TargetType == "ivr" && b.DIDs[i].TargetID == flowID {
				return errs.InvalidRequest("仍有 DID 引用该 IVR")
			}
		}
		return nil
	})
	return err
}

// PublishIVRFlow 将草稿发布进配置包并激活。
func (s *Service) PublishIVRFlow(ctx context.Context, flowID string) (ports.IVRVersionView, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return ports.IVRVersionView{}, err
	}
	var row models.IVRFlow
	if err := s.db.WithContext(ctx).Where("application_id = ? AND id = ?", appID, flowID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.IVRVersionView{}, errs.NotFound("IVR 流程不存在")
		}
		return ports.IVRVersionView{}, err
	}
	payload := strings.TrimSpace(row.DraftJSON)
	if !json.Valid([]byte(payload)) {
		return ports.IVRVersionView{}, errs.InvalidRequest("草稿无效，无法发布")
	}
	var view ports.IVRVersionView
	_, err = s.ApplyConfigMutation(ctx, func(b *ports.ConfigBundle) error {
		next := 1
		for _, ivr := range b.IVRs {
			if ivr.FlowID == flowID && ivr.Version >= next {
				next = ivr.Version + 1
			}
		}
		replaced := false
		for i, ivr := range b.IVRs {
			if ivr.FlowID == flowID {
				b.IVRs[i] = ports.IVRConfig{FlowID: flowID, Version: next, PayloadJSON: payload}
				replaced = true
				break
			}
		}
		if !replaced {
			b.IVRs = append(b.IVRs, ports.IVRConfig{FlowID: flowID, Version: next, PayloadJSON: payload})
		}
		view = ports.IVRVersionView{FlowID: flowID, Version: next, PayloadJSON: payload, PublishedAt: time.Now().UTC()}
		return nil
	})
	return view, err
}

func (s *Service) ListIVRVersions(ctx context.Context, flowID string) ([]ports.IVRVersionView, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return nil, err
	}
	bundle, err := s.loadActiveBundleOrEmpty(ctx, appID)
	if err != nil {
		return nil, err
	}
	out := []ports.IVRVersionView{}
	for _, ivr := range bundle.IVRs {
		if ivr.FlowID == flowID {
			out = append(out, ports.IVRVersionView{FlowID: flowID, Version: ivr.Version, PayloadJSON: ivr.PayloadJSON, PublishedAt: time.Now().UTC()})
		}
	}
	if len(out) == 0 {
		if _, err := s.GetIVRFlow(ctx, flowID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// RollbackIVRFlow 将草稿恢复为指定已发布版本并再次发布。
func (s *Service) RollbackIVRFlow(ctx context.Context, flowID string, version int) (ports.IVRVersionView, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return ports.IVRVersionView{}, err
	}
	bundle, err := s.loadActiveBundleOrEmpty(ctx, appID)
	if err != nil {
		return ports.IVRVersionView{}, err
	}
	var payload string
	for _, ivr := range bundle.IVRs {
		if ivr.FlowID == flowID && ivr.Version == version {
			payload = ivr.PayloadJSON
			break
		}
	}
	if payload == "" {
		return ports.IVRVersionView{}, errs.NotFound("IVR 版本不存在")
	}
	if err := s.db.WithContext(ctx).Model(&models.IVRFlow{}).
		Where("application_id = ? AND id = ?", appID, flowID).
		Updates(map[string]any{"draft_json": payload, "updated_at": time.Now().UTC()}).Error; err != nil {
		return ports.IVRVersionView{}, err
	}
	return s.PublishIVRFlow(ctx, flowID)
}
