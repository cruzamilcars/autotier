package adapters

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Generate escribe plantillas nativas del sistema pedido en outDir.
// Devuelve las rutas escritas. Con force=false no sobrescribe.
func Generate(system, outDir string, force bool) ([]string, error) {
	files, err := templates(system)
	if err != nil {
		return nil, err
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

func Systems() []string {
	return []string{"opencode", "claude-code", "cursor", "cline", "aider", "windsurf", "codex", "antigravity"}
}

func templates(system string) (map[string]string, error) {
	switch system {
	case "opencode":
		return map[string]string{"opencode.autotier.json": opencodeJSON()}, nil
	case "claude-code":
		return map[string]string{
			".claude/agents/explore.md": claudeExplore(),
			".claude/agents/judge.md":   claudeJudge(),
		}, nil
	case "cursor":
		return map[string]string{
			".cursor/agents/explore.md": cursorExplore(),
			".cursor/rules/autotier.md": cursorRules(),
		}, nil
	case "cline":
		return map[string]string{".clinerules/autotier.yaml": clineRules()}, nil
	case "aider":
		return map[string]string{"aider.autotier.conf.yml": aiderConf()}, nil
	case "windsurf":
		return map[string]string{"windsurf.autotier.md": windsurfNotes()}, nil
	case "codex":
		return map[string]string{"codex.autotier.md": codexNotes()}, nil
	case "antigravity":
		return map[string]string{"antigravity.autotier.md": antigravityNotes()}, nil
	default:
		return nil, fmt.Errorf("sistema desconocido: %s (usa: opencode|claude-code|cursor|cline|aider|windsurf|codex|antigravity)", system)
	}
}

func opencodeJSON() string {
	return `{
  "$schema": "https://opencode.ai/config.json",
  "default_agent": "plan",
  "model": "anthropic/claude-sonnet-4-5",
  "small_model": "anthropic/claude-haiku-4-5",
  "subagent_depth": 1,
  "agent": {
    "plan": { "model": "anthropic/claude-haiku-4-5" },
    "build": { "model": "anthropic/claude-sonnet-4-5", "steps": 25 },
    "explore": {
      "description": "Exploracion read-only barata (grep, listar, leer logs)",
      "mode": "subagent",
      "model": "anthropic/claude-haiku-4-5",
      "steps": 8
    },
    "judge": {
      "description": "Juez frontera: arquitectura, seguridad, decisiones ambiguas",
      "mode": "subagent",
      "model": "anthropic/claude-opus-5",
      "steps": 15
    }
  }
}
`
}

func claudeExplore() string {
	return `---
name: explore
description: Busqueda read-only barata. Usar para grep, listar, leer logs.
model: haiku
tools: Read, Grep, Glob
---

Eres un explorador barato. Devuelve solo rutas + resumen de 5 lineas. Sin edicion. Agrupa 10 lookups por invocacion (un worker x 10 sale mas barato que 10 workers x 1).
`
}

func claudeJudge() string {
	return `---
name: judge
description: Juez frontera para arquitectura, seguridad y decisiones ambiguas.
model: opus
---

Decides el plan final a partir de resumenes de workers baratos. Se explicito en riesgos, rollback y tests. Si el resumen es insuficiente, pide una sola ronda extra acotada.
`
}

func cursorExplore() string {
	return `---
name: explore
description: Discovery barato, hereda el modelo del chat.
model: inherit
---

Busca y resume. No edites.
`
}

func cursorRules() string {
	return `# Router (replica abierta de Cursor Auto)
- Diario: Auto/Balance. Solo sube a Intelligence (frontera) en arquitectura, seguridad, migraciones, auth.
- Thinking models para planes y scans; modelos rapidos para ejecutar.
- Subagentes con 'model: inherit' siguen al chat; fija modelo solo donde ahorra (explore=barato, judge=frontera).
- Proxy local opcional: http://localhost:4000/v1 (OpenAI-compatible).
`
}

func clineRules() string {
	return `# Cline: Plan vs Act ya separan decision vs ejecucion.
mode_tiers:
  Plan: frontier-tier
  Act: balanced-tier
  Ask: haiku-tier
# Proveedores: apunta la API base a http://localhost:4000/v1 para resolver gratis/sano primero.
`
}

func aiderConf() string {
	return `# Aider ya trae el patron caro->barato nativo.
model: claude-opus-5
editor-model: claude-sonnet-4-6
weak-model: claude-haiku-4-5
architect: true
# Con proxy: --openai-api-base http://localhost:4000/v1
`
}

func windsurfNotes() string {
	return `# Windsurf Cascade via proxy
Apunta Cascade a http://localhost:4000 (OpenAI-compatible).

Ojo dialectos (bug real #102):
- kimi-k2 y kimi-k2-thinking -> kimi_k2 vLLM
- kimi-k2.5 / kimi-k2.6 -> openai_json_xml (si no, Cascade rechaza con "invalid tool call")

El proxy de este repo ya distingue el dialecto por modelo.
`
}

func codexNotes() string {
	return `# Codex CLI via proxy
export OPENAI_BASE_URL=http://localhost:4000/v1
# El proxy clasifica (complejidad+capacidad), resuelve proveedor gratis/sano primero
# e intenta barato primero con escalacion automatica (max 2).
# Sin proxy, Codex no tiene routing por agente: todo corre en --model.
`
}

func antigravityNotes() string {
	return `# Antigravity via proxy
Antigravity es cerrado: la unica via es interceptar la API.
Configura su endpoint OpenAI-compatible a http://localhost:4000/v1
y deja que el proxy haga classifier -> policy -> provider-resolver -> cascade.
`
}
