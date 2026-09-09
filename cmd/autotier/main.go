package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cruzamilcars/autotier/internal/adapters"
	"github.com/cruzamilcars/autotier/internal/evals"
	"github.com/cruzamilcars/autotier/internal/policy"
	"github.com/cruzamilcars/autotier/internal/proxy"
	"github.com/cruzamilcars/autotier/internal/store"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	var code int
	switch os.Args[1] {
	case "init":
		code = cmdInit(os.Args[2:])
	case "proxy":
		code = cmdProxy(os.Args[2:])
	case "status":
		code = cmdStatus(os.Args[2:])
	case "eval":
		code = cmdEval(os.Args[2:])
	case "agent":
		code = cmdAgent(os.Args[2:])
	case "learn":
		code = cmdLearn(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println("autotier", version)
	default:
		fmt.Println("comando desconocido:", os.Args[1])
		usage()
		code = 1
	}
	os.Exit(code)
}

func usage() {
	fmt.Println("uso: autotier <init|proxy|status|eval|agent|learn> [flags]")
	fmt.Println("  init   --system ... --out DIR [--force] [--with-agents] [--agents-dir agents]")
	fmt.Println("  proxy  --port 4000 [--upstream URL] [--api-key KEY|$AUTOTIER_API_KEY] [--mock] [--log autotier.log.jsonl] [--mode cost|balance|intelligence] [--agents-dir agents] [--learn learn.json]")
	fmt.Println("  status --log autotier.log.jsonl")
	fmt.Println("  eval   --suite evals/suite.yaml --mode cost|balance|intelligence")
	fmt.Println("  agent  list|show <nombre>|call <nombre> \"tarea @otro\" [--context f] [--to otro] [--learn learn.json]")
	fmt.Println("  learn  status|reset [--learn learn.json]")
	fmt.Println("  (nota: los --flags van ANTES de los posicionales, limite del parser stdlib)")
}

func cmdInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	system := fs.String("system", "", "sistema destino")
	out := fs.String("out", "out", "directorio destino")
	force := fs.Bool("force", false, "sobrescribir archivos existentes")
	withAgents := fs.Bool("with-agents", false, "exportar registry agents/ a subagentes nativos")
	agentsDir := fs.String("agents-dir", "agents", "registry de agentes")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *system == "" {
		fmt.Println("falta --system (usa:", strings.Join(adapters.Systems(), "|")+")")
		return 1
	}
	var written []string
	var err error
	if *withAgents {
		written, err = adapters.GenerateAgents(*system, *agentsDir, *out, *force)
	} else {
		written, err = adapters.Generate(*system, *out, *force)
	}
	if err != nil {
		fmt.Println("error:", err)
		return 1
	}
	fmt.Printf("adapter %s -> %s (%d archivos):\n", *system, *out, len(written))
	for _, w := range written {
		fmt.Println("  +", w)
	}
	return 0
}

func cmdProxy(args []string) int {
	fs := flag.NewFlagSet("proxy", flag.ContinueOnError)
	port := fs.Int("port", 4000, "puerto")
	upstream := fs.String("upstream", "", "URL base upstream (vacio = mock)")
	mock := fs.Bool("mock", true, "respuestas mock deterministas sin API keys")
	logPath := fs.String("log", "autotier.log.jsonl", "log JSONL")
	mode := fs.String("mode", "cost", "cost|balance|intelligence")
	minTier := fs.String("min-tier", "", "piso de calidad (tier id)")
	maxCost := fs.Float64("max-cost", 0, "techo USD por request")
	agentsDir := fs.String("agents-dir", "agents", "registry de agentes nombrados")
	learnPath := fs.String("learn", "", "estado bandit JSON (aprende de outcomes)")
	apiKey := fs.String("api-key", "", "Bearer upstream (o env AUTOTIER_API_KEY|OPENAI_API_KEY)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	srv := proxy.New(proxy.Config{
		UpstreamURL: *upstream, Mock: *mock || *upstream == "",
		UpstreamAPIKey: apiKeyOrEnv(*apiKey),
		LogPath:        *logPath, Mode: policy.Mode(*mode),
		MinTier: *minTier, MaxCostUSD: *maxCost, AgentsDir: *agentsDir,
		LearnPath: *learnPath,
	})
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	httpSrv := &http.Server{Addr: addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	fmt.Printf("autotier %s proxy en http://%s (mode=%s mock=%v log=%s)\n", version, addr, *mode, *mock || *upstream == "", *logPath)
	fmt.Println("  POST /v1/chat/completions  (OpenAI: Codex, Cursor, Cline, Windsurf, Aider, Kimi)")
	fmt.Println("  POST /v1/messages          (Anthropic: Claude Code, Cline, Windsurf)")
	fmt.Println("  GET  /healthz")
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Println("error:", err)
		return 1
	}
	return 0
}

func cmdStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	logPath := fs.String("log", "autotier.log.jsonl", "log JSONL")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	recs, err := store.ReadAll(*logPath)
	if err != nil {
		fmt.Println("error:", err)
		return 1
	}
	s := store.Summarize(recs)
	fmt.Printf("requests:      %d\n", s.Requests)
	fmt.Printf("costo total:   $%.4f\n", s.TotalCostUSD)
	fmt.Printf("costo frontera:$%.4f\n", s.FrontierCost)
	fmt.Printf("ahorro:        %.1f%%\n", s.SavingsPct)
	fmt.Printf("escalations:   %d\n", s.Escalations)
	fmt.Printf("quality pass:  %.1f%%\n", s.QualityPassPct)
	fmt.Printf("p95 latencia:  %.0fms\n", s.P95LatencyMs)
	fmt.Println("por tier:")
	for _, t := range []string{"haiku-tier", "balanced-tier", "frontier-tier"} {
		fmt.Printf("  %-14s %d\n", t, s.ByTier[t])
	}
	if len(s.ByAgent) > 0 {
		fmt.Println("por agente:")
		for _, a := range []string{"explore", "build", "judge", "orchestrator"} {
			if n := s.ByAgent[a]; n > 0 {
				fmt.Printf("  @%-13s %d\n", a, n)
			}
		}
		for a, n := range s.ByAgent {
			switch a {
			case "explore", "build", "judge", "orchestrator":
			default:
				fmt.Printf("  @%-13s %d\n", a, n)
			}
		}
	}
	if len(s.ByDecider) > 0 {
		fmt.Println("por decisor:")
		for _, d := range []string{"rules", "learn:learned-cheaper", "learn:learned-escalate"} {
			if n := s.ByDecider[d]; n > 0 {
				fmt.Printf("  %-22s %d\n", d, n)
			}
		}
		for d, n := range s.ByDecider {
			switch d {
			case "rules", "learn:learned-cheaper", "learn:learned-escalate":
			default:
				fmt.Printf("  %-22s %d\n", d, n)
			}
		}
	}
	return 0
}

// apiKeyOrEnv resuelve --api-key o env (AUTOTIER_API_KEY, luego OPENAI_API_KEY).
func apiKeyOrEnv(flag string) string {
	if flag != "" {
		return flag
	}
	if k := os.Getenv("AUTOTIER_API_KEY"); k != "" {
		return k
	}
	return os.Getenv("OPENAI_API_KEY")
}

func cmdEval(args []string) int {
	fs := flag.NewFlagSet("eval", flag.ContinueOnError)
	suite := fs.String("suite", "evals/suite.yaml", "suite yaml")
	mode := fs.String("mode", "cost", "cost|balance|intelligence")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	cases, err := evals.ParseSuite(*suite)
	if err != nil {
		fmt.Println("error:", err)
		return 1
	}
	results := evals.Run(cases, policy.Mode(*mode))
	pass := 0
	for _, r := range results {
		mark := "OK  "
		if !r.Pass {
			mark = "FAIL"
		} else {
			pass++
		}
		fmt.Printf("%s %-18s domain=%-12s expect=%-13s got=%-13s c=%.2f\n",
			mark, r.Name, r.Domain, r.Expect, r.Got, r.Complexity)
	}
	fmt.Printf("%d/%d pass (mode=%s)\n", pass, len(results), *mode)
	if pass != len(results) {
		return 1
	}
	return 0
}
