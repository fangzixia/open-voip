package configpub

import (
	"context"
	"strings"

	"uuid"

	"open-call/internal/errs"
	"open-call/internal/ports"
)

// normalizeDID 与 Switch NormalizeDID 一致：仅保留数字，首位可保留 +。
func normalizeDID(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for i, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
			continue
		}
		if r == '+' && i == 0 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// DIDDTO DID 路由。
type DIDDTO struct {
	ID         string `json:"id"`
	TrunkID    string `json:"trunk_id"`
	DID        string `json:"did"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id,omitempty"`
}

// SnapshotService 通过 Switch 管理 DID 与队列 IVR 绑定。
type SnapshotService struct {
	sw ports.SwitchAdminPort
}

// NewSnapshotService 创建配置快照服务。
func NewSnapshotService(sw ports.SwitchAdminPort) *SnapshotService {
	return &SnapshotService{sw: sw}
}

// ListDID DID 列表。
func (s *SnapshotService) ListDID(ctx context.Context) ([]DIDDTO, error) {
	rows, err := s.sw.ListDIDConfigs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]DIDDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, DIDDTO{ID: r.ID, TrunkID: r.TrunkID, DID: r.DID, TargetType: r.TargetType, TargetID: r.TargetID})
	}
	return out, nil
}

// UpsertDID 创建或更新 DID。
func (s *SnapshotService) UpsertDID(ctx context.Context, trunkID, did, targetType, targetID string) (DIDDTO, error) {
	if trunkID == "" {
		trunkID = "*"
	}
	did = normalizeDID(did)
	if did == "" || (targetType != "queue" && targetType != "ivr" && targetType != "reject") {
		return DIDDTO{}, errs.InvalidRequest("did 或 target_type 无效")
	}
	if targetType == "reject" && targetID != "" {
		return DIDDTO{}, errs.InvalidRequest("reject 路由不能包含 target_id")
	}
	if targetType != "reject" && targetID == "" {
		return DIDDTO{}, errs.InvalidRequest("target_id 必填")
	}
	if targetType == "queue" {
		q, err := s.sw.GetQueueConfig(ctx, targetID)
		if err != nil {
			return DIDDTO{}, errs.InvalidRequest("目标队列不存在")
		}
		if q.VideoEnabled {
			return DIDDTO{}, errs.InvalidRequest("电话 DID 不能路由到视频队列")
		}
	}
	if targetType == "ivr" {
		flow, err := s.sw.GetIVRFlow(ctx, targetID)
		if err != nil || flow.Version == 0 {
			return DIDDTO{}, errs.InvalidRequest("目标 IVR 尚未发布")
		}
	}
	existing, _ := s.sw.ListDIDConfigs(ctx)
	var id string
	for _, row := range existing {
		if row.TrunkID == trunkID && normalizeDID(row.DID) == did {
			id = row.ID
			break
		}
	}
	if id == "" {
		id = uuid.New().String()
	}
	cfg := ports.SwitchDIDConfig{ID: id, TrunkID: trunkID, DID: did, TargetType: targetType, TargetID: targetID}
	out, err := s.sw.UpsertDIDConfig(ctx, cfg)
	if err != nil {
		return DIDDTO{}, err
	}
	return DIDDTO{ID: out.ID, TrunkID: out.TrunkID, DID: out.DID, TargetType: out.TargetType, TargetID: out.TargetID}, nil
}

// DeleteDID 删除。
func (s *SnapshotService) DeleteDID(ctx context.Context, id string) error {
	return s.sw.DeleteDIDConfig(ctx, id)
}

// BindQueueIVR 队列绑定 IVR。
func (s *SnapshotService) BindQueueIVR(ctx context.Context, queueID, flowID string) error {
	q, err := s.sw.GetQueueConfig(ctx, queueID)
	if err != nil {
		return err
	}
	if q.VideoEnabled {
		return errs.InvalidRequest("IVR 只能绑定语音队列")
	}
	if flowID != "" {
		flow, err := s.sw.GetIVRFlow(ctx, flowID)
		if err != nil || flow.Version == 0 {
			return errs.InvalidRequest("只能绑定已经发布的 IVR 流程")
		}
	}
	q.IVRFlowID = flowID
	_, err = s.sw.UpdateQueueConfig(ctx, queueID, q)
	return err
}
