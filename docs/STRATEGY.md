# Estrategia autotier (documento vivo)

> Propósito: no perder el hilo. Las decisiones quedan con fecha y motivo.
> Puede evolucionar; lo que no puede es perderse por falta de contexto.

## Tesis (2026-09-10)

Ser la **capa de routing de modelos para todos los sistemas de agentes**,
no otro harness más. Suiza, no competidor.

## Las tres capas (no es un dilema, es una pila)

| Capa | Rol | Control | Costo | Estado |
|---|---|---|---|---|
| **MCP server** | Distribución universal: una integración funciona en Claude, OpenCode, Cursor, Cline... | Bajo (el modelo decide cuándo llamar) | Días (wrapper sobre lógica existente) | Siguiente |
| **Proxy** | Producto/núcleo: decide el modelo, ve cada token, mide y puede facturar. Único que entra a sistemas cerrados | Total | Hecho (v0-v3) | Núcleo, no se suelta |
| **Plugins nativos** | Profundidad (hooks pre-model-call por harness) | Alto por harness | Alto (N codebases × breaking changes) | Oportunistas |

Orden: MCP para alcance → proxy como producto → plugins donde el hook dé
poder real (OpenCode plugin, `dsh-plugin`).

## Harness propio / fork: NO (evaluado 2026-09-10)

`deepseek-ai/deepseek-harness` (`dsh`, "everything is a plugin"): 219k★,
25.9k forks, 16.5k commits, developer preview **con breaking changes
anunciados**, stack TS/pnpm monorepo (Cordis).

1. **Rebase hell**: hiperactividad + rupturas = fork obsoleto en semanas sin
   dedicación full-time.
2. **Stack ajeno**: arrastraría a TS/pnpm/Cordis contra Go-stdlib actual.
3. **Posicionamiento**: con harness propio competimos contra nuestros
   usuarios e integraciones.
4. **Capa equivocada**: TUI/tools/permisos es commodity; el routing se ahoga ahí.

Alternativa adoptada: **plugin PARA el gorila, no fork DEL gorila**
(`dsh-plugin` es su canal oficial de discoverability). Y como máximo, un
harness de referencia mínimo contra nuestro proxy como demo.

## Decision log

- 2026-09-09: nombre `autotier` (modelrouter saturado, automodel colisiona con HF/NVIDIA).
- 2026-09-09: Go stdlib, cero dependencias (binario offline, CI reproducible).
- 2026-09-09: proxy primero (control) antes que plugins (alcance).
- 2026-09-09: tier fijo de agente = pin exacto; cascade como red (no subir preventivo).
- 2026-09-09: `deny` y `pin` explícitos, fail-closed (nunca override silencioso).
- 2026-09-10: no fork de harnesses; MCP + plugins delgados.
- 2026-09-10: evidencia de benchmarks solo con fuente+fecha; exclusiones documentadas.

## Preguntas abiertas

- Monetización: ¿proxy hosteado con metering o siempre self-hosted?
- `learn` cruzado entre usuarios (federado) vs local-only actual.
- Pesos del blend prior/evidencia por dominio (hoy 1:1 global).
