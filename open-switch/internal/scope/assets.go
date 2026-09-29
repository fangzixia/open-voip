package scope

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
)

// AssetNamespace is a stable, path-safe directory derived from the authenticated application.
func AssetNamespace(ctx context.Context) string {
	sum := sha256.Sum256([]byte(Application(ctx)))
	return hex.EncodeToString(sum[:])
}
