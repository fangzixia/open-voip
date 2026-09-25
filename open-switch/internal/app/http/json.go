package http

import (
	"encoding/json"
	"net/http"
	"open-switch/internal/datetime"
	"open-switch/internal/httpapi"
	"strconv"

	"open-switch/internal/app/http/middleware"
	"open-switch/internal/authctx"
	"open-switch/internal/errs"
)

func writeJSON(w http.ResponseWriter, status int, v any) { httpapi.Write(w, status, v) }

func writeErr(w http.ResponseWriter, err error) { httpapi.Error(w, err) }

func decodeJSON(r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return errs.InvalidRequest("JSON 无法解析")
	}
	if err := datetime.UnmarshalCurrent(raw, dst); err != nil {
		return errs.InvalidRequest("JSON 无法解析")
	}
	return nil
}

func pageParams(r *http.Request) (page, size int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	size, _ = strconv.Atoi(r.URL.Query().Get("page_size"))
	return page, size
}

func principal(r *http.Request) (authctx.Principal, bool) {
	return middleware.PrincipalFromContext(r.Context())
}
