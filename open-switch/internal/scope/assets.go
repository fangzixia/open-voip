package scope

import (
	"crypto/sha256"
	"encoding/hex"
)

// AssetNamespace 单租户固定素材目录命名空间。
func AssetNamespace() string {
	sum := sha256.Sum256([]byte("default"))
	return hex.EncodeToString(sum[:])
}
