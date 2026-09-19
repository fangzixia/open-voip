package ivr

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"open-voip/internal/errs"
	"open-voip/internal/store/models"
)

// Node IVR 节点。
type Node struct {
	Type        string            `json:"type"`
	Prompt      string            `json:"prompt,omitempty"`
	File        string            `json:"file,omitempty"`
	TimeoutSec  int               `json:"timeout_sec,omitempty"`
	Choices     map[string]string `json:"choices,omitempty"`
	Default     string            `json:"default,omitempty"`
	QueueID     string            `json:"queue_id,omitempty"`
	SessionType string            `json:"session_type,omitempty"`
	Next        string            `json:"next,omitempty"`
	Open        string            `json:"open,omitempty"`
	Closed      string            `json:"closed,omitempty"`
}

// Doc 已发布/草稿文档。
type Doc struct {
	Start string          `json:"start"`
	Nodes map[string]Node `json:"nodes"`
}

// FlowDTO 流程对外表示。
type FlowDTO struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Draft         Doc       `json:"draft"`
	PublishedVer  int       `json:"published_version,omitempty"`
	PublishedJSON string    `json:"published_json,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// SnapshotDTO 发布快照。
type SnapshotDTO struct {
	ID          string    `json:"id"`
	FlowID      string    `json:"flow_id"`
	Version     int       `json:"version"`
	PayloadJSON string    `json:"payload_json"`
	PublishedAt time.Time `json:"published_at"`
}

// Service IVR 草稿与发布。
type Service struct {
	db *gorm.DB
}

// NewService 创建 IVR 服务。
func NewService(db *gorm.DB) *Service { return &Service{db: db} }

// List 列出流程。
func (s *Service) List(ctx context.Context) ([]FlowDTO, error) {
	var rows []models.IVRFlow
	if err := s.db.WithContext(ctx).Order("created_at").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]FlowDTO, 0, len(rows))
	for _, r := range rows {
		dto, err := s.toDTO(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, dto)
	}
	return out, nil
}

// Create 创建草稿。
func (s *Service) Create(ctx context.Context, name string, draft Doc) (FlowDTO, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return FlowDTO{}, errs.InvalidRequest("流程名称必填")
	}
	if err := validateDoc(draft); err != nil {
		return FlowDTO{}, err
	}
	raw, _ := json.Marshal(draft)
	now := time.Now().UTC()
	row := models.IVRFlow{ID: uuid.New().String(), Name: name, DraftJSON: string(raw), CreatedAt: now, UpdatedAt: now}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return FlowDTO{}, err
	}
	return s.toDTO(ctx, row)
}

// Update 更新草稿。
func (s *Service) Update(ctx context.Context, id, name string, draft *Doc) (FlowDTO, error) {
	var row models.IVRFlow
	if err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return FlowDTO{}, errs.NotFound("IVR 流程不存在")
		}
		return FlowDTO{}, err
	}
	updates := map[string]any{"updated_at": time.Now().UTC()}
	if name != "" {
		updates["name"] = name
	}
	if draft != nil {
		if err := validateDoc(*draft); err != nil {
			return FlowDTO{}, err
		}
		raw, _ := json.Marshal(draft)
		updates["draft_json"] = string(raw)
	}
	if err := s.db.WithContext(ctx).Model(&row).Updates(updates).Error; err != nil {
		return FlowDTO{}, err
	}
	return s.Get(ctx, id)
}

// Get 按 ID。
func (s *Service) Get(ctx context.Context, id string) (FlowDTO, error) {
	var row models.IVRFlow
	if err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return FlowDTO{}, errs.NotFound("IVR 流程不存在")
		}
		return FlowDTO{}, err
	}
	return s.toDTO(ctx, row)
}

// Publish 校验并发布快照。
func (s *Service) Publish(ctx context.Context, id string) (SnapshotDTO, error) {
	var row models.IVRFlow
	if err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SnapshotDTO{}, errs.NotFound("IVR 流程不存在")
		}
		return SnapshotDTO{}, err
	}
	var doc Doc
	if err := json.Unmarshal([]byte(row.DraftJSON), &doc); err != nil {
		return SnapshotDTO{}, errs.InvalidRequest("草稿 JSON 无效")
	}
	if err := validateDoc(doc); err != nil {
		return SnapshotDTO{}, err
	}
	var max int
	_ = s.db.WithContext(ctx).Model(&models.IVRPublishedSnapshot{}).Where("flow_id = ?", id).Select("COALESCE(MAX(version),0)").Scan(&max)
	snap := models.IVRPublishedSnapshot{
		ID: uuid.New().String(), FlowID: id, Version: max + 1, PayloadJSON: row.DraftJSON, PublishedAt: time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Create(&snap).Error; err != nil {
		return SnapshotDTO{}, err
	}
	return SnapshotDTO{ID: snap.ID, FlowID: snap.FlowID, Version: snap.Version, PayloadJSON: snap.PayloadJSON, PublishedAt: snap.PublishedAt}, nil
}

func (s *Service) toDTO(ctx context.Context, row models.IVRFlow) (FlowDTO, error) {
	out := FlowDTO{ID: row.ID, Name: row.Name, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	_ = json.Unmarshal([]byte(row.DraftJSON), &out.Draft)
	var snap models.IVRPublishedSnapshot
	if err := s.db.WithContext(ctx).Where("flow_id = ?", row.ID).Order("version DESC").First(&snap).Error; err == nil {
		out.PublishedVer = snap.Version
		out.PublishedJSON = snap.PayloadJSON
	}
	return out, nil
}

func validateDoc(d Doc) error {
	if d.Start == "" || len(d.Nodes) == 0 {
		return errs.InvalidRequest("IVR 必须包含 start 与 nodes")
	}
	if _, ok := d.Nodes[d.Start]; !ok {
		return errs.InvalidRequest("start 节点不存在")
	}
	for id, n := range d.Nodes {
		switch n.Type {
		case "play", "menu", "route_queue", "time_check", "hangup":
		default:
			return errs.InvalidRequest("节点 " + id + " 类型无效")
		}
		if n.Type == "route_queue" && n.QueueID == "" {
			return errs.InvalidRequest("route_queue 必须指定 queue_id")
		}
	}
	return nil
}
