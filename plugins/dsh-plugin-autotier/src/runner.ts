import { execFile } from 'node:child_process'

export interface RunnerOptions {
  /** autotier binary. Default: $AUTOTIER_BIN or "autotier" on PATH. */
  binary?: string
  /** Timeout per call in ms. Default 120000. */
  timeoutMs?: number
  /** Working dir for state files (log/learn/agents). Default process.cwd(). */
  cwd?: string
}

export interface RunResult {
  stdout: string
}

/** Build argv for `autotier route` (dry-run: decides without spending). */
export function routeArgs(prompt: string, opts: { mode?: string; deny?: string } = {}): string[] {
  const argv = ['route', '--mode', opts.mode || 'cost']
  if (opts.deny) argv.push('--deny', opts.deny)
  argv.push(prompt)
  return argv
}

/** Build argv for `autotier agent call` (flags BEFORE positionals). */
export function callArgs(agent: string, task: string, opts: { to?: string } = {}): string[] {
  const argv = ['agent', 'call']
  if (opts.to) argv.push('--to', opts.to)
  argv.push(agent, task)
  return argv
}

export function statusArgs(): string[] {
  return ['status']
}

function binaryOf(opts: RunnerOptions): string {
  return opts.binary || process.env.AUTOTIER_BIN || 'autotier'
}

/** Run the autotier binary. Kills the child when signal aborts (dsh exec.signal). */
export function runBinary(
  argv: string[],
  opts: RunnerOptions & { signal?: AbortSignal } = {},
): Promise<RunResult> {
  return new Promise((resolve, reject) => {
    const child = execFile(
      binaryOf(opts),
      argv,
      { cwd: opts.cwd, timeout: opts.timeoutMs ?? 120000, maxBuffer: 4 * 1024 * 1024, windowsHide: true },
      (error, stdout, stderr) => {
        if (error) {
          const msg = (stdout || '') + (stderr || '') || String(error)
          reject(new Error(`autotier ${argv.slice(0, 2).join(' ')} failed: ${msg.trim().slice(0, 2000)}`))
          return
        }
        resolve({ stdout: String(stdout) })
      },
    )
    opts.signal?.addEventListener('abort', () => child.kill(), { once: true })
  })
}
