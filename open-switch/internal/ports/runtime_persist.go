package ports

import "context"

// BridgeSessionPort 持久化 os_bridges 运行时桥接记录。
type BridgeSessionPort interface {
	ActivatePair(ctx context.Context, callID, legA, legB string) (bridgeID string, err error)
	ReplacePair(ctx context.Context, callID, bridgeID, legA, legB string) error
	EndByCall(ctx context.Context, callID string) ([]string, error)
	EndBridge(ctx context.Context, callID, bridgeID string) error
}
