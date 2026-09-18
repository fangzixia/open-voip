// Package errs 定义跨层可识别的 sentinel 错误。
package errs

import "errors"

// ErrNotImplemented 表示 Phase 0 占位实现，业务逻辑将在后续迭代补齐。
var ErrNotImplemented = errors.New("功能尚未实现")
