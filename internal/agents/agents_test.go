package agents

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cruzamilcars/autotier/internal/catalog"
	"github.com/cruzamilcars/autotier/internal/policy"
)

func TestBuiltinsPresent(t *testing.T) {
	reg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"explore", "build", "judge", "orchestrator"} {
		if _, ok := reg[n]; !ok {
			t.Fatalf("falta builtin %s", n)
		}
	}
	if reg["explore"].Tier != catalog.TierHaiku {
		t.Fatalf("explore debe ser haiku, got %s", reg["explore"].Tier)
	}
}

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "reviewer.md")
	content := "---\nname: reviewer\ndescription: Revisa codigo\ntier: sonnet\ndelegates: [explore]\nmax_steps: 12\n---\n\nRevisa sin editar.\n"
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	a, err := ParseFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "reviewer" || a.Tier != catalog.TierBalanced {
		t.Fatalf("parseo mal: %+v", a)
	}
	if len(a.Delegates) != 1 || a.Delegates[0] != "explore" {
		t.Fatalf("delegates mal: %+v", a.Delegates)
	}
	if a.MaxSteps != 12 || a.System != "Revisa sin editar." {
		t.Fatalf("campos mal: %+v", a)
	}
}

func TestFileOverridesBuiltin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "explore.md"),
		[]byte("---\ntier: frontier-tier\n---\nCustom.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	reg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reg["explore"].Tier != catalog.TierFrontier || reg["explore"].Name != "explore" {
		t.Fatalf("override mal: %+v", reg["explore"])
	}
}

func TestMentions(t *testing.T) {
	got := Mentions("delega a @explore y luego @judge decide, @explore otra vez")
	if len(got) != 2 || got[0] != "explore" || got[1] != "judge" {
		t.Fatalf("got %v", got)
	}
	if len(Mentions("sin menciones")) != 0 {
		t.Fatalf("falso positivo")
	}
}

func TestCanDelegate(t *testing.T) {
	reg, _ := Load("")
	if !reg["orchestrator"].CanDelegate("judge") {
		t.Fatalf("orchestrator debe poder llamar a judge")
	}
	if reg["explore"].CanDelegate("judge") {
		t.Fatalf("explore no delega a nadie")
	}
	if reg["build"].CanDelegate("judge") {
		t.Fatalf("build solo delega a explore")
	}
}

func TestResolveTierFixedVsAuto(t *testing.T) {
	reg, _ := Load("")
	// judge siempre frontera aunque la tarea sea trivial
	d := reg["judge"].ResolveTier(0.05, "research", policy.ModeCost, 0, 800, 512)
	if d.Tier != catalog.TierFrontier {
		t.Fatalf("judge fijo debe dar frontier, got %s", d.Tier)
	}
	// orchestrator auto: trivial -> haiku
	d2 := reg["orchestrator"].ResolveTier(0.05, "research", policy.ModeCost, 0, 800, 512)
	if d2.Tier != catalog.TierHaiku {
		t.Fatalf("orchestrator auto trivial debe dar haiku, got %s", d2.Tier)
	}
}
