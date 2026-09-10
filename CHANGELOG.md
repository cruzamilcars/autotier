# Changelog

## Unreleased (v3)

- Bench: MMMU->multimodal y OCRBench->ocr (con URLs y fechas); recencia con
  decaimiento exponencial (`--as-of`, `--recency-half-life`, `--recency-min`).
- El blend parte de priors curados (`Priors()`), sin ratchet entre imports.
- Cambios aplicados: haiku multimodal 5→7, balanced ocr 8→9.

## v0.1.0 — 2026-09-09

Router universal de modelos para sistemas de agentes + agentes nombrados +
aprendizaje Thompson Sampling + ingesta de benchmarks.

### Verificación (Go 1.27.1, todo ejecutado)

- `go vet` limpio, `gofmt` limpio, `go test -count=1 ./...` OK (9 pkgs con tests).
- `eval` 6/6 en cost: trivial->haiku (0.05), auth+threat-model+migracion->frontier
  (0.75), refactor+tests->balanced (0.50), landing+lanzamiento->balanced (0.40).
- Proxy mock: trivial->haiku 80% ahorro; `fail-first`->escala a balanced (1 esc,
  40% ahorro); `/v1/messages`->frontier en prompt arquitectura; `/v1/models` + SSE OK.
- **OpenCode real**: `opencode run` contra el proxy completa el loop
  (step_finish `stop`, texto fluye). Hallazgo: sin `finish_reason: stop` en el
  stream, OpenCode repetía la llamada en loop infinito (`reason: unknown`,
  0 tokens); corregido + test de regresión. Los reintentos idénticos ahora se
  ven como `cache hits` en el log en vez de ser invisibles.
- Agentes: `orchestrator "@explore @judge"` delega y sintetiza; denegación
  `@explore->@judge` falla auditable; pin fijo verificado; handoff `--to judge`;
  `X-Autotier-Agent` y `@menciones` sobre HTTP OK.
- Learn: tras 12 fallos de haiku, prompt trivial rutea directo a balanced con
  0 escalaciones (`learn:learned-escalate`); `status` desglosa por decisor.
- Bench: SWE-bench Verified + GPQA Diamond (Sep 2026) -> haiku code 6→7,
  frontier code/reasoning 10→9, haiku reasoning 5→6; 1 test actualizado
  (`TestBudgetCap`) por el cambio de elegibilidad resultante.
- `init --with-agents` genera 4 subagentes Claude + `opencode.autotier.json` válido.
