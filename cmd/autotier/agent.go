package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/cruzamilcars/autotier/internal/agents"
	"github.com/cruzamilcars/autotier/internal/policy"
	"github.com/cruzamilcars/autotier/internal/proxy"
)

func cmdAgent(args []string) int {
	if len(args) == 0 {
		fmt.Println("uso: autotier agent <list|show|call> ...")
		return 1
	}
	switch args[0] {
	case "list":
		return agentList(args[1:])
	case "show":
		return agentShow(args[1:])
	case "call":
		return agentCall(args[1:])
	default:
		fmt.Println("subcomando desconocido:", args[0])
		return 1
	}
}

func loadRegistry(dir string) (map[string]agents.Agent, int) {
	if dir == "" {
		dir = agents.DefaultDir
	}
	reg, err := agents.Load(dir)
	if err != nil {
		fmt.Println("error:", err)
		return nil, 1
	}
	return reg, 0
}

func agentList(args []string) int {
	fs := flag.NewFlagSet("agent list", flag.ContinueOnError)
	dir := fs.String("agents-dir", agents.DefaultDir, "registry")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	reg, code := loadRegistry(*dir)
	if code != 0 {
		return code
	}
	fmt.Printf("%-14s %-12s %s\n", "NOMBRE", "TIER", "DESCRIPCION")
	for _, n := range agents.Names(reg) {
		a := reg[n]
		fmt.Printf("@%-13s %-12s %s\n", a.Name, a.Tier, a.Description)
	}
	return 0
}

func agentShow(args []string) int {
	fs := flag.NewFlagSet("agent show", flag.ContinueOnError)
	dir := fs.String("agents-dir", agents.DefaultDir, "registry")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Println("uso: autotier agent show <nombre>")
		return 1
	}
	reg, code := loadRegistry(*dir)
	if code != 0 {
		return code
	}
	a, ok := reg[strings.ToLower(rest[0])]
	if !ok {
		fmt.Printf("agente desconocido (disponibles: %s)\n", strings.Join(agents.Names(reg), ", "))
		return 1
	}
	fmt.Printf("nombre:      @%s\ntier:        %s\ndelegates:   [%s]\nmax_steps:   %d\nmax_depth:   %d\norigen:      %s\n--- system ---\n%s\n",
		a.Name, a.Tier, strings.Join(a.Delegates, ", "), a.MaxSteps, a.MaxDepth, a.Source, a.System)
	return 0
}

func agentCall(args []string) int {
	fs := flag.NewFlagSet("agent call", flag.ContinueOnError)
	dir := fs.String("agents-dir", agents.DefaultDir, "registry")
	contextFile := fs.String("context", "", "handoff fork: archivo con conversacion previa")
	to := fs.String("to", "", "handoff: ejecutar con otro agente heredando el resultado (fork)")
	upstream := fs.String("upstream", "", "URL base upstream (vacio = mock)")
	mock := fs.Bool("mock", true, "mock determinista sin API keys")
	logPath := fs.String("log", "autotier.log.jsonl", "log JSONL")
	mode := fs.String("mode", "cost", "cost|balance|intelligence")
	maxTokens := fs.Int("max-tokens", 512, "tope output")
	learnPath := fs.String("learn", "", "estado bandit JSON (aprende de outcomes)")
	apiKey := fs.String("api-key", "", "Bearer upstream (o env AUTOTIER_API_KEY|OPENAI_API_KEY)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	rest := fs.Args()
	if len(rest) < 2 {
		fmt.Println("uso: autotier agent call [--log f] [--to otro] [--context f] <nombre> \"tarea con @menciones\"")
		return 1
	}
	name, task := strings.ToLower(rest[0]), rest[1]
	reg, code := loadRegistry(*dir)
	if code != 0 {
		return code
	}
	srv := proxy.New(proxy.Config{
		UpstreamURL: *upstream, Mock: *mock || *upstream == "",
		UpstreamAPIKey: apiKeyOrEnv(*apiKey),
		LogPath:        *logPath, Mode: policy.Mode(*mode), AgentsDir: *dir,
		LearnPath: *learnPath,
	})
	call := func(n, t, ctx, parent string, depth int) (proxy.AgentResult, error) {
		return srv.RunAgent(reg, n, t, proxy.AgentCallOpts{
			Context: ctx, Parent: parent, Depth: depth,
			MaxTokens: *maxTokens, Mode: policy.Mode(*mode),
		})
	}
	var ctx string
	if *contextFile != "" {
		b, err := os.ReadFile(*contextFile)
		if err != nil {
			fmt.Println("error:", err)
			return 1
		}
		ctx = string(b)
	}
	res, err := call(name, task, ctx, "", 0)
	if err != nil {
		fmt.Println("error:", err)
		return 1
	}
	printAgentResult(res, "")
	// Handoff --to: el resultado pasa como contexto al siguiente agente.
	if *to != "" {
		fmt.Printf("\n--- handoff @%s -> @%s ---\n\n", res.Agent, *to)
		res2, err := call(*to, task, res.Text, res.Agent, 0)
		if err != nil {
			fmt.Println("error:", err)
			return 1
		}
		printAgentResult(res2, "")
	}
	return 0
}

func printAgentResult(res proxy.AgentResult, indent string) {
	fmt.Printf("%s@%s [%s] c=%.2f domain=%s esc=%d $%.6f ahorro=%.0f%% via=%s\n",
		indent, res.Agent, res.Tier, res.Complexity, res.Domain,
		res.Escalations, res.CostUSD, res.SavingsPct, res.Decider)
	for _, s := range res.Subs {
		fmt.Printf("%s  └─ @%s [%s] esc=%d $%.6f\n", indent, s.Agent, s.Tier, s.Escalations, s.CostUSD)
	}
	fmt.Printf("%s---\n%s%s\n", indent, indent, res.Text)
}
