package policy

import (
	"testing"

	"github.com/cruzamilcars/autotier/internal/catalog"
)

func TestParseDeny(t *testing.T) {
	d, err := ParseDeny("haiku, frontier-tier")
	if err != nil {
		t.Fatal(err)
	}
	if len(d) != 2 || d[0] != catalog.TierHaiku || d[1] != catalog.TierFrontier {
		t.Fatalf("got %v", d)
	}
	if _, err := ParseDeny("haikku"); err == nil {
		t.Fatalf("typo debe fallar")
	}
	if d, err := ParseDeny(""); err != nil || len(d) != 0 {
		t.Fatalf("vacio debe dar nil: %v %v", d, err)
	}
}

func TestDecideWithDenyDowngrade(t *testing.T) {
	// Tarea critica (frontera) con frontier vetado -> balanced (bajar, no subir a prohibido).
	d, err := DecideWith(0.95, "reasoning", ModeCost, "", []string{catalog.TierFrontier}, 0, 800, 512)
	if err != nil {
		t.Fatal(err)
	}
	if d.Tier != catalog.TierBalanced {
		t.Fatalf("got %s want balanced-tier", d.Tier)
	}
}

func TestDecideWithDenyFailClosed(t *testing.T) {
	// Vetar todo -> error, nunca un tier prohibido en silencio.
	_, err := DecideWith(0.1, "general", ModeCost, "",
		[]string{catalog.TierHaiku, catalog.TierBalanced, catalog.TierFrontier}, 0, 800, 512)
	if err == nil {
		t.Fatalf("deny total debe fallar cerrado")
	}
}

func TestDecideWithDenyVsMinTier(t *testing.T) {
	// min-tier vetado = error de configuracion, no override silencioso.
	_, err := DecideWith(0.1, "general", ModeCost,
		catalog.TierFrontier, []string{catalog.TierFrontier}, 0, 800, 512)
	if err == nil {
		t.Fatalf("min-tier en deny debe fallar")
	}
}

func TestDecideWithoutDenyUnchanged(t *testing.T) {
	// Sin deny, DecideWith == Decide.
	a := Decide(0.5, "code", ModeCost, "", 0, 800, 512)
	b, err := DecideWith(0.5, "code", ModeCost, "", nil, 0, 800, 512)
	if err != nil || a != b {
		t.Fatalf("divergen: %+v %+v %v", a, b, err)
	}
}
