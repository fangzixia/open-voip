package ports

import (
	"context"
	"time"
)

// BridgeSessionPort 持久化 os_bridges 运行时桥接记录。
type BridgeSessionPort interface {
	ActivatePair(ctx context.Context, applicationID, callID, legA, legB string) (bridgeID string, err error)
	ReplacePair(ctx context.Context, applicationID, callID, bridgeID, legA, legB string) error
	EndByCall(ctx context.Context, applicationID, callID string) ([]string, error)
	EndBridge(ctx context.Context, applicationID, callID, bridgeID string) error
}

// IVRSessionView 从 os_ivr_sessions 读取的运行时行。
type IVRSessionView struct {
	CallID        string
	ApplicationID string
	FlowID        string
	FlowVersion   int
	NodeID        string
	StateJSON     string
	DeadlineAt    *time.Time
}

// IVRSessionPort 持久化 os_ivr_sessions 运行时节点状态。
type IVRSessionPort interface {
	UpsertIVRSession(ctx context.Context, applicationID, callID, flowID string, flowVersion int, nodeID, stateJSON string, deadline *time.Time) error
	GetIVRSession(ctx context.Context, callID string) (IVRSessionView, error)
	DeleteIVRSession(ctx context.Context, callID string) error
}
