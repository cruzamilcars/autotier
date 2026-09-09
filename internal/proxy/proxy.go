package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cruzamilcars/autotier/internal/agents"
	"github.com/cruzamilcars/autotier/internal/catalog"
	"github.com/cruzamilcars/autotier/internal/classifier"
	"github.com/cruzamilcars/autotier/internal/gate"
	"github.com/cruzamilcars/autotier/internal/learn"
	"github.com/cruzamilcars/autotier/internal/policy"
	"github.com/cruzamilcars/autotier/internal/store"
)

// Config del gateway.
type Config struct {
	UpstreamURL    string // ej http://localhost:11434/v1 ; vacio = mock
	UpstreamAPIKey string // Bearer para upstreams con auth (o env AUTOTIER_API_KEY)
	Mock           bool
	LogPath        string
	Mode           policy.Mode
	MinTier        string
	MaxCostUSD     float64
	AgentsDir      string // registry de agentes nombrados ("agents" por defecto)
	LearnPath      string // estado bandit JSON (vacio = solo reglas)
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens"`
	TestsFailed bool          `json:"tests_failed,omitempty"`
	Agent       string        `json:"agent,omitempty"`
}

type chatChoice struct {
	Index   int         `json:"index"`
	Message chatMessage `json:"message"`
}

type chatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Choices []chatChoice `json:"choices"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Router struct {
		Tier           string  `json:"tier"`
		Provider       string  `json:"provider"`
		Model          string  `json:"model"`
		Domain         string  `json:"domain"`
		Mode           string  `json:"mode"`
		Complexity     float64 `json:"complexity"`
		Escalations    int     `json:"escalations"`
		CostUSD        float64 `json:"cost_usd"`
		SavingsPct     float64 `json:"savings_pct"`
		BudgetExceeded bool    `json:"budget_exceeded"`
		Agent          string  `json:"agent,omitempty"`
		Decider        string  `json:"decider,omitempty"`
	} `json:"autotier"`
}

// Server expone gateway OpenAI-compatible + health.
type Server struct {
	cfg     Config
	cache   map[string]chatResponse
	mu      sync.Mutex
	learn   *learn.State
	learnMu sync.Mutex
	rng     *rand.Rand
}

func New(cfg Config) *Server {
	if cfg.LogPath == "" {
		cfg.LogPath = "autotier.log.jsonl"
	}
	if cfg.Mode == "" {
		cfg.Mode = policy.ModeCost
	}
	s := &Server{cfg: cfg, cache: map[string]chatResponse{}, rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
	if cfg.LearnPath != "" {
		if st, err := learn.Load(cfg.LearnPath); err == nil {
			s.learn = st
		} else {
			s.learn = learn.New()
		}
	}
	return s
}

// decideTier aplica el bandit si hay estado y aprendizaje permitido.
// El pin de agente fijo siempre gana sobre learn.
func (s *Server) decideTier(dec policy.Decision, domain string, allowLearn bool) (tier, decider string) {
	if s.learn == nil || !allowLearn {
		return dec.Tier, "rules"
	}
	s.learnMu.Lock()
	defer s.learnMu.Unlock()
	eligible := catalog.Eligible(catalog.Default(), domain, dec.MinScore)
	out := s.learn.Override(learn.Input{
		RulesTier: dec.Tier, Eligible: eligible,
		Complexity: dec.Complexity, Domain: domain, RNG: s.rng,
	})
	if out.Reason == "rules-cold" || out.Reason == "rules-unranked" || out.Reason == "rules-ok" {
		return out.Tier, "rules"
	}
	return out.Tier, "learn:" + out.Reason
}

// observeLearn registra el outcome en el bandit y persiste.
// El tier inicial aprende de si basto (sin escalacion + pass);
// el tier final aprende de si resolvio (pass).
func (s *Server) observeLearn(initialTier, finalTier, domain string, esc int, pass bool) {
	if s.learn == nil || s.cfg.LearnPath == "" {
		return
	}
	s.learnMu.Lock()
	defer s.learnMu.Unlock()
	if initialTier == finalTier {
		s.learn.Observe(initialTier, domain, pass)
	} else {
		s.learn.Observe(initialTier, domain, esc == 0 && pass)
		s.learn.Observe(finalTier, domain, pass)
	}
	_ = s.learn.Save(s.cfg.LearnPath)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/v1/chat/completions", s.handleChat)
	mux.HandleFunc("/v1/messages", s.handleAnthropic)
	return mux
}

func lastUserText(ms []chatMessage) string {
	for i := len(ms) - 1; i >= 0; i-- {
		if ms[i].Role == "user" {
			return ms[i].Content
		}
	}
	if len(ms) > 0 {
		return ms[len(ms)-1].Content
	}
	return ""
}

func cacheKey(model, agent, text string) string {
	h := sha256.Sum256([]byte(model + "\x00" + agent + "\x00" + text))
	return hex.EncodeToString(h[:])
}

// estimateTokens ~ len/4, suficiente para presupuesto y ventana en MVP.
func estimateTokens(text string) int {
	n := len(text) / 4
	if n < 8 {
		n = 8
	}
	return n
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	var req chatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	forceFail := r.Header.Get("X-Autotier-Force") == "fail-first"

	prompt := lastUserText(req.Messages)

	// Via agentes nombrados: campo "agent", header X-Autotier-Agent,
	// o @menciones (orquestador por defecto). Incluye delegacion.
	agentName := req.Agent
	if agentName == "" {
		agentName = r.Header.Get("X-Autotier-Agent")
	}
	if agentName == "" && len(agents.Mentions(prompt)) > 0 {
		agentName = "orchestrator"
	}
	if agentName != "" {
		s.serveAgent(w, r, agentName, req, prompt, forceFail)
		return
	}

	hasImage := strings.Contains(strings.ToLower(prompt), "[image]") ||
		strings.Contains(strings.ToLower(prompt), "screenshot")
	complexity := classifier.Score(prompt, false, hasImage, len(req.Messages))
	domain := classifier.DetectDomain(prompt)

	// Cache exacta: mismo modelo+prompt ahorra quota free.
	key := cacheKey(req.Model, "", prompt)
	s.mu.Lock()
	if hit, ok := s.cache[key]; ok {
		s.mu.Unlock()
		writeJSON(w, hit)
		return
	}
	s.mu.Unlock()

	estIn := estimateTokens(prompt)
	estOut := req.MaxTokens
	if estOut <= 0 {
		estOut = 512
	}
	dec := policy.Decide(complexity, domain, s.cfg.Mode, s.cfg.MinTier, s.cfg.MaxCostUSD, estIn, estOut)
	tier, decider := s.decideTier(dec, domain, true)

	start := time.Now()
	text, tier, prov, escalations, err := s.execute(tier, prompt, estOut, forceFail, req.TestsFailed)
	if err != nil {
		if strings.Contains(err.Error(), "no healthy provider") {
			http.Error(w, err.Error(), http.StatusBadGateway)
		} else {
			http.Error(w, "upstream: "+err.Error(), http.StatusBadGateway)
		}
		return
	}
	latency := time.Since(start)

	final := catalog.ByID(catalog.Default(), tier)
	inT := estIn
	outT := estimateTokens(text)
	cost := 0.0
	if final != nil {
		cost = catalog.CostFor(*final, inT, outT)
	}
	frontier := catalog.FrontierCost(inT, outT)
	savings := 0.0
	if frontier > 0 {
		savings = 100 * (frontier - cost) / frontier
	}

	resp := chatResponse{ID: "chatcmpl-autotier", Object: "chat.completion"}
	resp.Choices = []chatChoice{{Index: 0, Message: chatMessage{Role: "assistant", Content: text}}}
	resp.Usage.PromptTokens = inT
	resp.Usage.CompletionTokens = outT
	resp.Usage.TotalTokens = inT + outT
	resp.Router.Tier = tier
	if prov != nil {
		resp.Router.Provider = prov.Provider
		resp.Router.Model = prov.Model
	}
	resp.Router.Domain = domain
	resp.Router.Mode = string(dec.Mode)
	resp.Router.Complexity = dec.Complexity
	resp.Router.Escalations = escalations
	resp.Router.CostUSD = cost
	resp.Router.SavingsPct = savings
	resp.Router.BudgetExceeded = dec.BudgetExceeded
	resp.Router.Decider = decider

	pass := gate.Check(text, req.TestsFailed).Pass
	_ = store.Append(s.cfg.LogPath, store.Record{
		Tier: tier, Provider: resp.Router.Provider, Model: resp.Router.Model,
		Domain: domain, Mode: string(dec.Mode),
		InTokens: inT, OutTokens: outT, CostUSD: cost,
		LatencyMs:   float64(latency.Milliseconds()),
		Escalations: escalations,
		QualityPass: pass,
		Decider:     decider,
	})
	s.observeLearn(dec.Tier, tier, domain, escalations, pass)

	s.mu.Lock()
	s.cache[key] = resp
	s.mu.Unlock()
	writeJSON(w, resp)
}

// complete llama al upstream o al mock determinista.
func (s *Server) complete(tier, model, prompt string, outTokens int, forcePoor bool) (string, error) {
	if s.cfg.Mock || s.cfg.UpstreamURL == "" {
		return mockComplete(tier, prompt, forcePoor), nil
	}
	upstream := strings.TrimRight(s.cfg.UpstreamURL, "/") + "/chat/completions"
	payload := map[string]any{
		"model":      model,
		"max_tokens": outTokens,
		"messages":   []map[string]string{{"role": "user", "content": prompt}},
	}
	b, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 120 * time.Second}
	upReq, err := http.NewRequest(http.MethodPost, upstream, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	upReq.Header.Set("Content-Type", "application/json")
	if s.cfg.UpstreamAPIKey != "" {
		upReq.Header.Set("Authorization", "Bearer "+s.cfg.UpstreamAPIKey)
	}
	resp, err := client.Do(upReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("upstream %d: %s", resp.StatusCode, truncate(string(rb), 300))
	}
	// Intenta forma OpenAI; si no, devuelve texto crudo.
	var parsed chatResponse
	if err := json.Unmarshal(rb, &parsed); err == nil && len(parsed.Choices) > 0 {
		return parsed.Choices[0].Message.Content, nil
	}
	var generic struct {
		Content string `json:"content"`
		Text    string `json:"text"`
	}
	if err := json.Unmarshal(rb, &generic); err == nil && (generic.Content != "" || generic.Text != "") {
		return generic.Content + generic.Text, nil
	}
	return string(rb), nil
}

// mockComplete determinista para demos/tests sin API keys.
func mockComplete(tier, prompt string, forcePoor bool) string {
	if forcePoor {
		return "No estoy seguro, podria ser varias cosas."
	}
	short := truncate(prompt, 60)
	switch tier {
	case catalog.TierHaiku:
		return "Hallazgos: " + short + " — ver archivos citados. Sin cambios propuestos."
	case catalog.TierFrontier:
		return "Plan frontera:\n1) Alcance y riesgos\n2) Diseno por capas\n3) Migracion por pasos con rollback\n4) Tests + observabilidad\nContexto: " + short
	default:
		return "Implementacion:\n- Cambio acotado por pasos\n- Tests que lo cubren\n- Verificacion local\nContexto: " + short
	}
}

// handleAnthropic acepta /v1/messages y lo traduce al flujo interno,
// respondiendo en forma Anthropic minima. Suficiente para Cline/Windsurf/Cursor.
func (s *Server) handleAnthropic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		Agent     string `json:"agent,omitempty"`
		Messages  []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	b, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err := json.Unmarshal(b, &in); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	msgs := make([]chatMessage, 0, len(in.Messages))
	for _, m := range in.Messages {
		msgs = append(msgs, chatMessage{Role: m.Role, Content: m.Content})
	}
	// Reusa el handler OpenAI construyendo un sub-request.
	sub, _ := json.Marshal(chatRequest{Model: in.Model, Messages: msgs, MaxTokens: in.MaxTokens, Agent: in.Agent})
	r2, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(sub))
	r2.Header = r.Header
	rec := &captureWriter{header: http.Header{}}
	s.handleChat(rec, r2)
	if rec.status >= 400 {
		http.Error(w, rec.body.String(), rec.status)
		return
	}
	var cr chatResponse
	if err := json.Unmarshal(rec.body.Bytes(), &cr); err != nil || len(cr.Choices) == 0 {
		http.Error(w, "autotier error", http.StatusBadGateway)
		return
	}
	out := map[string]any{
		"id":   "msg_autotier",
		"type": "message",
		"role": "assistant",
		"content": []map[string]string{
			{"type": "text", "text": cr.Choices[0].Message.Content},
		},
		"model":         cr.Router.Model,
		"usage":         map[string]int{"input_tokens": cr.Usage.PromptTokens, "output_tokens": cr.Usage.CompletionTokens},
		"autotier_tier": cr.Router.Tier,
	}
	writeJSON(w, out)
}

type captureWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (c *captureWriter) Header() http.Header { return c.header }
func (c *captureWriter) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = 200
	}
	return c.body.Write(b)
}
func (c *captureWriter) WriteHeader(s int) { c.status = s }

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
