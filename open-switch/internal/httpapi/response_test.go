// 本文件验证HTTP 响应结构与错误转换的关键行为。
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"strings"
	"testing"
	"time"
)

func TestResponseContract(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		status  int
		code    string
	}{
		{"success", func(w http.ResponseWriter, r *http.Request) { Write(w, 201, map[string]string{"id": "one"}) }, 201, "OK"},
		{"empty", func(w http.ResponseWriter, r *http.Request) { Write(w, 204, nil) }, 200, "OK"},
		{"business", func(w http.ResponseWriter, r *http.Request) { Error(w, errs.Conflict("忙", "AGENT_BUSY")) }, 409, "AGENT_BUSY"},
		{"panic", func(w http.ResponseWriter, r *http.Request) { panic("secret") }, 500, "INTERNAL_ERROR"},
		{"missing", NotFound, 404, "NOT_FOUND"},
		{"method", MethodNotAllowed, 405, "METHOD_NOT_ALLOWED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("X-Request-ID", "contract-test")
			RequestID(Recover(tc.handler)).ServeHTTP(rec, req)
			var body map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"code", "message", "data", "request_id"} {
				if _, ok := body[key]; !ok {
					t.Fatalf("missing %s: %s", key, rec.Body)
				}
			}
			if rec.Code != tc.status || string(body["code"]) != strconvQuote(tc.code) || string(body["request_id"]) != strconvQuote("contract-test") {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
			}
			if strings.Contains(rec.Body.String(), "secret") {
				t.Fatal("panic detail leaked")
			}
		})
	}
}
func strconvQuote(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestTimestampResponseAndDecode(t *testing.T) {
	at := time.Date(2026, 9, 25, 10, 12, 35, 0, time.UTC)
	rec := httptest.NewRecorder()
	Write(rec, http.StatusOK, ports.CallView{CreatedAt: at})
	if !strings.Contains(rec.Body.String(), `"created_at":"2026-09-25 10:12:35"`) {
		t.Fatal(rec.Body.String())
	}
	var out ports.CallView
	res := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(rec.Body.String()))}
	if err := Decode(res, &out); err != nil || !out.CreatedAt.Equal(at) {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}
func TestDecodePeers(t *testing.T) {
	for _, body := range []string{`{"code":"OK","message":"成功","data":{"id":"one"},"request_id":"r"}`, `{"id":"one"}`} {
		var out struct {
			ID string `json:"id"`
		}
		res := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}
		if err := Decode(res, &out); err != nil || out.ID != "one" {
			t.Fatalf("out=%+v err=%v", out, err)
		}
	}
	res := &http.Response{StatusCode: 409, Body: io.NopCloser(strings.NewReader(`{"code":"AGENT_BUSY","error":"conflict","message":"忙","data":null,"request_id":"r"}`))}
	var api *errs.APIError
	if err := Decode(res, nil); !errors.As(err, &api) || api.HTTP != 409 || api.Code != "AGENT_BUSY" {
		t.Fatalf("error=%v", err)
	}
	res = &http.Response{StatusCode: 502, Body: io.NopCloser(strings.NewReader("bad gateway"))}
	if err := Decode(res, nil); !errors.As(err, &api) || api.Message == "" {
		t.Fatalf("error=%v", err)
	}
}

func TestRequestIDAcceptsAndReturnsTraceID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "request-1")
	req.Header.Set("X-Trace-ID", "trace-1")
	rec := httptest.NewRecorder()
	RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ID(r.Context()) != "request-1" || TraceID(r.Context()) != "trace-1" {
			t.Fatalf("context ids request=%q trace=%q", ID(r.Context()), TraceID(r.Context()))
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)
	if rec.Header().Get("X-Request-ID") != "request-1" || rec.Header().Get("X-Trace-ID") != "trace-1" {
		t.Fatalf("headers: %v", rec.Header())
	}
}
