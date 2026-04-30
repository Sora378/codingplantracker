package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleRequestRejectsOversizedBody(t *testing.T) {
	p := NewProxy(t.TempDir(), 0)
	body := strings.NewReader(strings.Repeat("x", maxProxyRequestBodyBytes+1))
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	rec := httptest.NewRecorder()

	p.handleRequest(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}
