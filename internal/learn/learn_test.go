package learn

import (
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/cruzamilcars/autotier/internal/catalog"
)

func eligible() []catalog.Model { return catalog.Default() } // ya viene barato->caro

func TestObserveCounts(t *testing.T) {
	s := New()
	s.Observe("haiku-tier", "general", true)
	s.Observe("haiku-tier", "general", false)
	if s.Arms[Key("haiku-tier", "general")].Total != 2 {
		t.Fatalf("total mal")
	}
	if s.Arms[Key("haiku-tier", "")].Wins != 1 {
		t.Fatalf("agregado mal")
	}
	if s.Pulls("haiku-tier", "general") != 2 {
		t.Fatalf("pulls mal")
	}
}

func TestColdStartRules(t *testing.T) {
	s := New()
	out := s.Override(Input{
		RulesTier: "balanced-tier", Eligible: eligible(),
		Complexity: 0.2, Domain: "general", RNG: rand.New(rand.NewSource(1)),
	})
	if out.Tier != "balanced-tier" || out.Reason != "rules-cold" {
		t.Fatalf("cold start debe respetar reglas: %+v", out)
	}
}

func TestLearnedCheaper(t *testing.T) {
	s := New()
	for i := 0; i < 20; i++ { // haiku funciona 20/20
		s.Observe("haiku-tier", "general", true)
	}
	out := s.Override(Input{
		RulesTier: "balanced-tier", Eligible: eligible(),
		Complexity: 0.1, Domain: "general", RNG: rand.New(rand.NewSource(7)),
	})
	if out.Tier != "haiku-tier" || out.Reason != "learned-cheaper" {
		t.Fatalf("debió bajar a haiku con evidencia: %+v", out)
	}
}

func TestLearnedEscalate(t *testing.T) {
	s := New()
	for i := 0; i < 20; i++ { // balanced falla 20/20
		s.Observe("balanced-tier", "code", false)
	}
	out := s.Override(Input{
		RulesTier: "balanced-tier", Eligible: eligible(),
		Complexity: 0.9, Domain: "code", RNG: rand.New(rand.NewSource(7)),
	})
	if out.Tier != "frontier-tier" || out.Reason != "learned-escalate" {
		t.Fatalf("debió subir a frontier con evidencia: %+v", out)
	}
}

func TestNoOverrideWithFewPulls(t *testing.T) {
	s := New()
	for i := 0; i < 3; i++ { // solo 3 < MinPulls
		s.Observe("haiku-tier", "general", true)
	}
	out := s.Override(Input{
		RulesTier: "balanced-tier", Eligible: eligible(),
		Complexity: 0.1, Domain: "general", RNG: rand.New(rand.NewSource(7)),
	})
	if out.Tier != "balanced-tier" {
		t.Fatalf("con pocas muestras mandan reglas: %+v", out)
	}
}

func TestSaveLoad(t *testing.T) {
	s := New()
	s.Observe("haiku-tier", "general", true)
	p := filepath.Join(t.TempDir(), "learn.json")
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	s2, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if s2.Pulls("haiku-tier", "general") != 1 {
		t.Fatalf("roundtrip mal")
	}
	if _, err := Load(filepath.Join(t.TempDir(), "noexiste.json")); err != nil {
		t.Fatalf("archivo inexistente debe dar estado vacio: %v", err)
	}
}

func TestBetaMeanSanity(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	n := 20000
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += sampleBeta(rng, 8, 2) // media teorica 0.8
	}
	if m := sum / float64(n); m < 0.77 || m > 0.83 {
		t.Fatalf("sampler sesgado: mean=%.3f", m)
	}
}
