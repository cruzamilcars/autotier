package agents

// Agentes nombrados: registry portable (Markdown + frontmatter) que define
// quien hace que, con que tier, con que contexto precargado y a quienes
// puede delegar. Misma convencion que Claude Code (.claude/agents/*.md) y
// OpenCode, para que `autotier init` los exporte sin reescribir.
//
// Tres formas de invocar (como Claude Code):
// - natural: el router sugiere por descripcion (list)
// - explicita: `autotier agent call <nombre> "tarea"` o `@nombre` en la tarea
// - sesion completa: el agente aporta system prompt + tier a todo el flujo
//
// Delegacion: `@otro` dentro de la tarea = sub-llamada con ese agente,
// limitada por `delegates` (allowlist) y `max_depth` (default 2).

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cruzamilcars/autotier/internal/catalog"
	"github.com/cruzamilcars/autotier/internal/policy"
)

type Agent struct {
	Name        string
	Description string
	Tier        string // haiku-tier|balanced-tier|frontier-tier|auto
	Delegates   []string
	MaxSteps    int
	MaxDepth    int
	System      string // system prompt + contexto precargado (cuerpo md)
	Source      string // archivo origen ("" = builtin)
}

const (
	DefaultDir  = "agents"
	DefaultTier = "auto"
	MaxDepthCap = 3
)

// Builtins cubren el flujo orquestador->workers->juez sin configurar nada.
func Builtins() []Agent {
	return []Agent{
		{
			Name: "explore", Description: "Exploracion read-only barata: grep, listar, leer logs",
			Tier: catalog.TierHaiku, MaxSteps: 8, MaxDepth: 0,
			System: "Eres un explorador barato. Devuelve rutas + resumen de 5 lineas. Sin edicion. Agrupa 10 lookups por invocacion.",
		},
		{
			Name: "build", Description: "Implementacion y refactor de cambios especificados",
			Tier: catalog.TierBalanced, MaxSteps: 25, MaxDepth: 1, Delegates: []string{"explore"},
			System: "Implementa cambios acotados con tests. Delega busquedas a @explore.",
		},
		{
			Name: "judge", Description: "Juez frontera: arquitectura, seguridad, decisiones ambiguas",
			Tier: catalog.TierFrontier, MaxSteps: 15, MaxDepth: 0,
			System: "Decide el plan final desde resumenes de workers. Explicita riesgos, rollback y tests.",
		},
		{
			Name: "orchestrator", Description: "Orquesta flujos: reparte a workers y sintetiza",
			Tier: "auto", MaxSteps: 25, MaxDepth: 2, Delegates: []string{"explore", "build", "judge"},
			System: "Orquestas el trabajo: delega exploracion a @explore, implementacion a @build y decisiones criticas a @judge. Sintetizas sus resultados.",
		},
	}
}

// Load combina builtins con *.md de dir (los archivos mandan por nombre).
func Load(dir string) (map[string]Agent, error) {
	reg := map[string]Agent{}
	for _, b := range Builtins() {
		reg[b.Name] = b
	}
	if dir == "" {
		return reg, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return reg, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		a, err := ParseFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		if a.Name == "" {
			a.Name = strings.TrimSuffix(e.Name(), ".md")
		}
		reg[a.Name] = a
	}
	return reg, nil
}

// ParseFile lee frontmatter ---...--- + cuerpo.
func ParseFile(path string) (Agent, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Agent{}, err
	}
	a := Agent{Source: path, Tier: DefaultTier, MaxDepth: 2}
	text := string(b)
	rest := text
	if strings.HasPrefix(text, "---") {
		end := strings.Index(text[3:], "---")
		if end >= 0 {
			head := text[3 : 3+end]
			rest = text[3+end+3:]
			for _, line := range strings.Split(head, "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				kv := strings.SplitN(line, ":", 2)
				if len(kv) != 2 {
					continue
				}
				k := strings.TrimSpace(kv[0])
				v := strings.Trim(strings.TrimSpace(kv[1]), `"'`)
				switch k {
				case "name":
					a.Name = v
				case "description":
					a.Description = v
				case "tier", "model":
					a.Tier = normalizeTier(v)
				case "delegates":
					a.Delegates = parseList(v)
				case "max_steps":
					fmt.Sscanf(v, "%d", &a.MaxSteps)
				case "max_depth":
					fmt.Sscanf(v, "%d", &a.MaxDepth)
				}
			}
		}
	}
	a.System = strings.TrimSpace(rest)
	if a.Tier == "" {
		a.Tier = DefaultTier
	}
	if a.MaxDepth > MaxDepthCap {
		a.MaxDepth = MaxDepthCap
	}
	if a.MaxDepth < 0 {
		a.MaxDepth = 0
	}
	if a.MaxSteps <= 0 {
		a.MaxSteps = policyStepsFor(a.Tier)
	}
	return a, nil
}

func normalizeTier(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case catalog.TierHaiku, "haiku":
		return catalog.TierHaiku
	case catalog.TierBalanced, "sonnet":
		return catalog.TierBalanced
	case catalog.TierFrontier, "opus", "frontier":
		return catalog.TierFrontier
	case "auto", "inherit", "":
		return "auto"
	default:
		return v // tier custom: el caller decide (policy lo trata como override)
	}
}

func parseList(v string) []string {
	v = strings.TrimSpace(strings.Trim(v, "[]"))
	if v == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(strings.Trim(p, `"'`))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func policyStepsFor(tier string) int {
	switch tier {
	case catalog.TierHaiku:
		return 8
	case catalog.TierFrontier:
		return 40
	default:
		return 25
	}
}

// Mentions extrae @nombres de un texto, en orden y sin duplicados.
func Mentions(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, tok := range strings.Fields(text) {
		tok = strings.Trim(tok, ".,;:!?()[]{}\"'")
		if len(tok) > 1 && tok[0] == '@' {
			name := strings.ToLower(tok[1:])
			// quita sufijos tipo @explore: o @agent-x
			name = strings.TrimPrefix(name, "agent-")
			name = strings.SplitN(name, ":", 2)[0]
			if name != "" && !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	return out
}

// StripMentions quita los @nombres para clasificar la tarea limpia.
func StripMentions(text string) string {
	var keep []string
	for _, tok := range strings.Fields(text) {
		t := strings.Trim(tok, ".,;:!?()[]{}\"'")
		if len(t) > 1 && t[0] == '@' {
			continue
		}
		keep = append(keep, tok)
	}
	return strings.Join(keep, " ")
}

// CanDelegate valida allowlist: el agente solo llama a sus delegates.
// Orchestrator (delegates con "*") o lista vacia con MaxDepth>0 y sin
// restriccion? No: vacio = nadie (salvo explore/build implicitos? no).
// Regla simple y auditable: solo lo listado.
func (a Agent) CanDelegate(name string) bool {
	for _, d := range a.Delegates {
		if d == name || d == "*" {
			return true
		}
	}
	return false
}

// ResolveTier devuelve el tier efectivo.
//   - Tier fijo (haiku/balanced/frontier): PIN exacto, como `model:` en Claude.
//     La red de seguridad es el cascade (quality-gate escala si falla), no subir
//     preventivamente: el usuario eligio ese costo a proposito.
//   - Tier auto: decision normal por complejidad x capacidad x modo.
func (a Agent) ResolveTier(complexity float64, domain string, mode policy.Mode, maxCostUSD float64, estIn, estOut int) policy.Decision {
	if a.Tier == "auto" || a.Tier == "" {
		return policy.Decide(complexity, domain, mode, "", maxCostUSD, estIn, estOut)
	}
	if mode != policy.ModeBalance && mode != policy.ModeIntelligence {
		mode = policy.ModeCost
	}
	d := policy.Decision{
		Tier: a.Tier, MaxSteps: policyStepsFor(a.Tier),
		MinScore:   policy.QualityFloor(mode),
		Complexity: complexity, Domain: domain, Mode: mode,
	}
	if maxCostUSD > 0 {
		if m := catalog.ByID(catalog.Default(), a.Tier); m != nil {
			d.BudgetExceeded = catalog.CostFor(*m, estIn, estOut) > maxCostUSD
		}
	}
	return d
}

// Names lista ordenada para help y errores.
func Names(reg map[string]Agent) []string {
	out := make([]string, 0, len(reg))
	for n := range reg {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
