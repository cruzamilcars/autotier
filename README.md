# autotier

Router inteligente y universal de modelos para sistemas de agentes AI.

Problema: en OpenCode, Claude Code, Codex, Antigravity, Cursor, Cline, Windsurf, Aider, Kimi debes elegir el modelo a mano. Pagas frontier para tareas que un modelo barato resuelve igual, o usas barato donde necesitas inteligencia.

Solución: `orquestador capaz -> workers baratos -> juez capaz`, con decisión automática por `capacidad x costo x ventana x tools x límites de proveedor`, más aprendizaje continuo.

> Inspirado en patrones validados: Cursor Router (Auto Cost/Balance/Intelligence), Claude `opusplan`, Aider `--weak-model`, OpenCode `agent.model + small_model`.

## Principio

- No cambiar el modelo principal mid-session (invalida caché, re-pagas historial).
- Delegar a subagentes/workers con modelo propio (caché aislada).
- Cascade: intenta barato primero, escala si el quality-gate falla.
- Mismo modelo en N proveedores → elige gratis/sano con cuota.

## Quickstart

```bash
go run ./cmd/autotier init --system opencode --out . --force
go run ./cmd/autotier init --system claude-code --out . --force
go run ./cmd/autotier eval --suite evals/suite.yaml   # 6/6 pass esperado
go run ./cmd/autotier proxy --port 4000               # mock determinista, sin API keys
go run ./cmd/autotier status                          # costos, savings_pct, escalations
```

Conectar sistemas cerrados al proxy:

```bash
export OPENAI_BASE_URL=http://127.0.0.1:4000/v1   # Codex, Aider, Cline, Windsurf
# POST /v1/chat/completions (OpenAI) y POST /v1/messages (Anthropic)
# Demo cascade: header X-Autotier-Force: fail-first (1er intento pobre -> escala)
```

## Agentes nombrados

```bash
go run ./cmd/autotier agent list
go run ./cmd/autotier agent call orchestrator "Prepara el cambio de login @explore @judge"
go run ./cmd/autotier agent call --to judge explore "lista los archivos de auth"  # handoff
go run ./cmd/autotier init --system claude-code --with-agents --out . --force    # exporta @mencionables
curl -H 'X-Autotier-Agent: judge' localhost:4000/v1/chat/completions -d '{"model":"auto","messages":[{"role":"user","content":"decide el plan"}]}'
```

Define los tuyos en `agents/*.md` (frontmatter: tier, delegates, max_steps).
Tier fijo = pin exacto; `auto` = decide el router. Delegar fuera de la
allowlist falla con error (no silencioso). Flags antes de posicionales.

## Verificacion (2026-09-09, Go 1.27.1 portable, todo ejecutado)

- `go vet` limpio, `gofmt` limpio, `go test -count=1 ./...` OK (5 pkgs con tests).
- `eval` 6/6 en cost: trivial->haiku (0.05), auth+threat-model+migracion->frontier
  (0.75), refactor+tests->balanced (0.50), landing+lanzamiento->balanced (0.40).
- Proxy mock: trivial->haiku 80% ahorro; `fail-first`->escala a balanced (1 esc,
  40% ahorro); `/v1/messages`->frontier en prompt arquitectura.
- Agentes: `orchestrator "@explore @judge"` delega y sintetiza; denegacion
  `@explore->@judge` falla auditable; pin fijo verificado (explore siempre
  haiku, judge siempre frontier); handoff `--to judge` hereda contexto;
  `X-Autotier-Agent: judge` y `@menciones` sobre HTTP OK.
- `init --with-agents` genera 4 subagentes Claude + `opencode.autotier.json`
  valido (JSON parseado).
- Bugs hallados y corregidos: criticas cortas sub-puntuadas; `max_cost_usd`
  podia degradar bajo el piso (ahora `budget_exceeded`); tier fijo subia por
  elegibilidad (ahora pin exacto + cascade como red); flags despues de
  posicionales no se parsean (documentado); `dataKeywords` duplicado.

## Estructura

- `agents/` — registry de agentes nombrados (define una vez, exporta a todos).
- `internal/catalog/capability-catalog.yaml` — qué modelo es bueno en qué: code, reasoning, research, presentation, design, data-analysis, ocr, multimodal, tool-use + ventana + $/1M + latencia.
- `internal/classifier/` — score complejidad 0-1 por reglas (<2ms, sin LLM extra).
- `internal/policy/tiers.yaml` — simple→barato, medio→equilibrado, crítico→frontera + `min_tier` / `max_cost_usd`.
- `internal/provider/` — resolver multi-proveedor: costo, salud, cuota, failover, caché.
- `adapters/` — plantillas nativas por sistema (ver matriz abajo).
- `evals/suite.yaml` — suite para probar que barato no rompe calidad.

## Matriz de sistemas

| Sistema | Mecanismo nativo | Adapter en este repo |
|---|---|---|
| OpenCode | `agent.<nombre>.model`, `small_model`, `steps`, `permission.task` | `adapters/opencode.json.example` |
| Claude Code | frontmatter `model: haiku/sonnet/opus/inherit`, `opusplan` | `adapters/claude-agents/` |
| Cursor | Auto Router (Cost/Balance/Intelligence, solo Teams/Enterprise) + `model: inherit` en subagentes | `adapters/cursor/` replica lógica en abierto |
| Cline | multi-provider, `.clinerules`, modos Plan/Act | `adapters/cline/` |
| Windsurf Cascade | dropdown + créditos, parser por dialecto (ojo Kimi K2 vs K2.5) | `adapters/windsurf/` vía proxy |
| Aider | `--model` (main) + `--weak-model` + `--editor-model` + architect mode | `adapters/aider/` |
| Codex CLI | `--model`, effort | vía `proxy` |
| Antigravity | cerrado | vía `proxy` |
| Kimi / Moonshot | `kimi-k2` usa dialecto vLLM, `k2.5/k2.6` usan openai_json_xml | `provider` lo distingue |

Ver `docs/ARCHITECTURE.md` para el diseño completo.
