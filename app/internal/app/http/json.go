package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"open-voip/internal/app/http/middleware"
	"open-voip/internal/errs"
	"open-voip/internal/layers/biz/auth"
)

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
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return errs.InvalidRequest("JSON 无法解析")
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
