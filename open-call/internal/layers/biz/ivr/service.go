package ivr

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"uuid"

	"open-call/internal/errs"
	"open-call/internal/ports"
	"open-call/internal/store/models"
)

type Node struct {
	Type           string            `json:"type"`
	Action         string            `json:"action,omitempty"`
	Prompt         string            `json:"prompt,omitempty"`
	File           string            `json:"file,omitempty"`
	TimeoutSec     int               `json:"timeout_sec,omitempty"`
	MaxRetries     *int              `json:"max_retries,omitempty"`
	Choices        map[string]string `json:"choices,omitempty"`
	Default        string            `json:"default,omitempty"`
	Invalid        string            `json:"invalid,omitempty"`
	QueueID        string            `json:"queue_id,omitempty"`
	SessionType    string            `json:"session_type,omitempty"`
	Next           string            `json:"next,omitempty"`
	Open           string            `json:"open,omitempty"`
	Closed         string            `json:"closed,omitempty"`
	Schedule       string            `json:"schedule,omitempty"`
	Busy           string            `json:"busy,omitempty"`
	WaitingGt      int               `json:"waiting_gt,omitempty"`
	AcceptedDigits string            `json:"accepted_digits,omitempty"`
	ResultKey      string            `json:"result_key,omitempty"`
}
type Doc struct {
	Start  string              `json:"start"`
	Nodes  map[string]Node     `json:"nodes"`
	Layout map[string]Position `json:"layout,omitempty"`
}
type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}
type FlowDTO struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Draft         Doc       `json:"draft"`
	PublishedVer  int       `json:"published_version,omitempty"`
	PublishedJSON string    `json:"published_json,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
type SnapshotDTO struct {
	ID          string    `json:"id"`
	FlowID      string    `json:"flow_id"`
	Version     int       `json:"version"`
	PayloadJSON string    `json:"payload_json"`
	PublishedAt time.Time `json:"published_at"`
}

type Service struct {
	db *gorm.DB
	sw ports.SwitchAdminPort
}

func NewService(db *gorm.DB, sw ports.SwitchAdminPort) *Service {
	return &Service{db: db, sw: sw}
}

func (s *Service) List(ctx context.Context) ([]FlowDTO, error) {
	var rows []models.IVRFlow
	if err := s.db.WithContext(ctx).Order("created_at").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]FlowDTO, 0, len(rows))
	for _, r := range rows {
		dto, err := s.flowDTO(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, dto)
	}
	return out, nil
}

func (s *Service) Create(ctx context.Context, name string, draft Doc) (FlowDTO, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return FlowDTO{}, errs.InvalidRequest("流程名称必填")
	}
	if len(draft.Nodes) > 100 {
		return FlowDTO{}, errs.InvalidRequest("IVR 节点不能超过 100 个")
	}
	if len(draft.Nodes) == 0 {
		draft = Doc{Start: "end", Nodes: map[string]Node{"end": {Type: "hangup"}}}
	}
	raw, err := json.Marshal(draft)
	if err != nil {
		return FlowDTO{}, err
	}
	now := time.Now().UTC()
	row := models.IVRFlow{ID: uuid.New().String(), Name: name, DraftJSON: string(raw), CreatedAt: now, UpdatedAt: now}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return FlowDTO{}, err
	}
	return s.flowDTO(ctx, row)
}

func (s *Service) Update(ctx context.Context, id, name string, draft *Doc) (FlowDTO, error) {
	var row models.IVRFlow
	if err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return FlowDTO{}, errs.NotFound("IVR 流程不存在")
		}
		return FlowDTO{}, err
	}
	updates := map[string]any{"updated_at": time.Now().UTC()}
	if strings.TrimSpace(name) != "" {
		updates["name"] = strings.TrimSpace(name)
	}
	if draft != nil {
		if len(draft.Nodes) > 100 {
			return FlowDTO{}, errs.InvalidRequest("IVR 节点不能超过 100 个")
		}
		raw, err := json.Marshal(draft)
		if err != nil {
			return FlowDTO{}, err
		}
		updates["draft_json"] = string(raw)
	}
	if err := s.db.WithContext(ctx).Model(&row).Updates(updates).Error; err != nil {
		return FlowDTO{}, err
	}
	return s.Get(ctx, id)
}

func (s *Service) Get(ctx context.Context, id string) (FlowDTO, error) {
	var row models.IVRFlow
	if err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return FlowDTO{}, errs.NotFound("IVR 流程不存在")
		}
		return FlowDTO{}, err
	}
	return s.flowDTO(ctx, row)
}

func (s *Service) Publish(ctx context.Context, id string) (SnapshotDTO, error) {
	var row models.IVRFlow
	if err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SnapshotDTO{}, errs.NotFound("IVR 流程不存在")
		}
		return SnapshotDTO{}, err
	}
	payload, err := CompilePayload(row.DraftJSON)
	if err != nil {
		return SnapshotDTO{}, err
	}
	if err := s.sw.ValidateIVRFlowPayload(ctx, payload); err != nil {
		return SnapshotDTO{}, err
	}
	var next int
	if err := s.db.WithContext(ctx).Raw(
		`SELECT COALESCE(MAX(version), 0) + 1 FROM oc_ivr_published_snapshots WHERE flow_id = ?`, id,
	).Scan(&next).Error; err != nil {
		return SnapshotDTO{}, err
	}
	now := time.Now().UTC()
	snap := models.IVRPublishedSnapshot{
		ID: uuid.New().String(), FlowID: id, Version: next, PayloadJSON: payload, PublishedAt: now,
	}
	if err := s.db.WithContext(ctx).Create(&snap).Error; err != nil {
		return SnapshotDTO{}, err
	}
	if _, err := s.sw.UpsertIVRFlow(ctx, id, payload); err != nil {
		_ = s.db.WithContext(ctx).Delete(&models.IVRPublishedSnapshot{}, "id = ?", snap.ID)
		return SnapshotDTO{}, err
	}
	return snapshotDTO(snap), nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	queues, err := s.sw.ListQueueConfigs(ctx)
	if err != nil {
		return err
	}
	for _, q := range queues {
		if q.IVRFlowID == id || q.PostCallIVRFlowID == id {
			return errs.Conflict("IVR 仍被队列引用", "")
		}
	}
	routes, err := s.sw.ListDIDConfigs(ctx)
	if err != nil {
		return err
	}
	for _, d := range routes {
		if d.TargetType == "ivr" && d.TargetID == id {
			return errs.Conflict("IVR 仍被 DID 路由引用", "")
		}
	}
	if err := s.sw.DeleteIVRFlow(ctx, id); err != nil {
		if api := errs.AsAPIError(err); api == nil || api.HTTP != 404 {
			return err
		}
	}
	res := s.db.WithContext(ctx).Delete(&models.IVRFlow{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("IVR 流程不存在")
	}
	return nil
}

func (s *Service) ListVersions(ctx context.Context, id string) ([]SnapshotDTO, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	var rows []models.IVRPublishedSnapshot
	if err := s.db.WithContext(ctx).Where("flow_id = ?", id).Order("version desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]SnapshotDTO, 0, len(rows))
	for _, x := range rows {
		out = append(out, snapshotDTO(x))
	}
	return out, nil
}

func (s *Service) ListSnapshots(ctx context.Context, id string) ([]SnapshotDTO, error) {
	return s.ListVersions(ctx, id)
}

func (s *Service) Rollback(ctx context.Context, id string, version int) (SnapshotDTO, error) {
	var snap models.IVRPublishedSnapshot
	if err := s.db.WithContext(ctx).Where("flow_id = ? AND version = ?", id, version).First(&snap).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SnapshotDTO{}, errs.NotFound("IVR 版本不存在")
		}
		return SnapshotDTO{}, err
	}
	if err := s.db.WithContext(ctx).Model(&models.IVRFlow{}).Where("id = ?", id).
		Updates(map[string]any{"draft_json": snap.PayloadJSON, "updated_at": time.Now().UTC()}).Error; err != nil {
		return SnapshotDTO{}, err
	}
	return s.Publish(ctx, id)
}

func (s *Service) flowDTO(ctx context.Context, row models.IVRFlow) (FlowDTO, error) {
	out := FlowDTO{
		ID: row.ID, Name: row.Name,
		CreatedAt: row.CreatedAt.UTC(),
		UpdatedAt: row.UpdatedAt.UTC(),
	}
	_ = json.Unmarshal([]byte(row.DraftJSON), &out.Draft)
	var latest models.IVRPublishedSnapshot
	err := s.db.WithContext(ctx).Where("flow_id = ?", row.ID).Order("version desc").First(&latest).Error
	if err == nil {
		out.PublishedVer = latest.Version
		out.PublishedJSON = latest.PayloadJSON
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return FlowDTO{}, err
	}
	return out, nil
}

func snapshotDTO(x models.IVRPublishedSnapshot) SnapshotDTO {
	return SnapshotDTO{
		ID:          x.ID,
		FlowID:      x.FlowID,
		Version:     x.Version,
		PayloadJSON: x.PayloadJSON,
		PublishedAt: x.PublishedAt.UTC(),
	}
}

func (s *Service) LatestPublished(ctx context.Context, flowID string) (models.IVRPublishedSnapshot, error) {
	var latest models.IVRPublishedSnapshot
	err := s.db.WithContext(ctx).Where("flow_id = ?", flowID).Order("version desc").First(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.IVRPublishedSnapshot{}, errs.NotFound("IVR 尚未发布")
	}
	return latest, err
}
