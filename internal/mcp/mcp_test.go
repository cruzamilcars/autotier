package mcp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cruzamilcars/autotier/internal/proxy"
)

func testSrv() *Server {
	return New(proxy.Config{Mock: true, LogPath: "test-mcp.log.jsonl", AgentsDir: "test-agents-none", Mode: "cost"})
}

func call(t *testing.T, s *Server, line string) map[string]any {
	t.Helper()
	resp := s.handleLine([]byte(line))
	if resp == nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(resp, &out); err != nil {
		t.Fatalf("respuesta no JSON: %v", err)
		return nil
	}
	return out
}

func TestInitialize(t *testing.T) {
	s := testSrv()
	out := call(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	res := out["result"].(map[string]any)
	if res["protocolVersion"] != protocolVersion {
		t.Fatalf("protocolVersion mal: %+v", res)
	}
	info := res["serverInfo"].(map[string]any)
	if info["name"] != "autotier" {
		t.Fatalf("serverInfo mal: %+v", info)
	}
}

func TestToolsList(t *testing.T) {
	s := testSrv()
	out := call(t, s, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	tools := out["result"].(map[string]any)["tools"].([]any)
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.(map[string]any)["name"].(string)] = true
	}
	for _, want := range []string{"route_decide", "agent_call", "cost_status"} {
		if !names[want] {
			t.Fatalf("falta tool %s", want)
		}
	}
}

func textOf(t *testing.T, out map[string]any) string {
	t.Helper()
	res, ok := out["result"].(map[string]any)
	if !ok {
		t.Fatalf("sin result (error?): %+v", out)
	}
	content := res["content"].([]any)
	return content[0].(map[string]any)["text"].(string)
}

func TestRouteDecide(t *testing.T) {
	s := testSrv()
	out := call(t, s, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"route_decide","arguments":{"prompt":"busca donde se define login"}}}`)
	txt := textOf(t, out)
	if !strings.Contains(txt, "tier=haiku-tier") {
		t.Fatalf("debió rutear a haiku:\n%s", txt)
	}
	if !strings.Contains(txt, "ahorro_vs_frontera") {
		t.Fatalf("sin numeros:\n%s", txt)
	}
	// veto con typo falla cerrado
	out2 := call(t, s, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"route_decide","arguments":{"prompt":"hola","deny":"haikku"}}}`)
	if _, hasErr := out2["error"]; !hasErr {
		if !strings.Contains(textOf(t, out2), "error:") {
			t.Fatalf("typo en deny debe fallar: %+v", out2)
		}
	}
	// veto total falla cerrado
	out3 := call(t, s, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"route_decide","arguments":{"prompt":"hola","deny":"haiku-tier,balanced-tier,frontier-tier"}}}`)
	txt3 := textOf(t, out3)
	if !strings.Contains(txt3, "error:") {
		t.Fatalf("deny total debe fallar cerrado:\n%s", txt3)
	}
}

func TestAgentCall(t *testing.T) {
	s := testSrv()
	out := call(t, s, `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"agent_call","arguments":{"agent":"explore","task":"lista archivos"}}}`)
	txt := textOf(t, out)
	if !strings.Contains(txt, "@explore") {
		t.Fatalf("sin agente:\n%s", txt)
	}
	// agente inexistente = error protocolo
	out2 := call(t, s, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"agent_call","arguments":{"agent":"nadie","task":"x"}}}`)
	if _, hasErr := out2["error"]; !hasErr {
		t.Fatalf("agente inexistente debe dar error: %+v", out2)
	}
}

func TestCostStatusEmpty(t *testing.T) {
	s := New(proxy.Config{Mock: true, LogPath: filepath.Join(t.TempDir(), "empty.log.jsonl"), AgentsDir: "test-agents-none", Mode: "cost"})
	out := call(t, s, `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"cost_status","arguments":{}}}`)
	txt := textOf(t, out)
	if !strings.Contains(txt, "sin datos") {
		t.Fatalf("log inexistente debe decirlo: %s", txt)
	}
}

func TestUnknowns(t *testing.T) {
	s := testSrv()
	out := call(t, s, `{"jsonrpc":"2.0","id":9,"method":"tools/eat","params":{}}`)
	if _, hasErr := out["error"]; !hasErr {
		t.Fatalf("metodo desconocido debe dar error")
	}
	// notificacion: sin respuesta
	if resp := s.handleLine([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)); resp != nil {
		t.Fatalf("notificacion debe callar")
	}
	// linea rota: parse error con respuesta
	if resp := s.handleLine([]byte(`{no json`)); resp == nil {
		t.Fatalf("parse error debe responder")
	}
}
