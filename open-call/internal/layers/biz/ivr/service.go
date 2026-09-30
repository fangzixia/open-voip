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
	Type        string            `json:"type"`
	Action      string            `json:"action,omitempty"`
	Prompt      string            `json:"prompt,omitempty"`
	File        string            `json:"file,omitempty"`
	TimeoutSec  int               `json:"timeout_sec,omitempty"`
	MaxRetries  *int              `json:"max_retries,omitempty"`
	Choices     map[string]string `json:"choices,omitempty"`
	Default     string            `json:"default,omitempty"`
	Invalid     string            `json:"invalid,omitempty"`
	QueueID     string            `json:"queue_id,omitempty"`
	SessionType string            `json:"session_type,omitempty"`
	Next        string            `json:"next,omitempty"`
	Open        string            `json:"open,omitempty"`
	Closed      string            `json:"closed,omitempty"`
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
	var doc Doc
	if json.Unmarshal([]byte(row.DraftJSON), &doc) != nil {
		return SnapshotDTO{}, errs.InvalidRequest("草稿 JSON 无效")
	}
	if err := s.validateDoc(ctx, doc); err != nil {
		return SnapshotDTO{}, err
	}
	payload := strings.TrimSpace(row.DraftJSON)
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
		if q.IVRFlowID == id {
			return errs.Conflict("IVR 仍被队列引用", "")
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

func (s *Service) validateDoc(ctx context.Context, d Doc) error {
	if d.Start == "" || len(d.Nodes) == 0 {
		return errs.InvalidRequest("IVR 必须包含 start 与 nodes")
	}
	if _, ok := d.Nodes[d.Start]; !ok {
		return errs.InvalidRequest("start 节点不存在")
	}
	for id, pos := range d.Layout {
		if _, ok := d.Nodes[id]; !ok || pos.X < 0 || pos.X > 4000 || pos.Y < 0 || pos.Y > 4000 {
			return errs.InvalidRequest("IVR 画布位置无效")
		}
	}
	for id, n := range d.Nodes {
		if n.TimeoutSec < 0 || n.TimeoutSec > 120 {
			return errs.InvalidRequest("节点 " + id + " 超时时间必须在 0–120 秒")
		}
		if n.MaxRetries != nil && (*n.MaxRetries < 0 || *n.MaxRetries > 5) {
			return errs.InvalidRequest("节点 " + id + " 无效按键重试不能超过 5 次")
		}
		switch n.Type {
		case "play", "menu", "route_queue", "time_check", "hangup", "business_action", "tts", "asr":
		default:
			return errs.InvalidRequest("节点 " + id + " 类型无效")
		}
		refs := []string{}
		switch n.Type {
		case "play":
			refs = []string{n.Next}
		case "business_action":
			if n.Action == "" || n.TimeoutSec < 1 || n.Default == "" || len(n.Choices) == 0 {
				return errs.InvalidRequest("业务动作必须有 action、超时、default 与结果分支")
			}
			refs = append(refs, n.Default)
			for _, target := range n.Choices {
				refs = append(refs, target)
			}
		case "menu":
			if len(n.Choices) == 0 {
				return errs.InvalidRequest("menu 必须包含 choices")
			}
			for digit, v := range n.Choices {
				if len(digit) != 1 || !strings.Contains("0123456789*#", digit) {
					return errs.InvalidRequest("节点 " + id + " 包含无效按键")
				}
				refs = append(refs, v)
			}
			if n.Default == "" {
				return errs.InvalidRequest("节点 " + id + " 缺少超时去向")
			}
			refs = append(refs, n.Default)
			if n.Invalid != "" {
				refs = append(refs, n.Invalid)
			}
		case "time_check":
			if n.QueueID == "" {
				return errs.InvalidRequest("工作时间节点必须指定 queue_id")
			}
			if _, err := s.sw.GetQueueConfig(ctx, n.QueueID); err != nil {
				return errs.InvalidRequest("工作时间节点队列不存在")
			}
			refs = []string{n.Open, n.Closed}
		case "route_queue":
			if n.QueueID == "" {
				return errs.InvalidRequest("route_queue 必须指定 queue_id")
			}
			if _, err := s.sw.GetQueueConfig(ctx, n.QueueID); err != nil {
				return errs.InvalidRequest("节点 " + id + " 引用了不存在的队列")
			}
			if n.SessionType != "" && n.SessionType != "audio" && n.SessionType != "video" {
				return errs.InvalidRequest("节点 " + id + " 通话类型无效")
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
		case "menu", "business_action":
			for _, v := range n.Choices {
				visit(v)
			}
			visit(n.Default)
			visit(n.Invalid)
		case "time_check":
			visit(n.Open)
			visit(n.Closed)
		}
	}
	visit(d.Start)
	if len(reachable) != len(d.Nodes) {
		return errs.InvalidRequest("IVR 包含不可达节点")
	}
	visiting, done := map[string]bool{}, map[string]bool{}
	var checkCycle func(string) bool
	checkCycle = func(id string) bool {
		if visiting[id] {
			return true
		}
		if done[id] {
			return false
		}
		visiting[id] = true
		n := d.Nodes[id]
		refs := []string{}
		switch n.Type {
		case "play":
			refs = append(refs, n.Next)
		case "menu", "business_action":
			for _, target := range n.Choices {
				refs = append(refs, target)
			}
			refs = append(refs, n.Default, n.Invalid)
		case "time_check":
			refs = append(refs, n.Open, n.Closed)
		}
		for _, target := range refs {
			if target != "" && checkCycle(target) {
				return true
			}
		}
		visiting[id] = false
		done[id] = true
		return false
	}
	if checkCycle(d.Start) {
		return errs.InvalidRequest("IVR 不能包含循环节点")
	}
	return nil
}
