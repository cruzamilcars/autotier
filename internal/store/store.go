package store

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"sort"

	"github.com/cruzamilcars/autotier/internal/catalog"
)

// Record es una linea JSONL del log: un intento final por request
// (con escalations = reintentos en tiers superiores).
type Record struct {
	Tier        string  `json:"tier"`
	Provider    string  `json:"provider"`
	Model       string  `json:"model"`
	Domain      string  `json:"domain"`
	Mode        string  `json:"mode"`
	InTokens    int     `json:"in_tokens"`
	OutTokens   int     `json:"out_tokens"`
	CostUSD     float64 `json:"cost_usd"`
	LatencyMs   float64 `json:"latency_ms"`
	Escalations int     `json:"escalations"`
	QualityPass bool    `json:"quality_pass"`
	Agent       string  `json:"agent,omitempty"`
	Parent      string  `json:"parent,omitempty"`
	Decider     string  `json:"decider,omitempty"`
	Cached      bool    `json:"cached,omitempty"`
}

type Summary struct {
	Requests       int
	CacheHits      int
	TotalCostUSD   float64
	FrontierCost   float64
	SavingsPct     float64
	Escalations    int
	QualityPassPct float64
	ByTier         map[string]int
	ByAgent        map[string]int
	ByDecider      map[string]int
	P95LatencyMs   float64
}

func Append(path string, r Record) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

func ReadAll(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var r Record
		if err := json.Unmarshal(line, &r); err != nil {
			continue // linea corrupta: se salta, no rompe el status
		}
		out = append(out, r)
	}
	return out, sc.Err()
}

func Summarize(recs []Record) Summary {
	s := Summary{ByTier: map[string]int{}, ByAgent: map[string]int{}, ByDecider: map[string]int{}}
	if len(recs) == 0 {
		return s
	}
	lats := make([]float64, 0, len(recs))
	pass := 0
	for _, r := range recs {
		if r.Cached {
			s.CacheHits++
			continue // no duplica costo ni requests
		}
		s.Requests++
		s.TotalCostUSD += r.CostUSD
		s.FrontierCost += catalog.FrontierCost(r.InTokens, r.OutTokens)
		s.Escalations += r.Escalations
		s.ByTier[r.Tier]++
		if r.Agent != "" {
			s.ByAgent[r.Agent]++
		}
		if r.Decider != "" {
			s.ByDecider[r.Decider]++
		}
		lats = append(lats, r.LatencyMs)
		if r.QualityPass {
			pass++
		}
	}
	if s.FrontierCost > 0 {
		s.SavingsPct = 100 * (s.FrontierCost - s.TotalCostUSD) / s.FrontierCost
	}
	s.QualityPassPct = 100 * float64(pass) / float64(len(recs))
	sort.Float64s(lats)
	idx := int(math.Ceil(0.95*float64(len(lats)))) - 1
	if idx < 0 {
		idx = 0
	}
	s.P95LatencyMs = lats[idx]
	return s
}
