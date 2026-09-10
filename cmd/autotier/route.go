package main

import (
	"flag"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/cruzamilcars/autotier/internal/catalog"
	"github.com/cruzamilcars/autotier/internal/classifier"
	"github.com/cruzamilcars/autotier/internal/learn"
	"github.com/cruzamilcars/autotier/internal/policy"
)

// cmdRoute explica la decision SIN gastar: la prueba de que el router
// decide de verdad (misma pipeline que el proxy, sin llamada a modelo).
func cmdRoute(args []string) int {
	fs := flag.NewFlagSet("route", flag.ContinueOnError)
	mode := fs.String("mode", "cost", "cost|balance|intelligence")
	denyS := fs.String("deny", "", "tiers vetados, ej. frontier-tier")
	minTier := fs.String("min-tier", "", "piso de calidad")
	maxCost := fs.Float64("max-cost", 0, "techo USD por request")
	learnPath := fs.String("learn", "", "estado bandit (muestra que diria learn)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	prompt := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(prompt) == "" {
		fmt.Println("uso: autotier route [--mode cost] [--deny ...] [--min-tier ...] [--max-cost N] [--learn f] \"prompt\"")
		return 1
	}
	deny, err := policy.ParseDeny(*denyS)
	if err != nil {
		fmt.Println("error:", err)
		return 1
	}
	mt := ""
	if *minTier != "" {
		t, ok := policy.NormalizeTier(*minTier)
		if !ok {
			fmt.Println("error: min-tier desconocido:", *minTier)
			return 1
		}
		mt = t
	}
	m := policy.Mode(*mode)
	complexity := classifier.Score(prompt, false, strings.Contains(strings.ToLower(prompt), "screenshot"), 0)
	domain := classifier.DetectDomain(prompt)
	dec, err := policy.DecideWith(complexity, domain, m, mt, deny, *maxCost, 800, 512)
	if err != nil {
		fmt.Println("error:", err)
		return 1
	}
	models := catalog.Default()
	fmt.Printf("complexity=%.2f domain=%s mode=%s floor=%d\n", complexity, domain, dec.Mode, dec.MinScore)
	fmt.Println("elegibles (tier: score):")
	for _, em := range catalog.Eligible(models, domain, dec.MinScore) {
		mark := ""
		if em.ID == dec.Tier {
			mark = "  <- elegido por reglas"
		}
		vetoed := ""
		for _, d := range deny {
			if d == em.ID {
				vetoed = "  [VETADO]"
			}
		}
		fmt.Printf("  %-14s %d%s%s\n", em.ID, em.ScoreFor(domain), mark, vetoed)
	}
	mdl := catalog.ByID(models, dec.Tier)
	cost := 0.0
	if mdl != nil {
		cost = catalog.CostFor(*mdl, 800, 512)
	}
	frontier := catalog.FrontierCost(800, 512)
	fmt.Printf("decision=%s costo_est=$%.6f ahorro_vs_frontera=%.0f%% budget_exceeded=%v max_steps=%d\n",
		dec.Tier, cost, 100*(frontier-cost)/frontier, dec.BudgetExceeded, dec.MaxSteps)
	if *learnPath != "" {
		st, err := learn.Load(*learnPath)
		if err != nil {
			fmt.Println("learn error:", err)
			return 1
		}
		out := st.Override(learn.Input{
			RulesTier:  dec.Tier,
			Eligible:   catalog.Eligible(models, domain, dec.MinScore),
			Complexity: complexity, Domain: domain,
			RNG: rand.New(rand.NewSource(time.Now().UnixNano())),
		})
		fmt.Printf("learn diria: %s (%s)\n", out.Tier, out.Reason)
	}
	fmt.Println("nota: dry-run, $0 gastados, 0 llamadas a modelos.")
	return 0
}
