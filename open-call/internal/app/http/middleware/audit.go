// 本文件负责请求审计中间件。
package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	chimw "github.com/go-chi/chi/v5/middleware"

	"open-call/internal/layers/biz/audit"
)

type auditStatusWriter struct {
	http.ResponseWriter
	status int
}

func (w *auditStatusWriter) WriteHeader(code int) {
	if w.status != 0 {
		return
	}
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
func (w *auditStatusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

// AuditMutations 记录每个已认证且会修改状态的 HTTP 请求。
// 业务审计可以补充细节，此记录保证所有相关路由都有审计信息。
func AuditMutations(service *audit.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if service == nil || r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			aw := &auditStatusWriter{ResponseWriter: w}
			next.ServeHTTP(aw, r)
			p, _ := PrincipalFromContext(r.Context())
			outcome := "success"
			if aw.status >= 400 {
				outcome = "failure"
			}
			ip := r.RemoteAddr
			if i := strings.LastIndex(ip, ":"); i >= 0 {
				ip = ip[:i]
			}
			ip = strings.Trim(ip, "[]")
			detail := map[string]string{"outcome": outcome, "request_id": chimw.GetReqID(r.Context()), "remote_ip": ip, "method": r.Method}
			if err := service.Write(r.Context(), p.UserID, "http_mutation", r.URL.Path, detail); err != nil {
				slog.ErrorContext(r.Context(), "审计日志写入失败", "method", r.Method, "path", r.URL.Path, "err", err)
			}
		})
	}
}
