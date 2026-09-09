package catalog

import "testing"

// El overlay generado por `bench import` debe aplicarse sobre los priors.
func TestGeneratedOverlayApplies(t *testing.T) {
	models := Default()
	byID := map[string]Model{}
	for _, m := range models {
		byID[m.ID] = m
	}
	// Estos valores los escribe bench import desde evidencia real;
	// el test fija el contrato: code haiku sube, reasoning frontier baja.
	if got := byID[TierHaiku].ScoreFor("code"); got != 7 {
		t.Fatalf("haiku code con evidencia = %d, want 7", got)
	}
	if got := byID[TierFrontier].ScoreFor("reasoning"); got != 9 {
		t.Fatalf("frontier reasoning con evidencia = %d, want 9", got)
	}
	if got := byID[TierBalanced].ScoreFor("code"); got != 8 {
		t.Fatalf("balanced code debe quedar 8, got %d", got)
	}
	if len(generatedProvenance) == 0 {
		t.Fatalf("provenance vacia")
	}
}
