package policy

import "github.com/cruzamilcars/autotier/internal/catalog"

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
	if mode != ModeBalance && mode != ModeIntelligence {
		mode = ModeCost
	}
	floor := QualityFloor(mode)
	base, steps := tierForComplexity(complexity)

	models := catalog.Default()
	eligible := catalog.Eligible(models, domain, floor)

	// Si el tier base no es elegible, subir al elegible mas barato.
	tier := base
	if !containsTier(eligible, base) {
		if len(eligible) > 0 {
			tier = eligible[0].ID // Eligible conserva orden barato->caro
		} else {
			tier = catalog.TierFrontier // sin elegibles: frontera por seguridad
		}
		steps = stepsFor(tier)
	}

	// Piso explicito nunca baja calidad.
	if minTierOverride != "" && tierRank[minTierOverride] > tierRank[tier] {
		tier = minTierOverride
		steps = stepsFor(tier)
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
	}
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
