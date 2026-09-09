package policy

import (
	"testing"

	"github.com/cruzamilcars/autotier/internal/catalog"
)

func TestDecideTrivialHaiku(t *testing.T) {
	d := Decide(0.1, "research", ModeCost, "", 0, 800, 512)
	if d.Tier != catalog.TierHaiku {
		t.Fatalf("got %s want haiku-tier", d.Tier)
	}
}

func TestDecideCriticalFrontier(t *testing.T) {
	d := Decide(0.95, "reasoning", ModeCost, "", 0, 800, 512)
	if d.Tier != catalog.TierFrontier {
		t.Fatalf("got %s want frontier-tier", d.Tier)
	}
}

func TestCapabilityOverride(t *testing.T) {
	// Complejidad baja pero dominio reasoning con floor intelligence (8):
	// haiku (5) no es elegible -> debe subir.
	d := Decide(0.1, "reasoning", ModeIntelligence, "", 0, 800, 512)
	if d.Tier == catalog.TierHaiku {
		t.Fatalf("haiku no alcanza floor 8 en reasoning, debio subir, got %s", d.Tier)
	}
}

func TestEscalate(t *testing.T) {
	n, ok := Escalate(catalog.TierHaiku)
	if !ok || n != catalog.TierBalanced {
		t.Fatalf("got %s,%v", n, ok)
	}
	if _, ok := Escalate(catalog.TierFrontier); ok {
		t.Fatalf("frontera no debe escalar")
	}
}

func TestBudgetCap(t *testing.T) {
	// Presupuesto que no alcanza ni frontera para 1M tokens.
	// El piso de calidad manda: baja hasta el elegible mas barato y marca
	// budget_exceeded. Nota: con evidencia bench, haiku reasoning=6, por eso
	// el elegible mas barato en cost (floor 6) es haiku, no balanced.
	d := Decide(0.95, "reasoning", ModeCost, "", 0.50, 1000000, 100000)
	if d.Tier != catalog.TierHaiku {
		t.Fatalf("con techo bajo debe bajar a haiku (piso), got %s", d.Tier)
	}
	if !d.BudgetExceeded {
		t.Fatalf("debio marcar budget_exceeded")
	}
}
