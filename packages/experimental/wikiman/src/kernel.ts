/** Owns the portable WikiMan process, credentials and bounded local RPCs. */
import { randomBytes } from 'node:crypto'
import { access, mkdtemp, readFile, realpath, rm, writeFile } from 'node:fs/promises'
import { constants } from 'node:fs'
import { tmpdir } from 'node:os'
import { isAbsolute, join } from 'node:path'
import type { SubprocessHandle, SubprocessRuntime } from '@deepseek-ai/dsh-subprocess'
import { z } from 'zod'

const protocolVersion = 1
const readySchema = z.object({
  ok: z.literal(true),
  type: z.literal('ready'),
  protocolVersion: z.literal(protocolVersion),
  address: z.string().regex(/^127\.0\.0\.1:[1-9][0-9]{0,4}$/),
  storeId: z.string(),
  sessionId: z.string().min(1),
  editorAssets: z.literal(true),
})
// Native metadata may contain opaque integers outside JavaScript's safe range.
interface JsonSourceContext { readonly source?: string }
const nativeJSON = JSON as typeof JSON & { rawJSON(source: string): object }

/**
 * Parse native JSON without rounding numeric values in opaque metadata.
 * @param source - native JSON payload.
 * @returns the value with round-trip numeric tokens in opaque fields.
 */
export function parseWikiJSON(source: string): unknown {
  return JSON.parse(source, (_key: string, value: unknown, context?: JsonSourceContext) => {
    if (typeof value !== 'number' || context?.source === undefined || Number.isSafeInteger(value) && String(value) === context.source) return value
    return nativeJSON.rawJSON(context.source)
  })
}

const envelope = z.discriminatedUnion('ok', [
  z.object({ ok: z.literal(true), result: z.unknown() }),
  z.object({ ok: z.literal(false), error: z.object({ type: z.string().regex(/^[a-z_]{1,80}$/) }) }),
])

/** Explicit paths and limits for the owned kernel. */
export interface KernelConfig {
  /** Absolute path to the executable matching the selected WikiMan runtime. */
  readonly kernelPath: string
  /** Absolute path to the matching wiki-app or original siyuan-app resource directory. */
  readonly appDir: string
  /** Absolute path to the portable store or original WikiMan library. */
  readonly sourceRoot: string
  /** Maximum time in milliseconds to wait for kernel readiness. */
  readonly startupTimeoutMs: number
  /** Grace period in milliseconds for the kernel to release the source during shutdown. */
  readonly shutdownGraceMs: number
  /** Maximum time in milliseconds for an authenticated kernel request. */
  readonly requestTimeoutMs: number
  /** Maximum bytes accepted from one kernel response. */
  readonly maxResponseBytes: number
}

/** A stable local kernel error suitable for transport without exposing private paths or credentials. */
export class WikiError extends Error {
  constructor(readonly code: string, options?: ErrorOptions) {
    super(`WikiMan: ${code}`, options)
  }
}

/** Read an authenticated, protocol-checked result from an owned portable WikiMan kernel. */
export class WikiKernel {
  private readonly lifetime = new AbortController()
  private child?: SubprocessHandle
  private privateDir?: string
  private ready?: z.infer<typeof readySchema>
  private token?: string
  private startup?: Promise<void>

  constructor(private readonly subprocess: SubprocessRuntime, private readonly config: KernelConfig) {}

  /** Cancellation of the owned kernel generation, including source replacement and teardown. */
  get signal(): AbortSignal { return this.lifetime.signal }

  /**
   * Read the live kernel identity.
   * @returns verified loopback editor origin and identity of the running source.
   */
  identity(): { baseUrl: string; storeId: string; sessionId: string } {
    if (!this.ready || this.lifetime.signal.aborted) throw new WikiError('kernel_unavailable')
    const { address, storeId, sessionId } = this.ready
    return { baseUrl: `http://${address}`, storeId, sessionId }
  }

  /**
   * Start the owned process once and share its readiness with callers.
   * @returns readiness of the kernel and portable editor assets.
   */
  start(): Promise<void> {
    return this.startup ??= this.launch()
  }

  private async launch(): Promise<void> {
    const { kernelPath, appDir, sourceRoot, startupTimeoutMs } = this.config
    if (![kernelPath, appDir, sourceRoot].every(isAbsolute)) throw new WikiError('absolute_paths_required')
    const [bin, assets, root] = await Promise.all([realpath(kernelPath), realpath(appDir), realpath(sourceRoot)])
    const { storeId } = z.object({ storeId: z.uuid() }).parse(JSON.parse(await readFile(join(root, 'store.json'), 'utf8')))
    await access(bin, constants.X_OK)
    this.lifetime.signal.throwIfAborted()
    const dir = await mkdtemp(join(tmpdir(), 'dsh-wikiman-'))
    this.privateDir = dir
    this.token = randomBytes(32).toString('hex')
    const tokenFile = join(dir, 'token')
    await writeFile(tokenFile, this.token, { flag: 'wx', mode: 0o600 })
    this.lifetime.signal.throwIfAborted()
    const child = this.subprocess.spawn({
      argv: [bin, 'serve', '--root', root, '--store', storeId, '--app-dir', assets, '--token-file', tokenFile, '--listen', '127.0.0.1:0'],
      cwd: dir,
      stdio: { stdin: 'ignore', stdout: 'pipe', stderr: { maxBytes: 4096 } },
      graceMs: this.config.shutdownGraceMs,
      signal: this.lifetime.signal,
    })
    this.child = child
    void child.done.then(
      () => { this.lifetime.abort(new WikiError('kernel_exited')) },
      () => { this.lifetime.abort(new WikiError('kernel_start_failed')) },
    )
    const stdout = child.stdout
    if (!stdout) throw new WikiError('kernel_stdout_missing')
    const signal = AbortSignal.any([this.lifetime.signal, AbortSignal.timeout(startupTimeoutMs)])
    const line = await new Promise<string>((resolve, reject) => {
      let buffer = ''
      const cleanup = () => {
        stdout.off('data', data)
        signal.removeEventListener('abort', abort)
      }
      const abort = () => { cleanup(); reject(new WikiError('kernel_not_ready', { cause: signal.reason })) }
      const data = (chunk: Buffer) => {
        buffer += chunk.toString('utf8')
        const end = buffer.indexOf('\n')
        if (buffer.length > 4096) {
          cleanup()
          reject(new WikiError('invalid_kernel_readiness'))
        } else if (end !== -1) {
          cleanup()
          resolve(buffer.slice(0, end))
        }
      }
      signal.addEventListener('abort', abort, { once: true })
      stdout.on('data', data)
      if (signal.aborted) abort()
    })
    stdout.resume()
    const ready = readySchema.safeParse(JSON.parse(line))
    if (!ready.success || ready.data.storeId !== storeId) throw new WikiError('invalid_kernel_readiness')
    this.ready = ready.data
  }

  /**
   * Call a registered local WikiMan route using the kernel generation's private credential.
   * @param route - portable kernel route owned by the plugin.
   * @param input - validated operation fields.
   * @param schema - decoder for the successful result.
   * @param signal - caller cancellation.
   * @returns the validated kernel result.
   */
  async call<T>(route: 'pages' | 'content' | 'editor', input: Record<string, unknown>, schema: z.ZodType<T>, signal: AbortSignal): Promise<T> {
    const { baseUrl, storeId, sessionId } = this.identity()
    const requestSignal = AbortSignal.any([signal, this.lifetime.signal, AbortSignal.timeout(this.config.requestTimeoutMs)])
    try {
      const response = await fetch(`${baseUrl}/v1/${route}`, {
        method: 'POST',
        redirect: 'error',
        headers: { authorization: `Bearer ${this.token}`, 'content-type': 'application/json', 'x-wiki-session': sessionId },
        body: JSON.stringify({ ...input, protocolVersion, storeId }),
        signal: requestSignal,
      })
      if (!response.body) throw new WikiError('invalid_kernel_response')
      const reader = response.body.getReader()
      const chunks: Uint8Array[] = []
      let bytes = 0
      try {
        for (;;) {
          const chunk = await reader.read()
          if (chunk.done) break
          bytes += chunk.value.byteLength
          if (bytes > this.config.maxResponseBytes) throw new WikiError('kernel_response_too_large')
          chunks.push(chunk.value)
        }
      } finally {
        await reader.cancel()
      }
      const decoded = envelope.parse(parseWikiJSON(Buffer.concat(chunks).toString('utf8')))
      if (!decoded.ok) throw new WikiError(decoded.error.type)
      if (!response.ok) throw new WikiError('invalid_kernel_response')
      return schema.parse(decoded.result)
    } catch (error) {
      signal.throwIfAborted()
      if (error instanceof WikiError) throw error
      throw new WikiError(requestSignal.aborted ? 'kernel_unavailable' : 'kernel_request_failed', { cause: error })
    }
  }

  /**
   * Stop the managed process before removing the private credential.
   * @returns completion of process teardown and private credential cleanup.
   */
  async dispose(): Promise<void> {
    this.lifetime.abort()
    await this.startup?.catch((_error: unknown) => { /* Startup failure is reported by plugin activation. */ })
    try {
      this.child?.terminate()
      await this.child?.waitForExit()
    } finally {
      if (this.privateDir) await rm(this.privateDir, { recursive: true, force: true })
    }
  }
}
