package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cruzamilcars/autotier/internal/mcp"
	"github.com/cruzamilcars/autotier/internal/policy"
	"github.com/cruzamilcars/autotier/internal/proxy"
)

func cmdMCP(args []string) int {
	if len(args) == 0 || args[0] != "serve" {
		fmt.Println("uso: autotier mcp serve [--mock] [--upstream URL] [--api-key KEY] [--agents-dir agents] [--log autotier.log.jsonl] [--mode cost] [--learn learn.json]")
		return 1
	}
	fs := flag.NewFlagSet("mcp serve", flag.ContinueOnError)
	upstream := fs.String("upstream", "", "URL base upstream (vacio = mock)")
	mock := fs.Bool("mock", true, "mock determinista sin API keys")
	apiKey := fs.String("api-key", "", "Bearer upstream (o env AUTOTIER_API_KEY|OPENAI_API_KEY)")
	agentsDir := fs.String("agents-dir", "agents", "registry de agentes")
	logPath := fs.String("log", "autotier.log.jsonl", "log JSONL")
	mode := fs.String("mode", "cost", "cost|balance|intelligence")
	learnPath := fs.String("learn", "", "estado bandit JSON")
	denyS := fs.String("deny", "", "veto global de tiers (fail closed)")
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}
	deny, err := policy.ParseDeny(*denyS)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	srv := mcp.New(proxy.Config{
		UpstreamURL: *upstream, Mock: *mock || *upstream == "",
		UpstreamAPIKey: apiKeyOrEnv(*apiKey),
		LogPath:        *logPath, Mode: policy.Mode(*mode),
		AgentsDir: *agentsDir, LearnPath: *learnPath, Deny: deny,
	})
	fmt.Fprintln(os.Stderr, "autotier mcp serve (stdio)")
	if err := srv.Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}
