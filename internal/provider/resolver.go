package provider

import (
	"strings"

	"github.com/cruzamilcars/autotier/internal/catalog"
)

// Resolver: mismo modelo en N proveedores -> elige por costo x salud x cuota.
// - free-tier primero si esta sano y con quota (caso OpenCode gratis vs pago).
// - circuit-breaker: el caller marca Healthy=false al fallar (ver proxy).
// - distingue dialectos: kimi-k2 (vLLM) vs k2.5/k2.6 (openai_json_xml).
// - cache SHA-256 de respuestas vive en proxy, no aqui.
type Candidate struct {
	Provider  string
	Model     string
	Tier      string
	CostPer1M float64
	Healthy   bool
	QuotaLeft int
	Dialect   string
}

func Pick(cands []Candidate) *Candidate {
	var best *Candidate
	for i := range cands {
		c := &cands[i]
		if !c.Healthy || c.QuotaLeft <= 0 {
			continue
		}
		if best == nil || c.CostPer1M < best.CostPer1M {
			best = c
		}
	}
	return best
}

// DialectFor distingue el dialecto tool-call por modelo.
// Leccion Windsurf #102: kimi-k2 original habla vLLM; k2.5/k2.6 hablan OpenAI JSON/XML.
func DialectFor(model string) string {
	m := strings.ToLower(model)
	switch m {
	case "kimi-k2", "kimi-k2-thinking":
		return "kimi_k2_vllm"
	default:
		return "openai_json_xml"
	}
}

// DefaultCandidates construye candidatos demo por tier: uno gratis y uno pago.
// En produccion se puebla desde env/flags (ver proxy: ROUTER_PROVIDERS).
func DefaultCandidates(tier string) []Candidate {
	m := catalog.ByID(catalog.Default(), tier)
	if m == nil || len(m.Examples) == 0 {
		return nil
	}
	cheap := m.Examples[0]
	paid := cheap
	if len(m.Examples) > 1 {
		paid = m.Examples[1]
	}
	return []Candidate{
		{Provider: "free-pool", Model: cheap, Tier: tier, CostPer1M: 0, Healthy: true, QuotaLeft: 100, Dialect: DialectFor(cheap)},
		{Provider: "paid", Model: paid, Tier: tier, CostPer1M: m.CostInPer1M, Healthy: true, QuotaLeft: 1 << 30, Dialect: DialectFor(paid)},
	}
}

// TierOfModel infiere el tier por nombre de modelo conocido.
func TierOfModel(model string) string {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "haiku"),
		strings.Contains(m, "mini"),
		strings.Contains(m, "flash-lite"),
		strings.Contains(m, "k2-lite"),
		strings.Contains(m, "7b"), strings.Contains(m, "8b"):
		return catalog.TierHaiku
	case strings.Contains(m, "opus"),
		strings.Contains(m, "thinking"),
		strings.Contains(m, "fable"),
		strings.Contains(m, "max"),
		strings.Contains(m, "1m-max"):
		return catalog.TierFrontier
	default:
		return catalog.TierBalanced
	}
}
