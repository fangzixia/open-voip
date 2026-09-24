package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"open-call/internal/app/http/middleware"
	"open-call/internal/errs"
	"open-call/internal/layers/biz/auth"
)

// WriteJSON 写入 JSON 响应。
func WriteJSON(w http.ResponseWriter, status int, v any) {
	writeJSON(w, status, v)
}

// WriteErr 写入 API 错误 JSON。
func WriteErr(w http.ResponseWriter, err error) {
	writeErr(w, err)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	api := errs.AsAPIError(err)
	body := map[string]any{
		"error":   api.Kind,
		"message": api.Message,
	}
	if api.Code != "" {
		body["code"] = api.Code
	}
	writeJSON(w, api.HTTP, body)
}

func decodeJSON(r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errs.InvalidRequest("JSON 无法解析")
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errs.InvalidRequest("请求只能包含一个 JSON 对象")
	}
	return nil
}

func pageParams(r *http.Request) (page, size int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	size, _ = strconv.Atoi(r.URL.Query().Get("page_size"))
	return page, size
}

func principal(r *http.Request) (auth.Principal, bool) {
	return middleware.PrincipalFromContext(r.Context())
}
