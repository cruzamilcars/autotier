package catalog

// Catalogo de capacidades: que tier es bueno en que dominio (0-10),
// ventana, costo $/1M tokens y latencia p95. Espeja
// internal/catalog/capability-catalog.yaml (fuente legible para humanos);
// este archivo es la version compilada que usa el router en runtime
// para no depender de parsers YAML externos (solo stdlib).

type Scores map[string]int

type Model struct {
	ID            string
	Examples      []string
	CostInPer1M   float64
	CostOutPer1M  float64
	ContextWindow int
	LatencyP95Ms  int
	Scores        Scores
}

// Tier IDs estables usados por policy, proxy y evals.
const (
	TierHaiku    = "haiku-tier"
	TierBalanced = "balanced-tier"
	TierFrontier = "frontier-tier"
)

// FrontierListPrice referencia para calcular savings_pct_vs_all_frontier.
const (
	FrontierInPer1M  = 5.0
	FrontierOutPer1M = 25.0
)

func Default() []Model {
	out := Priors()
	// Overlay de evidencia auditada (`autotier bench import`, ver catalog.gen.go).
	for i := range out {
		if ov, ok := generatedScores[out[i].ID]; ok {
			for d, sc := range ov {
				out[i].Scores[d] = sc
			}
		}
	}
	return out
}

// Priors son los valores curados a mano. El blend de bench siempre parte de
// aqui (no del ultimo blend) para no hacer ratchet hacia la evidencia.
func Priors() []Model {
	out := []Model{
		{
			ID:            TierHaiku,
			Examples:      []string{"claude-haiku-4-5", "gpt-4o-mini", "kimi-k2-lite"},
			CostInPer1M:   1.0,
			CostOutPer1M:  5.0,
			ContextWindow: 200000,
			LatencyP95Ms:  800,
			Scores: Scores{
				"code": 6, "reasoning": 5, "research": 6,
				"presentation": 6, "design": 5, "data_analysis": 6,
				"ocr": 7, "multimodal": 5, "tool_use": 6, "general": 6,
			},
		},
		{
			ID:            TierBalanced,
			Examples:      []string{"claude-sonnet-4-6", "gpt-5.x", "kimi-k2.5"},
			CostInPer1M:   3.0,
			CostOutPer1M:  15.0,
			ContextWindow: 1000000,
			LatencyP95Ms:  2000,
			Scores: Scores{
				"code": 8, "reasoning": 7, "research": 8,
				"presentation": 8, "design": 8, "data_analysis": 8,
				"ocr": 8, "multimodal": 8, "tool_use": 8, "general": 8,
			},
		},
		{
			ID:            TierFrontier,
			Examples:      []string{"claude-opus-5", "gpt-5.x-high", "kimi-k2-thinking"},
			CostInPer1M:   5.0,
			CostOutPer1M:  25.0,
			ContextWindow: 1000000,
			LatencyP95Ms:  4500,
			Scores: Scores{
				"code": 10, "reasoning": 10, "research": 9,
				"presentation": 9, "design": 9, "data_analysis": 9,
				"ocr": 9, "multimodal": 9, "tool_use": 9, "general": 9,
			},
		},
	}
	return out
}

func ByID(models []Model, id string) *Model {
	for i := range models {
		if models[i].ID == id {
			return &models[i]
		}
	}
	return nil
}

// ScoreFor devuelve el score del tier en un dominio (0 si dominio desconocido).
func (m Model) ScoreFor(domain string) int {
	if s, ok := m.Scores[domain]; ok {
		return s
	}
	return 0
}

// Eligible filtra tiers con score >= minScore en el dominio, ordenados de
// barato a caro (el orden de Default() ya es ese).
func Eligible(models []Model, domain string, minScore int) []Model {
	out := make([]Model, 0, len(models))
	for _, m := range models {
		if m.ScoreFor(domain) >= minScore {
			out = append(out, m)
		}
	}
	return out
}

// CostFor estima USD para inTokens/outTokens a precio de lista del tier.
func CostFor(m Model, inTokens, outTokens int) float64 {
	return float64(inTokens)/1e6*m.CostInPer1M + float64(outTokens)/1e6*m.CostOutPer1M
}

// FrontierCost estima lo que habria costado todo en frontera (para savings_pct).
func FrontierCost(inTokens, outTokens int) float64 {
	return float64(inTokens)/1e6*FrontierInPer1M + float64(outTokens)/1e6*FrontierOutPer1M
}

// FitsWindow indica si el tier acepta el contexto estimado.
func (m Model) FitsWindow(estimatedTokens int) bool {
	return estimatedTokens <= m.ContextWindow
}
