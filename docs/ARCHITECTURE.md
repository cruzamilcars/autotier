# Arquitectura: proxy universal + adapters nativos

## Por que ambos

- **Adapters nativos** (OpenCode, Claude Code, Cursor subagents, Aider, Cline):
  el subagente con modelo propio tiene cache aislada; el padre no se invalida.
  Es la via mas barata donde el sistema la soporta.
- **Proxy** (Codex, Antigravity, Windsurf Cascade, Cursor chat, Kimi y
  cualquier cliente OpenAI/Anthropic-compatible): la unica via en sistemas
  cerrados o sin routing por agente.

## Flujo por request

```
prompt -> classifier (<2ms, reglas, sin LLM extra)
  -> policy (complejidad x capacidad-catalogo x modo cost|balance|intelligence
             + guardarrails min_tier / max_cost_usd + budget_exceeded)
  -> provider-resolver (gratis/sano/con-quota primero; circuit-breaker)
  -> executor (tier barato primero)
  -> quality-gate (too_short | hedging | tests_failed)
  -> cascade: escala 1 rung si falla (max 2 escalaciones)
  -> log JSONL -> status (costo, savings_pct vs all-frontier, p95, pass%)
```

## Decisiones calibradas (auditoria 2026-09-09)

- Criticas apilables: 1ra keyword +0.35, extras +0.1 (tope +0.55).
  "auth + threat model + migracion" = 0.75 -> frontera aunque el prompt sea corto.
- Build/test (refactor, tests, deploy) +0.15 -> balanced.
- Stakes (lanzamiento, prod, pitch) +0.2 -> sube un rung.
- Triviales (busca, lista, resume, grep) -0.15.
- **El piso de calidad manda sobre el techo de costo**: si `max_cost_usd` no
  alcanza ni al elegible mas barato, se usa ese igual y se marca
  `budget_exceeded=true` en vez de degradar a un modelo incapaz.

## Modos (como Cursor Auto)

| modo | quality_floor | sesgo |
|---|---|---|
| cost | 6 | barato bueno-suficiente |
| balance | 7 | equilibrado diario |
| intelligence | 8 | frontera donde decide |

## Matriz de sistemas

| Sistema | Mecanismo nativo | Adapter |
|---|---|---|
| OpenCode | `agent.<id>.model`, `small_model`, `steps`, `permission.task` | `autotier init --system opencode` |
| Claude Code | frontmatter `model: haiku/sonnet/opus/inherit`, `opusplan` | `autotier init --system claude-code` |
| Cursor | Auto Router (cerrado, Teams/Enterprise) + `model: inherit` | replica abierta |
| Cline | modos Plan/Act/Ask + multi-provider | Plan=frontera, Act=balanced, Ask=haiku |
| Aider | `--model` + `--editor-model` + `--weak-model` + architect | config generada |
| Windsurf | Cascade dropdown; dialectos por modelo | via proxy (ojo Kimi #102) |
| Codex CLI | `--model` global, sin routing por agente | via proxy `OPENAI_BASE_URL` |
| Antigravity | cerrado | via proxy |
| Kimi/Moonshot | `kimi-k2`=vLLM, `k2.5/k2.6`=openai_json_xml | resolver distingue dialecto |

## Anti-patrones

- No cambiar modelo mid-session (rompe cache, re-paga historial).
- No over-fanout: cada subagente cuesta ~25-35k tokens de init.
- Batch: 1 worker x 10 lookups < 10 workers x 1 lookup.
- Allowlist silenciosa: si la org bloquea haiku, el worker corre caro sin error.

## Agentes nombrados (agents/ + `autotier agent`)

Registry portable Markdown+frontmatter (`agents/<nombre>.md`, misma convencion
que Claude Code): nombre, descripcion, tier fijo o `auto`, system prompt +
contexto precargado, `delegates` (allowlist), `max_steps`, `max_depth`.

Tres invocaciones (como Claude Code):
- `autotier agent call <nombre> "tarea"` — explicita, como @-mention.
- `@otro` dentro de la tarea — sub-llamada si el agente lo lista en delegates
  (si no: error auditable, ej. `@explore no puede delegar a @judge`).
- `--context f` / `--to otro` — handoff fork: hereda resultado sin re-explicar.

Sobre HTTP: campo `"agent"` o header `X-Autotier-Agent`; `@menciones` sueltas
usan `orchestrator`. Tier fijo = PIN exacto (como `model:` en Claude);
la red de seguridad es el cascade, no subir preventivamente.

`autotier init --system X --with-agents` exporta el registry a subagentes
nativos (@mencionables en Claude/OpenCode/Cursor; nota proxy en el resto).

## Roadmap aprendizaje

- v0 (hecho): reglas deterministas + catalogo curado + evals.
- v1 (hecho): Thompson Sampling sobre el log (`--learn learn.json`):
  brazos tier|dominio con prior optimista Beta(2,1); override de reglas solo
  con >=5 muestras; `learn status|reset`; campo `decider` en log/status/respuesta.
- v2: ingesta de benchmarks (SWE-bench, GPQA, OCRBench) al catalogo con evidencia.
