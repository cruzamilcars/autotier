# dsh-plugin-autotier

Autotier model routing inside [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness)
(`dsh`): route agent work to cheap models first, escalate only on failure.

Exposes three model-facing tools (thin wrappers over the `autotier` binary —
all routing logic lives in Go, this package is only the bridge):

| Tool | Hace |
|---|---|
| `autotier_route` | Dry-run: qué tier merece un prompt, sin gastar ($0, 0 llamadas) |
| `autotier_call` | Ejecuta un agente nombrado (`@explore`, `@build`, `@judge`, `@orchestrator`) |
| `autotier_status` | Ahorro vs todo-frontera, escalations, quality pass |

## Requisitos

- `autotier` en el PATH (`go install github.com/cruzamilcars/autotier/cmd/autotier@latest`)
  o `AUTOTIER_BIN=/ruta/a/autotier`.
- Node 20+.

## Instalar

```bash
npm install dsh-plugin-autotier   # cuando se publique; hoy: npm install ./plugins/dsh-plugin-autotier
```

Registra el plugin en tu configuración de `dsh` según su documentación
(developer preview: la ruta de instalación puede cambiar, ver
[docs de dsh](https://deepseek-harness.github.io/deepseek-harness/)).
Uso típico en el harness: "usa autotier_route antes de trabajo caro".

Config del plugin (`binary`, `timeoutMs`, `cwd`):

```ts
apply(ctx, { binary: '/usr/local/bin/autotier', timeoutMs: 60000 })
```

## Verificar

```bash
npm run build    # tsc limpio
AUTOTIER_BIN=$(which autotier) npm test   # 5 smoke contra el binario real
```

## Nota de compatibilidad

`dsh` está en developer preview con breaking changes anunciados. Este plugin
usa solo la superficie estable documentada (`name`/`inject`/`apply`,
`ctx.tools.register(defineTool(...))`, `exec.signal`) para sobrevivirlos.
Si un release de `dsh` lo rompe, el contrato a revisar es ese y nada más.
