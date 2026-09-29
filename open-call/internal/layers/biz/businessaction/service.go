// Package businessaction 处理 Switch IVR business_action 节点的业务决策（方案 §7.1）。
package businessaction

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"open-call/internal/errs"
	"open-call/internal/integration/switchapi"
	"open-call/internal/store/models"
)

// Completer 向 Switch 回填 IVR 业务分支结果。
type Completer interface {
	CompleteBusinessAction(ctx context.Context, callID, actionID, outcome string) error
}

// Service 根据 action 类型解析 outcome 并调用 Switch。
type Service struct {
	db *gorm.DB
}

// NewService 创建业务动作处理器。
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// HandleRequested 消费 business_action.requested 事件并完成 Switch 侧挂起节点。
func (s *Service) HandleRequested(ctx context.Context, ev switchapi.Event, complete Completer) error {
	if ev.Type != "business_action.requested" {
		return nil
	}
	callID := ev.CallID
	actionID := stringField(ev.Payload, "action_id")
	action := stringField(ev.Payload, "action")
	if callID == "" || actionID == "" || action == "" {
		return errs.InvalidRequest("business_action 事件缺少 call_id/action_id/action")
	}
	outcomes := outcomeKeys(ev.Payload["outcomes"])
	if len(outcomes) == 0 {
		return errs.InvalidRequest("business_action 事件缺少 outcomes")
	}
	outcome, err := s.resolve(ctx, action, callID, outcomes)
	if err != nil {
		return err
	}
	if err := complete.CompleteBusinessAction(ctx, callID, actionID, outcome); err != nil {
		if api := errs.AsAPIError(err); api != nil && api.HTTP == 409 {
			return nil
		}
		return err
	}
	return nil
}

func (s *Service) resolve(ctx context.Context, action, callID string, allowed []string) (string, error) {
	switch action {
	case "customer.eligible":
		var count int64
		if err := s.db.WithContext(ctx).Model(&models.GuestSession{}).Where("call_id = ?", callID).Count(&count).Error; err != nil {
			return "", err
		}
		if count > 0 {
			return pickOutcome(allowed, "yes", "eligible", "pass")
		}
		return pickOutcome(allowed, "no", "ineligible", "fail")
	default:
		return "", errs.InvalidRequest(fmt.Sprintf("未注册的业务动作: %s", action))
	}
}

func pickOutcome(allowed []string, preferred ...string) (string, error) {
	set := map[string]struct{}{}
	for _, k := range allowed {
		set[k] = struct{}{}
	}
	for _, p := range preferred {
		if _, ok := set[p]; ok {
			return p, nil
		}
	}
	if len(allowed) > 0 {
		return allowed[0], nil
	}
	return "", errors.New("无可用 outcome")
}

func outcomeKeys(raw any) []string {
	switch v := raw.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		return keys
	case map[string]string:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		return keys
	default:
		return nil
	}
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
