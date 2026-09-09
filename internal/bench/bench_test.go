package bench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cruzamilcars/autotier/internal/catalog"
)

func TestBlend(t *testing.T) {
	if got := Blend(6, 7.33, 1, 1); got != 7 {
		t.Fatalf("blend(6,7.33) = %d, want 7", got)
	}
	if got := Blend(10, 8.6, 1, 1); got != 9 {
		t.Fatalf("blend(10,8.6) = %d, want 9", got)
	}
	if got := Blend(8, 7.88, 1, 1); got != 8 {
		t.Fatalf("blend(8,7.88) = %d, want 8", got)
	}
	if got := Blend(5, 9.0, 0, 0); got != 5 {
		t.Fatalf("pesos cero debe devolver prior, got %d", got)
	}
}

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	content := "# comentario\nname: test-bench\ndomain: code\nurl: https://x\ndate: 2026-09-09\nweight: 2\nscores:\n  claude-opus-5: 96.0\n  kimi-k2-5: 76.8\nexclude:\n  foo-bar: \"motivo\"\n"
	if err := os.WriteFile(filepath.Join(dir, "t.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := parseFile(filepath.Join(dir, "t.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "test-bench" || s.Domain != "code" || s.Weight != 2 {
		t.Fatalf("escalares mal: %+v", s)
	}
	if s.Scores["claude-opus-5"] != 96.0 || len(s.Scores) != 2 {
		t.Fatalf("scores mal: %+v", s.Scores)
	}
	if s.Exclude["foo-bar"] == "" {
		t.Fatalf("exclude mal: %+v", s.Exclude)
	}
}

func TestTierEvidenceRealData(t *testing.T) {
	sources, err := LoadDir("data")
	if err != nil {
		t.Skipf("sin data dir en test: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("se esperaban 2 fuentes, got %d", len(sources))
	}
	ev := TierEvidence(catalog.Default(), sources, "code")
	// frontier: opus-5 9.6, gpt 8.0/7.63, k2-thinking sin dato
	f, ok := ev["frontier-tier"]
	if !ok || f.N == 0 {
		t.Fatalf("frontier sin evidencia: %+v", ev)
	}
	if f.Score < 7.5 || f.Score > 9.7 {
		t.Fatalf("evidencia frontier code fuera de rango: %.2f", f.Score)
	}
	// exclusion documentada: sonnet-4-6 no debe contaminar reasoning
	evR := TierEvidence(catalog.Default(), sources, "reasoning")
	for _, fr := range evR["balanced-tier"].From {
		if strings.Contains(fr, "sonnet-4-6") {
			t.Fatalf("exclusion no honrada: %s", fr)
		}
	}
}
