package ivr

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"open-call/internal/errs"
	"open-call/internal/ports"
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
	sw ports.SwitchAdminPort
}

func NewService(sw ports.SwitchAdminPort) *Service { return &Service{sw: sw} }

func (s *Service) List(ctx context.Context) ([]FlowDTO, error) {
	rows, err := s.sw.ListIVRFlows(ctx)
	if err != nil {
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
	raw, _ := json.Marshal(draft)
	created, err := s.sw.CreateIVRFlow(ctx, name, string(raw))
	if err != nil {
		return FlowDTO{}, err
	}
	return s.flowDTO(ctx, created)
}

func (s *Service) Update(ctx context.Context, id, name string, draft *Doc) (FlowDTO, error) {
	var draftJSON, flowName string
	if strings.TrimSpace(name) != "" {
		flowName = strings.TrimSpace(name)
	}
	if draft != nil {
		if len(draft.Nodes) > 100 {
			return FlowDTO{}, errs.InvalidRequest("IVR 节点不能超过 100 个")
		}
		raw, _ := json.Marshal(draft)
		draftJSON = string(raw)
	}
	updated, err := s.sw.UpdateIVRFlow(ctx, id, flowName, draftJSON)
	if err != nil {
		return FlowDTO{}, err
	}
	return s.flowDTO(ctx, updated)
}

func (s *Service) Get(ctx context.Context, id string) (FlowDTO, error) {
	row, err := s.sw.GetIVRFlow(ctx, id)
	if err != nil {
		return FlowDTO{}, err
	}
	return s.flowDTO(ctx, row)
}

func (s *Service) Publish(ctx context.Context, id string) (SnapshotDTO, error) {
	var row ports.SwitchIVRFlowView
	row, err := s.sw.GetIVRFlow(ctx, id)
	if err != nil {
		return SnapshotDTO{}, err
	}
	var doc Doc
	if json.Unmarshal([]byte(row.DraftJSON), &doc) != nil {
		return SnapshotDTO{}, errs.InvalidRequest("草稿 JSON 无效")
	}
	if err := s.validateDoc(ctx, doc); err != nil {
		return SnapshotDTO{}, err
	}
	ver, err := s.sw.PublishIVRFlow(ctx, id)
	if err != nil {
		return SnapshotDTO{}, err
	}
	return snapshotDTO(ver), nil
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
	return s.sw.DeleteIVRFlow(ctx, id)
}

func (s *Service) ListVersions(ctx context.Context, id string) ([]SnapshotDTO, error) {
	rows, err := s.sw.ListIVRVersions(ctx, id)
	if err != nil {
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
	ver, err := s.sw.RollbackIVRFlow(ctx, id, version)
	if err != nil {
		return SnapshotDTO{}, err
	}
	return snapshotDTO(ver), nil
}

func (s *Service) flowDTO(ctx context.Context, row ports.SwitchIVRFlowView) (FlowDTO, error) {
	out := FlowDTO{
		ID: row.ID, Name: row.Name,
		CreatedAt: row.CreatedAt.UTC(),
		UpdatedAt: row.UpdatedAt.UTC(),
	}
	_ = json.Unmarshal([]byte(row.DraftJSON), &out.Draft)
	out.PublishedVer = row.PublishedVersion
	if row.PublishedVersion > 0 {
		vers, err := s.sw.ListIVRVersions(ctx, row.ID)
		if err != nil {
			return FlowDTO{}, err
		}
		for _, v := range vers {
			if v.Version == row.PublishedVersion {
				out.PublishedJSON = v.PayloadJSON
				break
			}
		}
	}
	return out, nil
}

func snapshotDTO(x ports.SwitchIVRVersionView) SnapshotDTO {
	return SnapshotDTO{
		ID:          fmt.Sprintf("%s-v%d", x.FlowID, x.Version),
		FlowID:      x.FlowID,
		Version:     x.Version,
		PayloadJSON: x.PayloadJSON,
		PublishedAt: x.PublishedAt.UTC(),
	}
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
