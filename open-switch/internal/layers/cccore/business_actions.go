package cccore

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/scope"
	"open-switch/internal/store"
)

// businessActionRow 映射 os_business_actions 表行。
type businessActionRow struct {
	ID            string
	ApplicationID string
	CallID        string
	NodeID        string
	Action        string
	Outcomes      string
	DeadlineAt    time.Time
	Status        string
	Outcome       string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (businessActionRow) TableName() string { return "os_business_actions" }

// BeginBusinessAction 创建待处理动作并发布 business_action.requested 事件。
func (s *Service) BeginBusinessAction(ctx context.Context, a ports.BusinessAction) error {
	app, err := applicationID(ctx)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(a.Outcomes)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := businessActionRow{ID: a.ID, ApplicationID: app, CallID: a.CallID, NodeID: a.NodeID, Action: a.Action, Outcomes: string(raw), DeadlineAt: a.Deadline, Status: "pending", CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return (store.CallEvents{DB: tx}).PublishCallEvent(ctx, ports.CallEvent{CallID: a.CallID, Type: "business_action.requested", Payload: map[string]any{
			"call_id": a.CallID, "action_id": a.ID, "node_id": a.NodeID, "action": a.Action, "deadline": a.Deadline, "outcomes": a.Outcomes,
		}})
	})
}

// GetBusinessAction 按 ID 读取当前应用下的业务动作。
func (s *Service) GetBusinessAction(ctx context.Context, id string) (ports.BusinessAction, error) {
	app, err := applicationID(ctx)
	if err != nil {
		return ports.BusinessAction{}, err
	}
	var row businessActionRow
	if err := s.db.WithContext(ctx).Where("application_id=? AND id=?", app, id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.BusinessAction{}, errs.NotFound("业务动作不存在")
		}
		return ports.BusinessAction{}, err
	}
	var outcomes map[string]string
	if err := json.Unmarshal([]byte(row.Outcomes), &outcomes); err != nil {
		return ports.BusinessAction{}, err
	}
	return ports.BusinessAction{ID: row.ID, CallID: row.CallID, NodeID: row.NodeID, Action: row.Action, Outcomes: outcomes, Deadline: row.DeadlineAt, Status: row.Status, Outcome: row.Outcome}, nil
}

// ResolveBusinessAction 提交业务结果并发布 business_action.completed。
func (s *Service) ResolveBusinessAction(ctx context.Context, id, outcome string) error {
	return s.updateBusinessAction(ctx, id, outcome, false)
}

// ExpireBusinessAction 将超时动作标记为已过期并发布对应事件。
func (s *Service) ExpireBusinessAction(ctx context.Context, id string) error {
	return s.updateBusinessAction(ctx, id, "", true)
}

// updateBusinessAction 在行锁下校验截止时间、结果声明与终态，并写入事件流。
func (s *Service) updateBusinessAction(ctx context.Context, id, outcome string, expire bool) error {
	app, err := applicationID(ctx)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row businessActionRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("application_id=? AND id=?", app, id).First(&row).Error; err != nil {
			return err
		}
		if row.Status == "completed" {
			if !expire && row.Outcome == outcome {
				return nil
			}
			return errs.Conflict("业务动作已提交其他结果", "")
		}
		if row.Status == "expired" {
			if expire {
				return nil
			}
			return errs.Conflict("业务动作已过期", "")
		}
		status, kind := "completed", "business_action.completed"
		if expire {
			status, kind = "expired", "business_action.expired"
		} else {
			if !time.Now().Before(row.DeadlineAt) {
				return errs.Conflict("业务动作已超时", "")
			}
			var outcomes map[string]string
			if err := json.Unmarshal([]byte(row.Outcomes), &outcomes); err != nil {
				return err
			}
			if _, ok := outcomes[outcome]; !ok {
				return errs.InvalidRequest("业务结果未在流程中声明")
			}
		}
		if err := tx.Model(&row).Updates(map[string]any{"status": status, "outcome": outcome, "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		return (store.CallEvents{DB: tx}).PublishCallEvent(scope.WithApplication(ctx, app), ports.CallEvent{CallID: row.CallID, Type: kind, Payload: map[string]any{"call_id": row.CallID, "action_id": id, "outcome": outcome}})
	})
}
