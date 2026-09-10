import type { Context } from '@deepseek-ai/cordis'
import { defineTool } from '@deepseek-ai/dsh-tools'
import { callArgs, routeArgs, runBinary, statusArgs, type RunnerOptions } from './runner.js'

export const name = 'autotier'

export const inject = ['tools']

export interface AutotierConfig extends RunnerOptions {}

const DEFAULTS: Required<RunnerOptions> = {
  binary: '',
  timeoutMs: 120000,
  cwd: '',
}

function cfgOf(overrides?: AutotierConfig): RunnerOptions {
  return {
    binary: overrides?.binary || process.env.AUTOTIER_BIN || DEFAULTS.binary,
    timeoutMs: overrides?.timeoutMs ?? DEFAULTS.timeoutMs,
    cwd: overrides?.cwd || process.cwd(),
  }
}

const asText = {
  schema: { type: 'string' as const },
  render: (_args: unknown, value: string) => [{ type: 'text' as const, text: value }],
}

/**
 * Autotier routing inside DeepSeek Harness: route agent work to cheap models
 * first, escalate only on failure. Shells out to the `autotier` binary
 * (install: `go install github.com/cruzamilcars/autotier/cmd/autotier@latest`).
 */
export function apply(ctx: Context, overrides?: AutotierConfig) {
  const cfg = cfgOf(overrides)

  ctx.tools.register(
    defineTool({
      name: 'autotier_route',
      description:
        'Decide which model tier (haiku-tier|balanced-tier|frontier-tier) a prompt deserves, WITHOUT spending anything. Shows complexity, domain, eligible tiers, estimated cost and savings vs frontier. Use before expensive work.',
      parameters: {
        prompt: { type: 'string', required: true, description: 'The task prompt to route' },
        mode: { type: 'string', description: "'cost' (default), 'balance' or 'intelligence'" },
        deny: { type: 'string', description: "Vetoed tiers, e.g. 'frontier-tier'" },
      },
      output: asText,
      async execute(args, exec) {
        const mode = typeof args.mode === 'string' && args.mode ? args.mode : 'cost'
        const deny = typeof args.deny === 'string' ? args.deny : undefined
        const { stdout } = await runBinary(routeArgs(args.prompt, { mode, deny }), { ...cfg, signal: exec.signal })
        return stdout.trimEnd().slice(0, 6000)
      },
    }),
  )

  ctx.tools.register(
    defineTool({
      name: 'autotier_call',
      description:
        'Run a named autotier agent (@explore cheap search, @build balanced implementation, @judge frontier decisions, @orchestrator auto fan-out). The task may contain @mentions to delegate when the agent allows it.',
      parameters: {
        agent: { type: 'string', required: true, description: 'Agent name without @' },
        task: { type: 'string', required: true, description: 'Task text, may contain @mentions' },
      },
      output: asText,
      async execute(args, exec) {
        const { stdout } = await runBinary(callArgs(args.agent, args.task), { ...cfg, signal: exec.signal })
        return stdout.trimEnd().slice(0, 8000)
      },
    }),
  )

  ctx.tools.register(
    defineTool({
      name: 'autotier_status',
      description:
        'Spending summary of the autotier log: savings vs all-frontier, escalations, quality pass rate, per tier/agent/decider. Answers whether routing is actually helping.',
      parameters: {},
      output: asText,
      async execute(_args, exec) {
        const { stdout } = await runBinary(statusArgs(), { ...cfg, signal: exec.signal })
        return stdout.trimEnd().slice(0, 4000)
      },
    }),
  )
}
