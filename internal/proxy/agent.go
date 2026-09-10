package proxy

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cruzamilcars/autotier/internal/agents"
	"github.com/cruzamilcars/autotier/internal/catalog"
	"github.com/cruzamilcars/autotier/internal/classifier"
	"github.com/cruzamilcars/autotier/internal/gate"
	"github.com/cruzamilcars/autotier/internal/policy"
	"github.com/cruzamilcars/autotier/internal/provider"
	"github.com/cruzamilcars/autotier/internal/store"
)

// AgentCallOpts controla una invocacion nombrada.
type AgentCallOpts struct {
	Context     string // handoff tipo fork: conversacion previa heredada
	Parent      string // quien delego ("" = usuario directo)
	Depth       int    // nivel de anidamiento actual
	MaxTokens   int
	TestsFailed bool
	ForceFail   bool // demo: primer intento pobre para probar cascade
	MinTier     string
	MaxCostUSD  float64
	Mode        policy.Mode
	Deny        []string // tiers vetados (fail closed si no queda alternativa)
	Pin         string   // tier forzado por el usuario (gana a todo salvo deny)
}

type SubCall struct {
	Agent       string
	Tier        string
	Escalations int
	CostUSD     float64
}

type AgentResult struct {
	Agent          string
	Tier           string
	Provider       string
	Model          string
	Domain         string
	Mode           string
	Complexity     float64
	Escalations    int
	CostUSD        float64
	SavingsPct     float64
	BudgetExceeded bool
	QualityPass    bool
	LatencyMs      float64
	Decider        string
	Text           string
	Subs           []SubCall
}

// execute corre el cascade barato-primero para un prompt ya resuelto.
// Extraido del handler para reusarlo en llamadas de agentes.
func (s *Server) execute(tier, prompt string, estOut int, forceFail, testsFailed bool, deny []string) (text, finalTier string, prov *provider.Candidate, escalations int, err error) {
	outTokens := estOut
	// Respetar el max_tokens del cliente (harnesses como OpenCode piden 32k);
	// techo alto solo contra valores absurdos. El gasto se controla con maxCostUSD.
	if outTokens > 65536 {
		outTokens = 65536
	}
	finalTier = tier
	for attempt := 0; attempt <= 2; attempt++ {
		cands := provider.DefaultCandidates(finalTier)
		prov = provider.Pick(cands)
		if prov == nil {
			return "", finalTier, nil, escalations, fmt.Errorf("no healthy provider for tier " + finalTier)
		}
		text, err = s.complete(finalTier, prov.Model, prompt, outTokens, attempt == 0 && forceFail)
		if err != nil {
			// Circuit-breaker minimal: si falla el upstream, probar siguiente rung.
			// El cascade nunca entra a un tier vetado: ahi termina (best effort).
			if next, ok := escalateAllowed(finalTier, deny); ok {
				finalTier = next
				escalations++
				continue
			}
			return "", finalTier, prov, escalations, err
		}
		if g := gate.Check(text, testsFailed); !g.Pass {
			if next, ok := escalateAllowed(finalTier, deny); ok && attempt < 2 {
				finalTier = next
				escalations++
				continue
			}
		}
		break
	}
	return text, finalTier, prov, escalations, nil
}

// escalateAllowed sube un rung salvo que este vetado (deny) o sea el tope.
func escalateAllowed(tier string, deny []string) (string, bool) {
	next, ok := policy.Escalate(tier)
	if !ok {
		return tier, false
	}
	for _, d := range deny {
		if d == next {
			return tier, false
		}
	}
	return next, true
}

// RunAgent ejecuta un agente nombrado con delegacion @menciones.
// - `task` puede contener @otro: sub-llamada con ese agente (allowlist delegates).
// - `opts.Context` es handoff fork: el agente hereda conversacion sin re-explicar.
// - Profundidad limitada por MaxDepth del agente y MaxDepthCap global.
func (s *Server) RunAgent(reg map[string]agents.Agent, name, task string, o AgentCallOpts) (AgentResult, error) {
	var res AgentResult
	a, ok := reg[strings.ToLower(name)]
	if !ok {
		return res, fmt.Errorf("agente desconocido %q (disponibles: %s)", name, strings.Join(agents.Names(reg), ", "))
	}
	if o.Depth > agents.MaxDepthCap {
		return res, fmt.Errorf("profundidad maxima %d superada en @%s", agents.MaxDepthCap, a.Name)
	}
	// Deny global del servidor como default (el veto por llamada manda si viene).
	deny := o.Deny
	if len(deny) == 0 {
		deny = s.cfg.Deny
	}
	mode := o.Mode
	if mode == "" {
		mode = s.cfg.Mode
	}
	if mode == "" {
		mode = policy.ModeCost
	}

	clean := agents.StripMentions(task)
	if strings.TrimSpace(clean) == "" {
		clean = task // solo menciones: el contexto manda
	}

	// 1. Delegacion: cada @mencion corre como sub-agente con la misma tarea.
	var ctxParts []string
	if o.Context != "" {
		ctxParts = append(ctxParts, "[handoff de "+orParent(o.Parent)+"]\n"+o.Context)
	}
	for _, m := range agents.Mentions(task) {
		sub, ok := reg[m]
		if !ok {
			return res, fmt.Errorf("@%s no existe (desde @%s; disponibles: %s)", m, a.Name, strings.Join(agents.Names(reg), ", "))
		}
		if !a.CanDelegate(m) {
			return res, fmt.Errorf("@%s no puede delegar a @%s (delegates: [%s])", a.Name, m, strings.Join(a.Delegates, ", "))
		}
		if o.Depth >= a.MaxDepth {
			return res, fmt.Errorf("@%s llego a max_depth=%d, no puede llamar a @%s", a.Name, a.MaxDepth, m)
		}
		subRes, err := s.RunAgent(reg, sub.Name, clean, AgentCallOpts{
			Parent: a.Name, Depth: o.Depth + 1,
			MaxTokens: o.MaxTokens, TestsFailed: o.TestsFailed,
			MinTier: o.MinTier, MaxCostUSD: o.MaxCostUSD, Mode: mode,
			Deny: deny,
		})
		if err != nil {
			return res, err
		}
		res.Subs = append(res.Subs, SubCall{Agent: sub.Name, Tier: subRes.Tier, Escalations: subRes.Escalations, CostUSD: subRes.CostUSD})
		ctxParts = append(ctxParts, "[resultado de @"+sub.Name+" ("+subRes.Tier+")]\n"+subRes.Text)
	}

	// 2. Clasificacion sobre la tarea limpia.
	hasImage := strings.Contains(strings.ToLower(clean), "[image]") ||
		strings.Contains(strings.ToLower(clean), "screenshot")
	complexity := classifier.Score(clean, false, hasImage, len(ctxParts))
	domain := classifier.DetectDomain(clean)

	// 3. Tier: pin del usuario > fijo del agente > policy(auto) > learn.
	// El pin gana a todo salvo deny (fail closed). Lo pineado no entrena.
	minTier := o.MinTier
	dec := a.ResolveTier(complexity, domain, mode, o.MaxCostUSD, 800, 512)
	tier := dec.Tier
	decider := ""
	if o.Pin != "" {
		pin, ok := policy.NormalizeTier(o.Pin)
		if !ok {
			return res, fmt.Errorf("pin desconocido: %q", o.Pin)
		}
		for _, d := range deny {
			if d == pin {
				return res, fmt.Errorf("pin %s esta en deny %v", pin, deny)
			}
		}
		tier = pin
		dec.Tier = pin
		decider = "pin"
	} else {
		if minTier != "" {
			dec2 := policy.Decide(complexity, domain, mode, minTier, o.MaxCostUSD, 800, 512)
			tier = dec2.Tier
			dec.BudgetExceeded = dec2.BudgetExceeded
		}
		if len(deny) > 0 {
			dec2, err := policy.DecideWith(complexity, domain, mode, minTier, deny, o.MaxCostUSD, 800, 512)
			if err != nil {
				return res, err
			}
			// DecideWith respeta minTier y deny; solo aplica si el agente es auto
			// (el pin del agente fijo ya se valido abajo).
			if a.Tier == "auto" || a.Tier == "" {
				tier = dec2.Tier
				dec.BudgetExceeded = dec2.BudgetExceeded
			}
		}
		dec.Tier = tier
		// El pin del agente gana sobre learn; auto sí aprende.
		pinned := a.Tier != "auto" && a.Tier != ""
		if pinned {
			for _, d := range deny {
				if d == tier {
					return res, fmt.Errorf("@%s corre en %s pero esta en deny %v", a.Name, tier, deny)
				}
			}
		}
		tier, decider = s.decideTier(dec, domain, !pinned)
	}

	// 4. Prompt efectivo: system del agente + contexto + tarea.
	var sb strings.Builder
	if a.System != "" {
		sb.WriteString("[system @" + a.Name + "]\n" + a.System + "\n\n")
	}
	for _, c := range ctxParts {
		sb.WriteString(c + "\n\n")
	}
	sb.WriteString("[tarea]\n" + clean)
	fullPrompt := sb.String()

	estIn := estimateTokens(fullPrompt)
	estOut := o.MaxTokens
	if estOut <= 0 {
		estOut = 512
	}

	start := time.Now()
	text, finalTier, prov, esc, err := s.execute(tier, fullPrompt, estOut, o.ForceFail, o.TestsFailed, deny)
	if err != nil {
		return res, err
	}
	latency := time.Since(start)

	models := catalog.Default()
	final := catalog.ByID(models, finalTier)
	outT := estimateTokens(text)
	cost := 0.0
	if final != nil {
		cost = catalog.CostFor(*final, estIn, outT)
	}
	frontier := catalog.FrontierCost(estIn, outT)
	savings := 0.0
	if frontier > 0 {
		savings = 100 * (frontier - cost) / frontier
	}
	qpass := gate.Check(text, o.TestsFailed).Pass

	_ = store.Append(s.cfg.LogPath, store.Record{
		Tier: finalTier, Provider: prov.Provider, Model: prov.Model,
		Domain: domain, Mode: string(mode),
		InTokens: estIn, OutTokens: outT, CostUSD: cost,
		LatencyMs:   float64(latency.Milliseconds()),
		Escalations: esc, QualityPass: qpass,
		Agent: a.Name, Parent: o.Parent, Decider: decider,
	})
	// Lo pineado por el usuario no entrena al bandit (no fue decision del router).
	if decider != "pin" {
		s.observeLearn(dec.Tier, finalTier, domain, esc, qpass)
	}

	res.Agent = a.Name
	res.Tier = finalTier
	res.Provider = prov.Provider
	res.Model = prov.Model
	res.Domain = domain
	res.Mode = string(mode)
	res.Complexity = complexity
	res.Escalations = esc
	res.CostUSD = cost
	res.SavingsPct = savings
	res.BudgetExceeded = dec.BudgetExceeded
	res.QualityPass = qpass
	res.LatencyMs = float64(latency.Milliseconds())
	res.Decider = decider
	res.Text = text
	return res, nil
}

func orParent(p string) string {
	if p == "" {
		return "usuario"
	}
	return "@" + p
}

// serveAgent atiende la via de agentes nombrados sobre HTTP.
func (s *Server) serveAgent(w http.ResponseWriter, r *http.Request, agentName string, req chatRequest, prompt string, forceFail bool) {
	dir := s.cfg.AgentsDir
	if dir == "" {
		dir = agents.DefaultDir
	}
	reg, err := agents.Load(dir)
	if err != nil {
		http.Error(w, "registry: "+err.Error(), http.StatusInternalServerError)
		return
	}
	maxT := req.MaxTokens
	if maxT <= 0 {
		maxT = 512
	}
	deny, denyErr := policy.ParseDeny(firstNonEmpty(r.Header.Get("X-Autotier-Deny"), req.Deny, strings.Join(s.cfg.Deny, ",")))
	if denyErr != nil {
		http.Error(w, denyErr.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.RunAgent(reg, agentName, prompt, AgentCallOpts{
		MaxTokens: maxT, TestsFailed: req.TestsFailed, ForceFail: forceFail,
		MinTier: s.cfg.MinTier, MaxCostUSD: s.cfg.MaxCostUSD, Mode: s.cfg.Mode,
		Deny: deny, Pin: firstNonEmpty(r.Header.Get("X-Autotier-Pin"), req.Pin),
	})
	if err != nil {
		code := http.StatusBadRequest
		if strings.Contains(err.Error(), "no healthy") || strings.Contains(err.Error(), "upstream") {
			code = http.StatusBadGateway
		}
		http.Error(w, err.Error(), code)
		return
	}
	out := chatResponse{ID: "chatcmpl-autotier-agent", Object: "chat.completion"}
	out.Created = time.Now().Unix()
	out.Model = res.Model
	out.Choices = []chatChoice{{Index: 0, Message: chatMessage{Role: "assistant", Content: res.Text}, FinishReason: "stop"}}
	out.Usage.PromptTokens = estimateTokens(prompt + res.Text)
	out.Usage.CompletionTokens = estimateTokens(res.Text)
	out.Usage.TotalTokens = out.Usage.PromptTokens + out.Usage.CompletionTokens
	out.Router.Tier = res.Tier
	out.Router.Provider = res.Provider
	out.Router.Model = res.Model
	out.Router.Domain = res.Domain
	out.Router.Mode = res.Mode
	out.Router.Complexity = res.Complexity
	out.Router.Escalations = res.Escalations
	out.Router.CostUSD = res.CostUSD
	out.Router.SavingsPct = res.SavingsPct
	out.Router.BudgetExceeded = res.BudgetExceeded
	out.Router.Agent = res.Agent
	out.Router.Decider = res.Decider

	s.mu.Lock()
	s.cache[cacheKey(req.Model, res.Agent, prompt)] = out
	s.mu.Unlock()
	if req.Stream {
		writeSSE(w, res.Text, out.ID)
		return
	}
	writeJSON(w, out)
}
