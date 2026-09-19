// Package errs 定义跨层可识别的 sentinel 错误与 API 错误。
package errs

import (
	"errors"
	"fmt"
	"net/http"
)

// 通用 sentinel，供 errors.Is 判断。
var (
	ErrNotImplemented = errors.New("功能尚未实现")
	ErrInvalidRequest = errors.New("invalid_request")
	ErrUnauthorized   = errors.New("unauthorized")
	ErrForbidden      = errors.New("forbidden")
	ErrNotFound       = errors.New("not_found")
	ErrConflict       = errors.New("conflict")
)

// 业务码，与 docs/api/errors.md 对齐。
const (
	CodeCallNotRinging         = "CALL_NOT_RINGING"
	CodeAgentNotIdle           = "AGENT_NOT_IDLE"
	CodeAgentNotVideoCapable   = "AGENT_NOT_VIDEO_CAPABLE"
	CodeQueueFull              = "QUEUE_FULL"
	CodeGuestTokenExpired      = "GUEST_TOKEN_EXPIRED"
	CodeSIPDisabled            = "SIP_DISABLED"
	CodeTransferFailed         = "TRANSFER_FAILED"
	CodeAgentBusy              = "AGENT_BUSY"
)

// APIError 可映射为 HTTP JSON 错误体。
type APIError struct {
	// Kind 对应 error 字段，如 unauthorized。
	Kind string
	// Code 可选业务码。
	Code string
	// Message 人类可读说明。
	Message string
	// HTTP 状态码。
	HTTP int
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Kind
}

func (e *APIError) Unwrap() error {
	switch e.Kind {
	case "invalid_request":
		return ErrInvalidRequest
	case "unauthorized":
		return ErrUnauthorized
	case "forbidden":
		return ErrForbidden
	case "not_found":
		return ErrNotFound
	case "conflict":
		return ErrConflict
	case "not_implemented":
		return ErrNotImplemented
	default:
		return nil
	}
}

// InvalidRequest 400。
func InvalidRequest(msg string) *APIError {
	return &APIError{Kind: "invalid_request", Message: msg, HTTP: http.StatusBadRequest}
}

// Unauthorized 401。
func Unauthorized(msg string) *APIError {
	return &APIError{Kind: "unauthorized", Message: msg, HTTP: http.StatusUnauthorized}
}

// Forbidden 403。
func Forbidden(msg string) *APIError {
	return &APIError{Kind: "forbidden", Message: msg, HTTP: http.StatusForbidden}
}

// NotFound 404。
func NotFound(msg string) *APIError {
	return &APIError{Kind: "not_found", Message: msg, HTTP: http.StatusNotFound}
}

// Conflict 409。
func Conflict(msg, code string) *APIError {
	return &APIError{Kind: "conflict", Code: code, Message: msg, HTTP: http.StatusConflict}
}

// Unprocessable 422。
func Unprocessable(msg, code string) *APIError {
	return &APIError{Kind: "invalid_request", Code: code, Message: msg, HTTP: http.StatusUnprocessableEntity}
}

// NotImplemented 501。
func NotImplemented(feature string) *APIError {
	return &APIError{Kind: "not_implemented", Message: fmt.Sprintf("%s：尚未实现", feature), HTTP: http.StatusNotImplemented}
}

// Internal 500。
func Internal(msg string) *APIError {
	return &APIError{Kind: "internal_error", Message: msg, HTTP: http.StatusInternalServerError}
}

// AsAPIError 将任意 error 转为 APIError。
func AsAPIError(err error) *APIError {
	if err == nil {
		return nil
	}
	var api *APIError
	if errors.As(err, &api) {
		return api
	}
	switch {
	case errors.Is(err, ErrNotImplemented):
		return NotImplemented("功能")
	case errors.Is(err, ErrInvalidRequest):
		return InvalidRequest(err.Error())
	case errors.Is(err, ErrUnauthorized):
		return Unauthorized("未认证或令牌失效")
	case errors.Is(err, ErrForbidden):
		return Forbidden("无权限")
	case errors.Is(err, ErrNotFound):
		return NotFound("资源不存在")
	case errors.Is(err, ErrConflict):
		return Conflict(err.Error(), "")
	default:
		return Internal("服务器内部错误")
	}
}
