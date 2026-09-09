package adapters

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cruzamilcars/autotier/internal/agents"
	"github.com/cruzamilcars/autotier/internal/catalog"
)

// GenerateAgents exporta el registry agents/ a subagentes nativos del sistema.
// Define una vez en agents/*.md, despliega en todos: @mencionables en
// Claude Code / OpenCode / Cursor, via proxy en el resto.
func GenerateAgents(system, registryDir, outDir string, force bool) ([]string, error) {
	reg, err := agents.Load(registryDir)
	if err != nil {
		return nil, err
	}
	var files map[string]string
	switch system {
	case "claude-code":
		files = claudeAgentsFiles(reg)
	case "opencode":
		files, err = opencodeAgentsFile(reg)
		if err != nil {
			return nil, err
		}
	case "cursor":
		files = cursorAgentsFiles(reg)
	case "cline", "aider", "windsurf", "codex", "antigravity":
		files = proxyAgentsNote(system, reg)
	default:
		return nil, fmt.Errorf("sistema desconocido: %s", system)
	}
	var written []string
	for rel, content := range files {
		dst := filepath.Join(outDir, rel)
		if _, err := os.Stat(dst); err == nil && !force {
			return written, fmt.Errorf("existe %s (usa --force para sobrescribir)", dst)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return written, err
		}
		if err := os.WriteFile(dst, []byte(content), 0644); err != nil {
			return written, err
		}
		written = append(written, dst)
	}
	sort.Strings(written)
	return written, nil
}

// Tier -> alias de modelo por sistema.
func claudeModelAlias(tier string) string {
	switch tier {
	case catalog.TierHaiku:
		return "haiku"
	case catalog.TierBalanced:
		return "sonnet"
	case catalog.TierFrontier:
		return "opus"
	default:
		return "inherit" // auto: sigue al modelo de la sesion
	}
}

func opencodeModelID(tier string) string {
	switch tier {
	case catalog.TierHaiku:
		return "anthropic/claude-haiku-4-5"
	case catalog.TierBalanced:
		return "anthropic/claude-sonnet-4-5"
	case catalog.TierFrontier:
		return "anthropic/claude-opus-5"
	default:
		return "" // auto: hereda
	}
}

func agentFrontmatter(a agents.Agent, modelLine string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("name: " + a.Name + "\n")
	if a.Description != "" {
		sb.WriteString("description: " + a.Description + "\n")
	}
	if modelLine != "" {
		sb.WriteString(modelLine + "\n")
	}
	if len(a.Delegates) > 0 {
		sb.WriteString("delegates: [" + strings.Join(a.Delegates, ", ") + "]\n")
	}
	sb.WriteString("---\n\n")
	sb.WriteString(a.System + "\n")
	if len(a.Delegates) > 0 {
		sb.WriteString("\nPuedes delegar en: " + strings.Join(a.Delegates, ", ") + ".\n")
	}
	return sb.String()
}

func claudeAgentsFiles(reg map[string]agents.Agent) map[string]string {
	files := map[string]string{}
	for _, n := range agents.Names(reg) {
		a := reg[n]
		files[".claude/agents/"+a.Name+".md"] =
			agentFrontmatter(a, "model: "+claudeModelAlias(a.Tier))
	}
	return files
}

func cursorAgentsFiles(reg map[string]agents.Agent) map[string]string {
	files := map[string]string{
		".cursor/rules/autotier.md": cursorRules(),
	}
	for _, n := range agents.Names(reg) {
		a := reg[n]
		files[".cursor/agents/"+a.Name+".md"] =
			agentFrontmatter(a, "model: "+claudeModelAlias(a.Tier))
	}
	return files
}

func opencodeAgentsFile(reg map[string]agents.Agent) (map[string]string, error) {
	type agentCfg struct {
		Description string `json:"description,omitempty"`
		Mode        string `json:"mode"`
		Model       string `json:"model,omitempty"`
		Steps       int    `json:"steps,omitempty"`
	}
	cfg := map[string]any{
		"$schema":        "https://opencode.ai/config.json",
		"default_agent":  "plan",
		"model":          "anthropic/claude-sonnet-4-5",
		"small_model":    "anthropic/claude-haiku-4-5",
		"subagent_depth": 1,
	}
	agentMap := map[string]agentCfg{
		"plan":  {Mode: "primary", Model: "anthropic/claude-haiku-4-5"},
		"build": {Mode: "primary", Model: "anthropic/claude-sonnet-4-5", Steps: 25},
	}
	for _, n := range agents.Names(reg) {
		a := reg[n]
		if a.Name == "orchestrator" {
			continue // el primario cubre ese rol
		}
		c := agentCfg{Description: a.Description, Mode: "subagent", Steps: a.MaxSteps}
		if m := opencodeModelID(a.Tier); m != "" {
			c.Model = m
		}
		agentMap[a.Name] = c
	}
	cfg["agent"] = agentMap
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	return map[string]string{"opencode.autotier.json": string(b) + "\n"}, nil
}

func proxyAgentsNote(system string, reg map[string]agents.Agent) map[string]string {
	var sb strings.Builder
	sb.WriteString("# " + system + " via proxy con agentes nombrados\n\n")
	sb.WriteString("Este sistema no tiene subagentes nativos con modelo propio: apunta su\n")
	sb.WriteString("endpoint OpenAI-compatible a http://127.0.0.1:4000/v1 y elige agente con:\n\n")
	sb.WriteString("  header `X-Autotier-Agent: <nombre>` o campo `\"agent\": \"<nombre>\"`\n\n")
	sb.WriteString("Agentes disponibles:\n\n")
	for _, n := range agents.Names(reg) {
		a := reg[n]
		fmt.Fprintf(&sb, "- `@%s` [%s] — %s\n", a.Name, a.Tier, a.Description)
	}
	sb.WriteString("\nMenciones `@otro` en el prompt delegan si el agente lo permite (`delegates`).\n")
	return map[string]string{system + ".agents.md": sb.String()}
}
