package scope

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
)

// AssetNamespace 根据已认证应用 ID 生成稳定的、可用于路径的素材目录名。
func AssetNamespace(ctx context.Context) string {
	sum := sha256.Sum256([]byte(Application(ctx)))
	return hex.EncodeToString(sum[:])
}
