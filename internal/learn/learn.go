package learn

// Bandit Thompson Sampling sobre el log de outcomes.
// Cada brazo = tier|dominio con prior optimista Beta(2,1): sin evidencia se
// exploran los baratos primero; con evidencia (>=MinPulls) el router puede
// bajar de tier (learned-cheaper) o subirlo (learned-escalate).
// Sin estado (--learn vacio) o sin evidencia: mandan las reglas (cold start
// determinista, el eval no cambia).
//
// Exito de un brazo = quality pass sin escalacion para el tier inicial,
// quality pass para el tier final.

import (
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"sort"

	"github.com/cruzamilcars/autotier/internal/catalog"
)

const (
	// MinPulls evita override con ruido de pocas muestras.
	MinPulls = 5
	// Prior optimista: explorar barato primero.
	priorAlpha = 2.0
	priorBeta  = 1.0
)

type Arm struct {
	Wins  int `json:"wins"`
	Total int `json:"total"`
}

type State struct {
	Arms map[string]*Arm `json:"arms"`
}

func New() *State { return &State{Arms: map[string]*Arm{}} }

// Key identifica un brazo. Dominio "" = agregado global del tier.
func Key(tier, domain string) string { return tier + "|" + domain }

func (s *State) arm(key string) *Arm {
	a, ok := s.Arms[key]
	if !ok {
		a = &Arm{}
		s.Arms[key] = a
	}
	return a
}

// Observe registra un outcome en el brazo especifico y en el agregado.
func (s *State) Observe(tier, domain string, success bool) {
	for _, k := range []string{Key(tier, domain), Key(tier, "")} {
		a := s.arm(k)
		a.Total++
		if success {
			a.Wins++
		}
	}
}

// Mean devuelve la media posterior (determinista, para status/tests).
func (s *State) Mean(tier, domain string) float64 {
	a := s.forRead(tier, domain)
	if a == nil {
		return priorAlpha / (priorAlpha + priorBeta)
	}
	return (float64(a.Wins) + priorAlpha) / (float64(a.Total) + priorAlpha + priorBeta)
}

// Pulls devuelve muestras acumuladas (especifico o agregado).
func (s *State) Pulls(tier, domain string) int {
	if a := s.forRead(tier, domain); a != nil {
		return a.Total
	}
	return 0
}

func (s *State) forRead(tier, domain string) *Arm {
	if a, ok := s.Arms[Key(tier, domain)]; ok && a.Total > 0 {
		return a
	}
	if a, ok := s.Arms[Key(tier, "")]; ok && a.Total > 0 {
		return a
	}
	return nil
}

// Theta muestrea la prob. de exito (Thompson Sampling).
func (s *State) Theta(tier, domain string, rng *rand.Rand) float64 {
	a := s.forRead(tier, domain)
	alpha, beta := priorAlpha, priorBeta
	if a != nil {
		alpha += float64(a.Wins)
		beta += float64(a.Total - a.Wins)
	}
	return sampleBeta(rng, alpha, beta)
}

// RequiredP mapea complejidad a prob. de exito exigida (0.55..0.90).
func RequiredP(complexity float64) float64 {
	if complexity < 0 {
		complexity = 0
	}
	if complexity > 1 {
		complexity = 1
	}
	return 0.55 + 0.35*complexity
}

type Input struct {
	RulesTier  string
	Eligible   []catalog.Model // orden barato->caro
	Complexity float64
	Domain     string
	RNG        *rand.Rand
}

type Output struct {
	Tier   string
	Reason string // rules-cold | rules-unranked | learned-cheaper | learned-escalate | rules-ok
}

// Override decide con evidencia: solo se aparta de reglas con >=MinPulls.
func (s *State) Override(in Input) Output {
	idx := -1
	for i, m := range in.Eligible {
		if m.ID == in.RulesTier {
			idx = i
			break
		}
	}
	if idx < 0 {
		return Output{Tier: in.RulesTier, Reason: "rules-unranked"}
	}
	req := RequiredP(in.Complexity)
	thetas := map[string]float64{}
	for _, m := range in.Eligible {
		thetas[m.ID] = s.Theta(m.ID, in.Domain, in.RNG)
	}
	// Bajar: el mas barato con evidencia y theta suficiente.
	for i := 0; i < idx; i++ {
		t := in.Eligible[i].ID
		if s.Pulls(t, in.Domain) >= MinPulls && thetas[t] >= req {
			return Output{Tier: t, Reason: "learned-cheaper"}
		}
	}
	// Subir: el tier de reglas con evidencia de que falla.
	if s.Pulls(in.RulesTier, in.Domain) >= MinPulls && thetas[in.RulesTier] < req-0.15 && idx+1 < len(in.Eligible) {
		return Output{Tier: in.Eligible[idx+1].ID, Reason: "learned-escalate"}
	}
	if s.Pulls(in.RulesTier, in.Domain) < MinPulls {
		return Output{Tier: in.RulesTier, Reason: "rules-cold"}
	}
	return Output{Tier: in.RulesTier, Reason: "rules-ok"}
}

// Stats expone un brazo por clave cruda (para `learn status`).
func (s *State) Stats(key string) (pulls, wins int, mean float64) {
	mean = priorAlpha / (priorAlpha + priorBeta)
	a, ok := s.Arms[key]
	if !ok {
		return 0, 0, mean
	}
	return a.Total, a.Wins, (float64(a.Wins) + priorAlpha) / (float64(a.Total) + priorAlpha + priorBeta)
}

// Keys ordenadas para `learn status`.
func (s *State) Keys() []string {
	out := make([]string, 0, len(s.Arms))
	for k := range s.Arms {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func Load(path string) (*State, error) {
	s := New()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, err
	}
	if s.Arms == nil {
		s.Arms = map[string]*Arm{}
	}
	return s, nil
}

func (s *State) Save(path string) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}

// sampleBeta via gammas (Marsaglia-Tsang, solo stdlib).
func sampleBeta(rng *rand.Rand, alpha, beta float64) float64 {
	x := sampleGamma(rng, alpha)
	y := sampleGamma(rng, beta)
	if x+y == 0 {
		return 0.5
	}
	return x / (x + y)
}

func sampleGamma(rng *rand.Rand, a float64) float64 {
	if a < 1 {
		return sampleGamma(rng, a+1) * math.Pow(rng.Float64(), 1/a)
	}
	d := a - 1.0/3.0
	c := 1.0 / math.Sqrt(9*d)
	for {
		x := rng.NormFloat64()
		v := 1 + c*x
		if v <= 0 {
			continue
		}
		v = v * v * v
		u := rng.Float64()
		if u < 1-0.0331*(x*x)*(x*x) {
			return d * v
		}
		if math.Log(u) < 0.5*x*x+d*(1-v+math.Log(v)) {
			return d * v
		}
	}
}
