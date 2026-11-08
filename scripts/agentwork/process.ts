/** Shell-free subprocesses with one cancellation owner and bounded descendant teardown. */
import { execa, ExecaError } from 'execa'
import { pnpmInvocation } from '../executable-packaging.ts'
import { ReleaseError } from './model.ts'

/** Build process settings inherited by every platform adapter. */
export interface Execution {
  /** Absolute working directory. */
  cwd: string
  /** Cancellation propagated from the CLI. */
  signal?: AbortSignal
  /** Maximum duration of each subprocess in milliseconds. */
  timeoutMs: number
  /** Explicit per-operation toolchain and signing configuration. */
  env?: NodeJS.ProcessEnv
}

/**
 * Execute a build tool and await descendant termination before returning failure.
 * @param command - Executable, never a shell program fragment.
 * @param args - Literal argument vector; credentials must not appear in it.
 * @param execution - Working directory, cancellation and duration limit.
 * @param capture - Capture stdout for parser-owned metadata; normal build logs use stderr.
 * @param input - Optional protocol request on stdin; never an argument or diagnostic.
 * @returns Captured stdout, or an empty string for streamed build output.
 */
export async function run(command: string, args: readonly string[], execution: Execution,
  capture = false, input?: string): Promise<string> {
  try {
    const result = await execa(command, [...args], {
      cwd: execution.cwd, ...execution.env ? { env: execution.env } : {}, ...execution.signal ? { cancelSignal: execution.signal } : {},
      timeout: execution.timeoutMs, killDescendants: true, forceKillAfterDelay: 10_000,
      ...input === undefined ? { stdin: 'ignore' as const } : { input }, stdout: capture ? 'pipe' : process.stderr, stderr: process.stderr,
    })
    return typeof result.stdout === 'string' ? result.stdout.trim() : ''
  } catch (error) {
    if (error instanceof ExecaError) {
      const code = error.isCanceled ? 'cancelled' : error.timedOut ? 'timed-out' : 'tool-failed'
      throw new ReleaseError(code, command + ' failed; inspect stderr and retry only after resolving the reported tool failure')
    }
    throw error
  }
}

/**
 * Run the workspace's pinned package manager without a platform shell shim.
 * @param args - Literal package-manager arguments.
 * @param execution - Shared process ownership and toolchain settings.
 * @param capture - Whether to return stdout instead of streaming it to stderr.
 * @returns Captured stdout when requested.
 */
export async function pnpm(args: string[], execution: Execution, capture = false): Promise<string> {
  const [command, argv] = pnpmInvocation(args)
  return run(command, argv, execution, capture)
}
