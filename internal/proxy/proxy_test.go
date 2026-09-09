package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testServer() *Server {
	return New(Config{Mock: true, LogPath: "test.log.jsonl", AgentsDir: "test-agents-none"})
}

func TestChatFinishReason(t *testing.T) {
	s := testServer()
	body := `{"model":"auto","messages":[{"role":"user","content":"hola"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"finish_reason":"stop"`) {
		t.Fatalf("sin finish_reason stop: %s", rec.Body.String())
	}
}

func TestStreamEndsWithFinishReason(t *testing.T) {
	s := testServer()
	body := `{"model":"auto","stream":true,"messages":[{"role":"user","content":"hola"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	b := rec.Body.String()
	if !strings.Contains(b, `"finish_reason":"stop"`) {
		t.Fatalf("SSE sin finish_reason stop:\n%s", b)
	}
	if !strings.HasSuffix(strings.TrimSpace(b), "data: [DONE]") {
		t.Fatalf("SSE sin cierre [DONE]:\n%s", b)
	}
	// El chunk final debe llegar ANTES de [DONE] (si no, el SDK repite en loop).
	idxFin := strings.Index(b, `"finish_reason":"stop"`)
	idxDone := strings.Index(b, "data: [DONE]")
	if idxFin < 0 || idxDone < 0 || idxFin > idxDone {
		t.Fatalf("orden invalido finish/[DONE]")
	}
}

func TestModelsEndpoint(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"id":"auto"`) {
		t.Fatalf("models mal: %d %s", rec.Code, rec.Body.String())
	}
}
