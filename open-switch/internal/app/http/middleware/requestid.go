// Package middleware 提供 HTTP 横切关注点，handler 不得包含业务逻辑。
package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// RequestID 为每个请求注入 chi 生成的 Request-ID，并写入响应头。
func RequestID(next http.Handler) http.Handler {
	return middleware.RequestID(next)
}
