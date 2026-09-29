package ports

import (
	"context"
	"time"
)

// BusinessAction describes a bounded request for an application-owned decision.
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

type BusinessActionStore interface {
	BeginBusinessAction(context.Context, BusinessAction) error
	GetBusinessAction(context.Context, string) (BusinessAction, error)
	ResolveBusinessAction(context.Context, string, string) error
	ExpireBusinessAction(context.Context, string) error
}
