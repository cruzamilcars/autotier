package bench

import "testing"

func TestRecencyFactor(t *testing.T) {
	if got := RecencyFactor("2026-09-09", "2026-09-09", 180, 0.2); got != 1 {
		t.Fatalf("misma fecha = 1, got %.3f", got)
	}
	if got := RecencyFactor("2026-09-09", "2026-09-09", 0, 0.2); got != 1 {
		t.Fatalf("halfLife<=0 desactiva, got %.3f", got)
	}
	if got := RecencyFactor("2026-03-09", "2026-09-09", 184, 0.2); got < 0.49 || got > 0.51 {
		t.Fatalf("una vida media ~= 0.5, got %.3f", got)
	}
	if got := RecencyFactor("2020-01-01", "2026-09-09", 180, 0.2); got != 0.2 {
		t.Fatalf("piso minF, got %.3f", got)
	}
	if got := RecencyFactor("2026-12-01", "2026-09-09", 180, 0.2); got != 1 {
		t.Fatalf("fecha futura = 1, got %.3f", got)
	}
	if got := RecencyFactor("no-fecha", "2026-09-09", 180, 0.2); got != 1 {
		t.Fatalf("fecha invalida = 1, got %.3f", got)
	}
}

func TestEffectiveWeightsCombine(t *testing.T) {
	srcs := []Source{{Name: "a", Weight: 2, Date: "2026-09-09"}, {Name: "b", Weight: 1, Date: "2020-01-01"}}
	w := EffectiveWeights(srcs, "2026-09-09", 180, 0.2)
	if w["a"] != 2 {
		t.Fatalf("a debe quedar 2, got %.3f", w["a"])
	}
	if w["b"] < 0.19 || w["b"] > 0.21 {
		t.Fatalf("b debe quedar ~0.2 (piso), got %.3f", w["b"])
	}
}
