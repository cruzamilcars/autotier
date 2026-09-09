package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cruzamilcars/autotier/internal/bench"
	"github.com/cruzamilcars/autotier/internal/catalog"
)

func cmdBench(args []string) int {
	if len(args) == 0 {
		fmt.Println("uso: autotier bench <report|import> [--bench-dir internal/bench/data] [--write]")
		return 1
	}
	switch args[0] {
	case "report":
		return benchReport(args[1:], false)
	case "import":
		return benchReport(args[1:], true)
	default:
		fmt.Println("subcomando desconocido:", args[0])
		return 1
	}
}

func benchReport(args []string, write bool) int {
	fs := flag.NewFlagSet("bench", flag.ContinueOnError)
	benchDir := fs.String("bench-dir", filepath.Join("internal", "bench", "data"), "fuentes yaml")
	wPrior := fs.Float64("prior-weight", 1, "peso del prior curado")
	wEv := fs.Float64("evidence-weight", 1, "peso de la evidencia")
	doWrite := fs.Bool("write", false, "aplicar al catalogo (solo con import)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	sources, err := bench.LoadDir(*benchDir)
	if err != nil {
		fmt.Println("error:", err)
		return 1
	}
	if len(sources) == 0 {
		fmt.Println("sin fuentes en", *benchDir)
		return 1
	}
	models := catalog.Default()
	cells := bench.Report(models, sources, *wPrior, *wEv)
	fmt.Printf("%-13s %-11s %5s %8s %3s %7s %7s\n", "TIER", "DOMINIO", "prior", "evidencia", "n", "blend", "cambio")
	for _, c := range cells {
		mark := ""
		if c.Changed {
			mark = "  <-"
		}
		fmt.Printf("%-13s %-11s %5d %8.2f %3d %7d %s\n",
			c.Tier, c.Domain, c.Prior, c.Evidence, c.N, c.Blended, mark)
	}
	fmt.Println("fuentes:")
	for _, s := range sources {
		fmt.Printf("  - %s [%s] %s (%s) peso=%.1f scores=%d excl=%d\n",
			s.Name, s.Domain, s.URL, s.Date, s.Weight, len(s.Scores), len(s.Exclude))
	}
	if !write || !*doWrite {
		fmt.Println("(solo reporte; `bench import --write` para aplicar)")
		return 0
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Println("error:", err)
		return 1
	}
	ap, err := bench.Import(models, sources, *wPrior, *wEv, cwd)
	if err != nil {
		fmt.Println("error:", err)
		return 1
	}
	fmt.Printf("aplicado: %s + %s\n", ap.YamlPath, ap.GenPath)
	return 0
}
