# autotier

[![CI](https://github.com/cruzamilcars/autotier/actions/workflows/ci.yml/badge.svg)](https://github.com/cruzamilcars/autotier/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.23+-blue.svg)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

**80% de ahorro en tareas triviales de agentes de código, sin perder calidad.**
Clasifica complejidad en <2ms (sin llamada LLM extra), rutea al modelo barato
primero y escala solo si falla. Aprende con Thompson Sampling. Verificado con
eval suite 6/6 y loop real contra OpenCode.

> Problema: en OpenCode, Claude Code, Codex, Cursor, Cline, Windsurf, Aider
> debes elegir el modelo a mano — pagas frontier donde un barato resuelve igual.
> Solución: `orquestador capaz -> workers baratos -> juez capaz`, automático.

## Sistemas soportados

| Sistema | Cómo se integra |
|---|---|
| OpenCode | Provider custom al proxy (verificado end-to-end) + `agent.<id>.model`, `small_model` |
| Claude Code | Subagentes exportados (`model: haiku/sonnet/opus/inherit`) + proxy `/v1/messages` |
| Cursor | Réplica abierta del Auto Router (Cost/Balance/Intelligence) |
| Cline | Modos Plan/Act/Ask + proxy |
| Windsurf Cascade | Vía proxy (dialectos Kimi switching incluidos) |
| Aider | `--model/--editor-model/--weak-model` + `OPENAI_BASE_URL` al proxy |
| Codex CLI, Antigravity, Kimi | Vía proxy OpenAI-compatible |

## Quickstart (2 minutos, sin API keys)

```bash
go run ./cmd/autotier eval --suite evals/suite.yaml
# 6/6 pass (mode=cost)

go run ./cmd/autotier agent call orchestrator "Prepara el cambio de login @explore @judge"
# @orchestrator [haiku-tier] c=0.20 ... ahorro=80%
#   └─ @explore [haiku-tier] ...
#   └─ @judge [frontier-tier] ...

go run ./cmd/autotier proxy --port 4000   # mock determinista
curl localhost:4000/v1/chat/completions -d '{"model":"auto","messages":[{"role":"user","content":"busca donde se define login"}]}'
# {"autotier":{"tier":"haiku-tier",...,"savings_pct":80,...}}

go run ./cmd/autotier status              # costos, savings_pct, escalations, por agente
```

OpenCode contra el proxy (verificado con `opencode run` real):

```json
{ "$schema": "https://opencode.ai/config.json",
  "model": "autotier/auto", "small_model": "autotier/auto",
  "provider": { "autotier": {
    "npm": "@ai-sdk/openai-compatible", "name": "Autotier (local)",
    "options": { "baseURL": "http://127.0.0.1:4000/v1" },
    "models": { "auto": { "name": "Autotier Auto",
      "limit": { "context": 200000, "output": 65536 } } } } } }
```

## Agentes nombrados

```bash
go run ./cmd/autotier agent list
go run ./cmd/autotier agent call --to judge explore "lista los archivos de auth"  # handoff
go run ./cmd/autotier init --system claude-code --with-agents --out . --force    # exporta @mencionables
```

Define los tuyos en `agents/*.md` (frontmatter: tier, delegates, max_steps).
Tier fijo = pin exacto; `auto` = decide el router. Delegar fuera de la
allowlist falla con error (no silencioso). Flags antes de posicionales.

## Aprendizaje (Thompson Sampling)

```bash
go run ./cmd/autotier proxy --learn learn.json              # aprende de cada outcome
go run ./cmd/autotier learn status --learn learn.json       # brazos tier|dominio: pulls, wins, media
```

Sin evidencia mandan las reglas (cold start determinista, el eval no cambia).
Con >=5 muestras el router baja (`learned-cheaper`) o sube (`learned-escalate`)
de tier; el pin de agente fijo siempre gana. Ver `CHANGELOG.md` por evidencia.

## Upstream real

```bash
go run ./cmd/autotier proxy --upstream https://api.openrouter.ai/v1 --api-key $env:OPENROUTER_KEY --mock=false
# o env AUTOTIER_API_KEY / OPENAI_API_KEY
```

## Benchmarks al catálogo

```bash
go run ./cmd/autotier bench report                              # deltas evidencia vs prior
go run ./cmd/autotier bench import --write                      # aplica a capability-catalog.yaml + catalog.gen.go
```

Fuentes con fecha y URL en `internal/bench/data/` (SWE-bench Verified, GPQA
Diamond, MMMU, OCRBench). Nada se aplica sin `--write`; las exclusiones van
documentadas. Recencia: el peso decae con la edad
(`--as-of`, `--recency-half-life` 180d, `--recency-min` 0.2).

## Estructura

- `agents/` — registry de agentes nombrados (define una vez, exporta a todos).
- `internal/catalog/` — capacidades por dominio + overlay de evidencia (`bench import`).
- `internal/classifier/` — score complejidad 0-1 por reglas (<2ms, sin LLM extra).
- `internal/policy/tiers.yaml` — simple→barato, medio→equilibrado, crítico→frontera.
- `internal/learn/` — bandit Thompson Sampling sobre outcomes.
- `internal/bench/` — ingesta de benchmarks públicos con procedencia.
- `internal/proxy/` — gateway OpenAI + Anthropic (`/v1/models`, SSE, caché, learn).
- `adapters/` — plantillas nativas por sistema.

Ver `docs/ARCHITECTURE.md` (diseño), `CHANGELOG.md` (verificación) y
`CONTRIBUTING.md` (cómo mandar adapters).
