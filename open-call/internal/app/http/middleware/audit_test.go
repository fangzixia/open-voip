package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuditStatusWriterKeepsFirstStatus(t *testing.T) {
	recorder := httptest.NewRecorder()
	w := &auditStatusWriter{ResponseWriter: recorder}

	w.WriteHeader(http.StatusCreated)
	w.WriteHeader(http.StatusInternalServerError)

	if w.status != http.StatusCreated {
		t.Fatalf("got recorded status %d, want %d", w.status, http.StatusCreated)
	}
	if recorder.Code != http.StatusCreated {
		t.Fatalf("got response status %d, want %d", recorder.Code, http.StatusCreated)
	}
}
