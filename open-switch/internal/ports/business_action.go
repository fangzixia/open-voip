package ports

import (
	"context"
	"time"
)

// BusinessAction 表示一次有界业务决策请求：IVR 暂停等待业务系统选择已声明的结果分支。
type BusinessAction struct {
	ID       string
	CallID   string
	NodeID   string
	Action   string
	Outcomes map[string]string
	Deadline time.Time
	Status   string
	Outcome  string
}

// BusinessActionStore 持久化并解析业务动作的生命周期。
type BusinessActionStore interface {
	BeginBusinessAction(context.Context, BusinessAction) error
	GetBusinessAction(context.Context, string) (BusinessAction, error)
	ResolveBusinessAction(context.Context, string, string) error
	ExpireBusinessAction(context.Context, string) error
}
