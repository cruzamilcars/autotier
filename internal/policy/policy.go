package policy

import (
	"fmt"
	"strings"

	"github.com/cruzamilcars/autotier/internal/catalog"
)

// NormalizeTier acepta IDs y alias (haiku/sonnet/opus, balanced/frontier).
func NormalizeTier(s string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case catalog.TierHaiku, "haiku":
		return catalog.TierHaiku, true
	case catalog.TierBalanced, "sonnet", "balanced":
		return catalog.TierBalanced, true
	case catalog.TierFrontier, "opus", "frontier":
		return catalog.TierFrontier, true
	default:
		return "", false
	}
}

// ParseDeny parsea "haiku-tier,frontier-tier" (o alias). Error en typo:
// un veto mal escrito no debe ignorarse en silencio.
func ParseDeny(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		t, ok := NormalizeTier(p)
		if !ok {
			return nil, fmt.Errorf("tier desconocido en deny: %q (usa haiku|balanced|frontier o sus IDs)", strings.TrimSpace(p))
		}
		out = append(out, t)
	}
	return out, nil
}

func denied(tier string, deny []string) bool {
	for _, d := range deny {
		if d == tier {
			return true
		}
	}
	return false
}

func filterDenied(models []catalog.Model, deny []string) []catalog.Model {
	if len(deny) == 0 {
		return models
	}
	out := make([]catalog.Model, 0, len(models))
	for _, m := range models {
		if !denied(m.ID, deny) {
			out = append(out, m)
		}
	}
	return out
}

// Modos globales estilo Cursor Auto: deslizan quality_floor y sesgo.
type Mode string

const (
	ModeCost         Mode = "cost"
	ModeBalance      Mode = "balance"
	ModeIntelligence Mode = "intelligence"
)

type Decision struct {
	Tier           string
	MaxSteps       int
	MinScore       int
	Complexity     float64
	Domain         string
	Mode           Mode
	BudgetExceeded bool
}

// QualityFloor es el score minimo exigible en el catalogo segun modo.
func QualityFloor(mode Mode) int {
	switch mode {
	case ModeIntelligence:
		return 8
	case ModeBalance:
		return 7
	default:
		return 6
	}
}

// tierForComplexity mapea score a tier base (reglas por fase).
func tierForComplexity(c float64) (string, int) {
	switch {
	case c <= 0.35:
		return catalog.TierHaiku, 8
	case c <= 0.70:
		return catalog.TierBalanced, 25
	default:
		return catalog.TierFrontier, 40
	}
}

var tierRank = map[string]int{
	catalog.TierHaiku:    0,
	catalog.TierBalanced: 1,
	catalog.TierFrontier: 2,
}

var rankTier = []string{catalog.TierHaiku, catalog.TierBalanced, catalog.TierFrontier}

// Escalate sube un rung (haiku->balanced->frontier). false si ya es frontera.
func Escalate(tier string) (string, bool) {
	r, ok := tierRank[tier]
	if !ok || r >= len(rankTier)-1 {
		return tier, false
	}
	return rankTier[r+1], true
}

// Decide elige tier cruzando complejidad + capacidad del catalogo:
// si el tier barato no alcanza el quality_floor en el dominio, sube.
// minTierOverride (piso) y maxCostUSD (techo) son guardarrails opcionales.
func Decide(complexity float64, domain string, mode Mode, minTierOverride string, maxCostUSD float64, estInTokens, estOutTokens int) Decision {
	d, _ := DecideWith(complexity, domain, mode, minTierOverride, nil, maxCostUSD, estInTokens, estOutTokens)
	return d
}

// DecideWith = Decide + veto de tiers. Si deny vacia los elegibles falla
// cerrado (error) en vez de correr un tier prohibido en silencio.
func DecideWith(complexity float64, domain string, mode Mode, minTierOverride string, deny []string, maxCostUSD float64, estInTokens, estOutTokens int) (Decision, error) {
	if mode != ModeBalance && mode != ModeIntelligence {
		mode = ModeCost
	}
	floor := QualityFloor(mode)
	base, steps := tierForComplexity(complexity)

	models := catalog.Default()
	raw := catalog.Eligible(models, domain, floor)
	eligible := filterDenied(raw, deny)
	if len(eligible) == 0 && len(deny) > 0 {
		return Decision{}, fmt.Errorf("deny %v no deja tiers elegibles para dominio %q (floor %d)", deny, domain, floor)
	}

	// Si el tier base no es elegible, subir al elegible mas barato.
	tier := base
	if !containsTier(raw, base) {
		if len(eligible) > 0 {
			tier = eligible[0].ID // Eligible conserva orden barato->caro
		} else {
			tier = catalog.TierFrontier // sin elegibles: frontera por seguridad
		}
		steps = stepsFor(tier)
	} else if denied(base, deny) {
		// Veto: el base es elegible por capacidad pero esta prohibido ->
		// bajar al permitido mas cercano por debajo (nunca subir a uno prohibido).
		tier = ""
		for i := len(eligible) - 1; i >= 0; i-- {
			if tierRank[eligible[i].ID] < tierRank[base] {
				tier = eligible[i].ID
				break
			}
		}
		if tier == "" {
			return Decision{}, fmt.Errorf("deny %v bloquea el tier %s sin alternativa por debajo", deny, base)
		}
		steps = stepsFor(tier)
	}

	// Piso explicito nunca baja calidad (y nunca viola deny: si el piso esta
	// vetado, es error del caller, no override silencioso).
	if minTierOverride != "" {
		if denied(minTierOverride, deny) {
			return Decision{}, fmt.Errorf("min-tier %s esta en deny %v", minTierOverride, deny)
		}
		if tierRank[minTierOverride] > tierRank[tier] {
			tier = minTierOverride
			steps = stepsFor(tier)
		}
	}

	// Techo de costo: baja al mejor tier elegible que quepa en presupuesto.
	// El piso de calidad manda: nunca baja de eligible[0] aunque siga caro
	// (el caller debe marcar budget_exceeded).
	budgetExceeded := false
	if maxCostUSD > 0 {
		tier = fitBudget(eligible, tier, maxCostUSD, estInTokens, estOutTokens)
		steps = stepsFor(tier)
		if m := catalog.ByID(models, tier); m != nil {
			budgetExceeded = catalog.CostFor(*m, estInTokens, estOutTokens) > maxCostUSD
		}
	}

	return Decision{
		Tier: tier, MaxSteps: steps, MinScore: floor,
		Complexity: complexity, Domain: domain, Mode: mode,
		BudgetExceeded: budgetExceeded,
	}, nil
}

func containsTier(models []catalog.Model, tier string) bool {
	for _, m := range models {
		if m.ID == tier {
			return true
		}
	}
	return false
}

func stepsFor(tier string) int {
	switch tier {
	case catalog.TierHaiku:
		return 8
	case catalog.TierFrontier:
		return 40
	default:
		return 25
	}
}

// fitBudget baja tiers hasta que el costo estimado quepa en maxCostUSD.
// Si ni el mas barato cabe, devuelve el mas barato (y el caller marca budget_exceeded).
func fitBudget(eligible []catalog.Model, current string, maxCostUSD float64, inT, outT int) string {
	byID := map[string]catalog.Model{}
	for _, m := range eligible {
		byID[m.ID] = m
	}
	// Candidatos de caro a barato: quedarnos con el mejor que quepa.
	order := []string{catalog.TierFrontier, catalog.TierBalanced, catalog.TierHaiku}
	best := ""
	for _, t := range order {
		m, ok := byID[t]
		if !ok {
			continue
		}
		if catalog.CostFor(m, inT, outT) <= maxCostUSD {
			best = t
			break
		}
	}
	if best == "" {
		// Nada cabe: el mas barato elegible (o el actual si no hay elegibles).
		if len(eligible) > 0 {
			return eligible[0].ID
		}
		return current
	}
	// No subir por encima del tier decidido (el techo solo puede bajar).
	if tierRank[best] > tierRank[current] {
		return current
	}
	return best
}
