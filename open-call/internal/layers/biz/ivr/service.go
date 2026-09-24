package ivr

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"open-call/internal/errs"
	"open-call/internal/store/models"
)

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
type Doc struct {
	Start string          `json:"start"`
	Nodes map[string]Node `json:"nodes"`
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

type Service struct{ db *gorm.DB }

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

func (s *Service) List(ctx context.Context) ([]FlowDTO, error) {
	var rows []models.IVRFlow
	if err := s.db.WithContext(ctx).Order("created_at").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]FlowDTO, 0, len(rows))
	for _, r := range rows {
		d, err := s.toDTO(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}
func (s *Service) Create(ctx context.Context, name string, draft Doc) (FlowDTO, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return FlowDTO{}, errs.InvalidRequest("流程名称必填")
	}
	if err := s.validateDoc(ctx, draft); err != nil {
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
		if err := s.validateDoc(ctx, *draft); err != nil {
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

func (s *Service) Publish(ctx context.Context, id string) (SnapshotDTO, error) {
	var snap models.IVRPublishedSnapshot
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", id).Error; err != nil {
			return err
		}
		var row models.IVRFlow
		if err := tx.First(&row, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errs.NotFound("IVR 流程不存在")
			}
			return err
		}
		var doc Doc
		if json.Unmarshal([]byte(row.DraftJSON), &doc) != nil {
			return errs.InvalidRequest("草稿 JSON 无效")
		}
		if err := s.validateDocWithDB(tx, doc); err != nil {
			return err
		}
		var max int
		if err := tx.Model(&models.IVRPublishedSnapshot{}).Where("flow_id = ?", id).Select("COALESCE(MAX(version),0)").Scan(&max).Error; err != nil {
			return err
		}
		snap = models.IVRPublishedSnapshot{ID: uuid.New().String(), FlowID: id, Version: max + 1, PayloadJSON: row.DraftJSON, PublishedAt: time.Now().UTC()}
		return tx.Create(&snap).Error
	})
	if err != nil {
		return SnapshotDTO{}, err
	}
	return snapshotDTO(snap), nil
}
func (s *Service) Delete(ctx context.Context, id string) error {
	var n int64
	if err := s.db.WithContext(ctx).Model(&models.Queue{}).Where("ivr_flow_id = ?", id).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return errs.Conflict("IVR 仍被队列引用", "")
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
	var rows []models.IVRPublishedSnapshot
	if err := s.db.WithContext(ctx).Where("flow_id = ?", id).Order("version DESC").Find(&rows).Error; err != nil {
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
	var old models.IVRPublishedSnapshot
	if err := s.db.WithContext(ctx).Where("flow_id = ? AND version = ?", id, version).First(&old).Error; err != nil {
		return SnapshotDTO{}, errs.NotFound("IVR 版本不存在")
	}
	if err := s.db.WithContext(ctx).Model(&models.IVRFlow{}).Where("id = ?", id).Updates(map[string]any{"draft_json": old.PayloadJSON, "updated_at": time.Now().UTC()}).Error; err != nil {
		return SnapshotDTO{}, err
	}
	return s.Publish(ctx, id)
}

func (s *Service) toDTO(ctx context.Context, row models.IVRFlow) (FlowDTO, error) {
	out := FlowDTO{ID: row.ID, Name: row.Name, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	_ = json.Unmarshal([]byte(row.DraftJSON), &out.Draft)
	var snap models.IVRPublishedSnapshot
	if s.db.WithContext(ctx).Where("flow_id = ?", row.ID).Order("version DESC").First(&snap).Error == nil {
		out.PublishedVer = snap.Version
		out.PublishedJSON = snap.PayloadJSON
	}
	return out, nil
}
func snapshotDTO(x models.IVRPublishedSnapshot) SnapshotDTO {
	return SnapshotDTO{ID: x.ID, FlowID: x.FlowID, Version: x.Version, PayloadJSON: x.PayloadJSON, PublishedAt: x.PublishedAt}
}

func (s *Service) validateDoc(ctx context.Context, d Doc) error {
	return s.validateDocWithDB(s.db.WithContext(ctx), d)
}
func (s *Service) validateDocWithDB(db *gorm.DB, d Doc) error {
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
		refs := []string{}
		switch n.Type {
		case "play":
			refs = []string{n.Next}
		case "menu":
			if len(n.Choices) == 0 {
				return errs.InvalidRequest("menu 必须包含 choices")
			}
			for _, v := range n.Choices {
				refs = append(refs, v)
			}
			if n.Default != "" {
				refs = append(refs, n.Default)
			}
		case "time_check":
			refs = []string{n.Open, n.Closed}
		case "route_queue":
			if n.QueueID == "" {
				return errs.InvalidRequest("route_queue 必须指定 queue_id")
			}
			var count int64
			if err := db.Model(&models.Queue{}).Where("id = ?", n.QueueID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return errs.InvalidRequest("节点 " + id + " 引用了不存在的队列")
			}
		}
		for _, ref := range refs {
			if ref == "" {
				return errs.InvalidRequest("节点 " + id + " 缺少后续节点")
			}
			if _, ok := d.Nodes[ref]; !ok {
				return errs.InvalidRequest("节点 " + id + " 引用了不存在的节点 " + ref)
			}
		}
	}
	reachable := map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		if id == "" || reachable[id] {
			return
		}
		reachable[id] = true
		n := d.Nodes[id]
		switch n.Type {
		case "play":
			visit(n.Next)
		case "menu":
			for _, v := range n.Choices {
				visit(v)
			}
			visit(n.Default)
		case "time_check":
			visit(n.Open)
			visit(n.Closed)
		}
	}
	visit(d.Start)
	if len(reachable) != len(d.Nodes) {
		return errs.InvalidRequest("IVR 包含不可达节点")
	}
	return nil
}
