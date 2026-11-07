/** The portable kernel's offline inspection and full content validation commands. */
import type { SubprocessRuntime } from '@deepseek-ai/dsh-subprocess'
import { z } from 'zod'
import { WikiError, type KernelConfig } from './kernel.ts'
import { portableStoreId } from './source-files.ts'

const resultSchema = z.discriminatedUnion('ok', [
  z.object({ ok: z.literal(true), protocolVersion: z.literal(1), result: z.unknown() }),
  z.object({ ok: z.literal(false), protocolVersion: z.literal(1), error: z.object({ type: z.string() }) }),
])

/** Runs offline operations through the CLI entrypoint of the owned native executable. */
export class NativeValidation {
  constructor(
    private readonly subprocess: SubprocessRuntime,
    private readonly config: KernelConfig & { readonly sourceOperationTimeoutMs: number },
  ) {}

  /**
   * Verify that the source is closed and has no unfinished native transaction.
   * @param root - portable source directory.
   * @param signal - operation cancellation.
   * @returns the verified store identity.
   */
  async offline(root: string, signal: AbortSignal): Promise<string> {
    const storeId = await portableStoreId(root)
    const status = z.object({
      storeId: z.literal(storeId),
      sessionLock: z.string(),
      sourceLock: z.string(),
      recovery: z.string(),
    }).parse(await this.invoke(root, 'status', storeId, {}, signal))
    const fresh = status.sessionLock === 'uninitialized' && status.sourceLock === 'uninitialized'
    if (!fresh && (status.sessionLock !== 'available' || status.sourceLock !== 'readable')) throw new WikiError('source_in_use')
    if (status.recovery !== 'clear') throw new WikiError('source_recovery_required')
    return storeId
  }

  /**
   * Validate native metadata, document contents and cross references in a staged store.
   * @param root - staged portable root.
   * @param storeId - expected portable store identity.
   * @param signal - operation cancellation.
   * @returns the number of native warnings.
   */
  async validate(root: string, storeId: string, signal: AbortSignal): Promise<number> {
    if (await this.offline(root, signal) !== storeId) throw new WikiError('store_identity_mismatch')
    const result = z.object({
      storeId: z.literal(storeId),
      diagnostics: z.array(z.object({ severity: z.enum(['warning', 'error']) })).optional(),
    }).parse(await this.invoke(root, 'page', storeId, { action: 'check', scope: 'content' }, signal))
    const diagnostics = result.diagnostics ?? []
    if (diagnostics.some(item => item.severity === 'error')) throw new WikiError('source_validation_failed')
    return diagnostics.length
  }

  private async invoke(root: string, command: 'status' | 'page', storeId: string, input: object, signal: AbortSignal): Promise<unknown> {
    signal.throwIfAborted()
    const child = this.subprocess.spawn({
      argv: [this.config.kernelPath, '--wiki-cli-entrypoint', command, '--root', root, '--request', '-', '--json'],
      cwd: root,
      stdio: {
        stdin: { data: JSON.stringify({ protocolVersion: 1, storeId, ...input }) },
        stdout: { maxBytes: this.config.maxResponseBytes },
        stderr: { maxBytes: 4096 },
      },
      signal: AbortSignal.any([signal, AbortSignal.timeout(this.config.sourceOperationTimeoutMs)]),
      graceMs: this.config.shutdownGraceMs,
    })
    try {
      const outcome = await child.done
      signal.throwIfAborted()
      const stdout = child.collected.stdout?.readFrom(0)
      if (!stdout || stdout.lossy || !stdout.text) throw new WikiError('native_validation_unavailable')
      const result = resultSchema.parse(JSON.parse(stdout.text))
      if (!result.ok || outcome.exitCode !== 0) throw new WikiError('source_validation_failed')
      return result.result
    } finally {
      child.terminate()
      await child.waitForExit()
    }
  }
}
