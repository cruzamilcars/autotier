package mcp

// Servidor MCP (JSON-RPC 2.0 por stdio, una linea por mensaje) que expone
// autotier a cualquier harness (Claude Code, OpenCode, Cursor, Cline...).
// Solo stdlib. Los logs van a stderr: stdout es solo protocolo.
//
// Tools:
// - route_decide: decide tier SIN gastar (puro: classifier+policy). Explica.
// - agent_call: ejecuta un agente nombrado (mock o upstream segun config).
// - cost_status: resumen del log (ahorro, decisores, calidad).

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/cruzamilcars/autotier/internal/agents"
	"github.com/cruzamilcars/autotier/internal/catalog"
	"github.com/cruzamilcars/autotier/internal/classifier"
	"github.com/cruzamilcars/autotier/internal/policy"
	"github.com/cruzamilcars/autotier/internal/proxy"
	"github.com/cruzamilcars/autotier/internal/store"
)

const protocolVersion = "2024-11-05"

type Server struct {
	Proxy proxy.Config
}

func New(cfg proxy.Config) *Server { return &Server{Proxy: cfg} }

// Serve corre el loop NDJSON. Termina en EOF.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	out := bufio.NewWriter(w)
	defer out.Flush()
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if resp := s.handleLine(line); resp != nil {
			out.Write(resp)
			out.WriteByte('\n')
			out.Flush()
		}
	}
	return sc.Err()
}

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func errResp(id any, code int, msg string) []byte {
	b, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id,
		"error": map[string]any{"code": code, "message": msg},
	})
	return b
}

func okResp(id any, result any) []byte {
	b, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id, "result": result,
	})
	return b
}

func textResult(text string) any {
	return map[string]any{"content": []map[string]string{{"type": "text", "text": text}}}
}

func (s *Server) handleLine(line []byte) []byte {
	var req rpcReq
	if err := json.Unmarshal(line, &req); err != nil {
		b, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": nil,
			"error": map[string]any{"code": -32700, "message": "parse error"},
		})
		return b
	}
	// Notificaciones (sin id string/num o metodo notification/): sin respuesta.
	if req.Method == "notifications/initialized" || req.Method == "notifications/cancelled" {
		return nil
	}
	switch req.Method {
	case "initialize":
		return okResp(req.ID, map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "autotier", "version": "0.1.0"},
		})
	case "ping":
		return okResp(req.ID, map[string]any{})
	case "tools/list":
		return okResp(req.ID, map[string]any{"tools": toolDefs()})
	case "tools/call":
		return s.handleCall(req.ID, req.Params)
	default:
		return errResp(req.ID, -32601, "method not found: "+req.Method)
	}
}

func toolDefs() []any {
	obj := func(props map[string]any, required []string) any {
		return map[string]any{"type": "object", "properties": props, "required": required}
	}
	str := map[string]any{"type": "string"}
	return []any{
		map[string]any{
			"name":        "route_decide",
			"description": "Decide que tier (haiku-tier|balanced-tier|frontier-tier) merece un prompt, sin gastar nada. Explica complejidad, dominio, elegibles, costo estimado y ahorro vs frontera.",
			"inputSchema": obj(map[string]any{
				"prompt":   str,
				"mode":     map[string]any{"type": "string", "enum": []string{"cost", "balance", "intelligence"}},
				"deny":     map[string]any{"type": "string", "description": "tiers vetados, ej. frontier-tier"},
				"min_tier": map[string]any{"type": "string"},
				"max_cost": map[string]any{"type": "number"},
			}, []string{"prompt"}),
		},
		map[string]any{
			"name":        "agent_call",
			"description": "Ejecuta un agente nombrado (@explore|@build|@judge|@orchestrator o custom de agents/). Acepta @menciones para delegar si el agente lo permite.",
			"inputSchema": obj(map[string]any{
				"agent": str, "task": str,
				"context": map[string]any{"type": "string", "description": "handoff de conversacion previa"},
			}, []string{"agent", "task"}),
		},
		map[string]any{
			"name":        "cost_status",
			"description": "Resumen de gasto del log: ahorro vs todo-frontera, escalations, quality pass, por tier/agente/decisor.",
			"inputSchema": obj(map[string]any{}, []string{}),
		},
	}
}

type callParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func strArg(a map[string]any, k string) string {
	if v, ok := a[k].(string); ok {
		return v
	}
	return ""
}

func numArg(a map[string]any, k string) float64 {
	if v, ok := a[k].(float64); ok {
		return v
	}
	return 0
}

func (s *Server) handleCall(id any, raw json.RawMessage) []byte {
	var p callParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return errResp(id, -32602, "invalid params")
	}
	if p.Arguments == nil {
		p.Arguments = map[string]any{}
	}
	switch p.Name {
	case "route_decide":
		return okResp(id, textResult(s.routeDecide(p.Arguments)))
	case "agent_call":
		text, isErr := s.agentCall(p.Arguments)
		if isErr {
			return errResp(id, -32602, text)
		}
		return okResp(id, textResult(text))
	case "cost_status":
		return okResp(id, textResult(s.costStatus()))
	default:
		return errResp(id, -32602, "unknown tool: "+p.Name)
	}
}

func (s *Server) routeDecide(a map[string]any) string {
	prompt := strArg(a, "prompt")
	if strings.TrimSpace(prompt) == "" {
		return "error: prompt vacio"
	}
	mode := policy.Mode(strArg(a, "mode"))
	if mode == "" {
		mode = policy.ModeCost
	}
	deny, err := policy.ParseDeny(strArg(a, "deny"))
	if err != nil {
		return "error: " + err.Error()
	}
	minTier := strArg(a, "min_tier")
	if minTier != "" {
		if t, ok := policy.NormalizeTier(minTier); ok {
			minTier = t
		} else {
			return "error: min_tier desconocido: " + minTier
		}
	}
	complexity := classifier.Score(prompt, false, strings.Contains(strings.ToLower(prompt), "screenshot"), 0)
	domain := classifier.DetectDomain(prompt)
	dec, err := policy.DecideWith(complexity, domain, mode, minTier, deny, numArg(a, "max_cost"), 800, 512)
	if err != nil {
		return "error: " + err.Error()
	}
	models := catalog.Default()
	m := catalog.ByID(models, dec.Tier)
	cost := 0.0
	if m != nil {
		cost = catalog.CostFor(*m, 800, 512)
	}
	frontier := catalog.FrontierCost(800, 512)
	sav := 0.0
	if frontier > 0 {
		sav = 100 * (frontier - cost) / frontier
	}
	var elig []string
	for _, em := range catalog.Eligible(models, domain, dec.MinScore) {
		elig = append(elig, fmt.Sprintf("%s(%d)", em.ID, em.ScoreFor(domain)))
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "tier=%s complexity=%.2f domain=%s mode=%s floor=%d\n", dec.Tier, complexity, domain, dec.Mode, dec.MinScore)
	fmt.Fprintf(&sb, "elegibles: %s\n", strings.Join(elig, ", "))
	fmt.Fprintf(&sb, "costo_est=$%.6f ahorro_vs_frontera=%.0f%% budget_exceeded=%v\n", cost, sav, dec.BudgetExceeded)
	fmt.Fprintf(&sb, "nota: decision sin gastar (dry-run). max_steps=%d", dec.MaxSteps)
	return sb.String()
}

func (s *Server) agentCall(a map[string]any) (string, bool) {
	name := strArg(a, "agent")
	task := strArg(a, "task")
	if name == "" || strings.TrimSpace(task) == "" {
		return "agent y task requeridos", true
	}
	dir := s.Proxy.AgentsDir
	if dir == "" {
		dir = agents.DefaultDir
	}
	reg, err := agents.Load(dir)
	if err != nil {
		return "registry: " + err.Error(), true
	}
	srv := proxy.New(s.Proxy)
	res, err := srv.RunAgent(reg, strings.ToLower(name), task, proxy.AgentCallOpts{
		Context: strArg(a, "context"), MaxTokens: 512, Mode: srvMode(s.Proxy.Mode),
	})
	if err != nil {
		return err.Error(), true
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "@%s [%s] ahorro=%.0f%% esc=%d via=%s\n---\n%s",
		res.Agent, res.Tier, res.SavingsPct, res.Escalations, res.Decider, res.Text)
	for _, sub := range res.Subs {
		fmt.Fprintf(&sb, "\n[sub @%s %s]", sub.Agent, sub.Tier)
	}
	return sb.String(), false
}

func srvMode(m policy.Mode) policy.Mode {
	if m == "" {
		return policy.ModeCost
	}
	return m
}

func (s *Server) costStatus() string {
	path := s.Proxy.LogPath
	if path == "" {
		path = "autotier.log.jsonl"
	}
	recs, err := store.ReadAll(path)
	if err != nil {
		return "error: " + err.Error()
	}
	sum := store.Summarize(recs)
	if sum.Requests == 0 && sum.CacheHits == 0 {
		return "sin datos en " + path + " (aun no hay requests)"
	}
	return fmt.Sprintf("requests=%d ahorro=%.1f%% escalations=%d quality=%.0f%% cache=%d tiers=%v",
		sum.Requests, sum.SavingsPct, sum.Escalations, sum.QualityPassPct, sum.CacheHits, sum.ByTier)
}
