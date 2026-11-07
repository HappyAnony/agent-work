/** Owns an IronMan SiYuan process and the browser's one-use authentication handoff. */
import { randomBytes, randomUUID } from 'node:crypto'
import { createServer, type Server } from 'node:http'
import { cp, mkdir, mkdtemp, readdir, rm, stat, symlink, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import type { SubprocessHandle, SubprocessRuntime } from '@deepseek-ai/dsh-subprocess'
import { z } from 'zod'
import { WikiError, type KernelConfig } from './kernel.ts'
import { readRegular } from './source-files.ts'
import { siyuanFrame } from './siyuan-frame.ts'
import type { WikiSiyuanView } from './types.ts'

const listener = /http server \[(127\.0\.0\.1:[1-9]\d{0,4})\] is booting/u
const nativeCookie = /^siyuan=[A-Za-z0-9_+=/%.-]+;/u
const resultSchema = z.object({ code: z.number(), data: z.unknown() })
const confSchema = z.object({
  accessAuthCode: z.string().optional(),
  system: z.object({ networkServe: z.boolean().optional() }).optional(),
})
const nativeLanguages = new Map([
  ['ar', 'ar_SA'], ['de', 'de_DE'], ['en', 'en_US'], ['es', 'es_ES'], ['fr', 'fr_FR'],
  ['he', 'he_IL'], ['it', 'it_IT'], ['ja', 'ja_JP'], ['pl', 'pl_PL'], ['pt', 'pt_BR'],
  ['ru', 'ru_RU'], ['zh', 'zh_CN'],
])

async function linkAssets(source: string, target: string, exclude: ReadonlySet<string>): Promise<void> {
  for (const name of await readdir(source)) {
    if (exclude.has(name)) continue
    const original = join(source, name)
    await symlink(original, join(target, name), (await stat(original)).isDirectory() ? 'junction' : 'file')
  }
}

/** One authenticated loopback SiYuan runtime for the selected WikiMan page. */
export class SiyuanKernel {
  private readonly lifetime = new AbortController()
  private child: SubprocessHandle | undefined
  private assets: string | undefined
  private origin: string | undefined
  private cookie: string | undefined
  private relay: Server | undefined
  private handoffDeadline: ReturnType<typeof setTimeout> | undefined
  private disposal: Promise<void> | undefined

  constructor(
    private readonly subprocess: SubprocessRuntime, private readonly config: KernelConfig, private readonly workspace: string,
  ) {}

  /**
   * Launch the matching original application and get a native owner cookie.
   * @param single - whether this WikiMan page uses SiYuan single-document mode.
   * @param title - initial title for a newly created single-document workspace.
   * @param locale - active DSH language, resolved to a shipped native language or English.
   * @param signal - cancellation before the runtime is handed to a browser.
   * @returns completion of the native boot and authentication.
   */
  async start(single: boolean, title: string, locale: string, signal: AbortSignal): Promise<void> {
    const requested = new Intl.Locale(locale)
    const traditional = requested.language === 'zh'
      && (requested.script === 'Hant' || ['TW', 'HK', 'MO'].includes(requested.region ?? ''))
    const language = traditional ? 'zh_CHT' : nativeLanguages.get(requested.language) ?? 'en_US'
    const request = AbortSignal.any([signal, this.lifetime.signal, AbortSignal.timeout(this.config.startupTimeoutMs)])
    const conf = await readRegular(join(this.workspace, 'conf', 'conf.json'), this.config.maxResponseBytes)
    const settings = conf ? confSchema.parse(JSON.parse(conf.toString('utf8'))) : {}
    if (settings.system?.networkServe) throw new WikiError('siyuan_network_enabled')
    const authCode = settings.accessAuthCode?.trim() || randomBytes(24).toString('hex')
    const assets = await mkdtemp(join(tmpdir(), 'dsh-siyuan-assets-'))
    this.assets = assets
    const stage = join(assets, 'stage')
    await mkdir(stage)
    await Promise.all([
      cp(join(this.config.appDir, 'appearance'), join(assets, 'appearance'), { recursive: true }),
      linkAssets(this.config.appDir, assets, new Set(['stage', 'appearance'])),
      linkAssets(join(this.config.appDir, 'stage'), stage, new Set(['dsh-workbench.html'])),
    ])
    await writeFile(join(stage, 'dsh-workbench.html'), siyuanFrame, { flag: 'wx' })
    request.throwIfAborted()
    const child = this.subprocess.spawn({
      argv: [
        this.config.kernelPath, '--workspace=' + this.workspace, '--wd=' + assets, '--port=0', '--fixedPort=0',
        '--lang=' + language,
        ...(single ? ['--single', '--single-doc-name=' + title] : []),
      ],
      env: { SIYUAN_ACCESS_AUTH_CODE: authCode, SIYUAN_ACCESS_AUTH_CODE_BYPASS: 'false' },
      cwd: this.workspace,
      stdio: { stdin: 'ignore', stdout: 'pipe', stderr: { maxBytes: 4096 } },
      graceMs: this.config.shutdownGraceMs, signal: this.lifetime.signal,
    })
    this.child = child
    void child.done.then(
      () => { this.lifetime.abort(new WikiError('kernel_exited')) },
      () => { this.lifetime.abort(new WikiError('kernel_start_failed')) },
    )
    const stdout = child.stdout
    if (!stdout) throw new WikiError('kernel_stdout_missing')
    this.origin = await new Promise<string>((resolve, reject) => {
      let tail = ''
      const finish = () => {
        stdout.off('data', consume)
        stdout.resume()
        request.removeEventListener('abort', abort)
      }
      const abort = () => { finish(); reject(new WikiError('kernel_not_ready', { cause: request.reason })) }
      const consume = (chunk: Buffer) => {
        tail = (tail + chunk.toString('utf8')).slice(-8192)
        const address = listener.exec(tail)?.[1]
        if (address) { finish(); resolve('http://' + address) }
        else if (tail.includes('http server [0.0.0.0:')) {
          finish()
          reject(new WikiError('siyuan_network_enabled'))
        }
      }
      stdout.on('data', consume)
      request.addEventListener('abort', abort, { once: true })
      if (request.aborted) abort()
    })
    const login = await fetch(this.origin + '/api/system/loginAuth', {
      method: 'POST', redirect: 'error', signal: request,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ authCode, captcha: '', rememberMe: false }),
    })
    const cookie = login.headers.getSetCookie().find(value => nativeCookie.test(value))
    const result = resultSchema.parse(await login.json())
    if (!login.ok || !cookie || result.code !== 0) throw new WikiError('siyuan_auth_failed')
    this.cookie = cookie
  }

  /**
   * Issue a one-use browser handoff that sets the native HttpOnly cookie on loopback.
   * @param parent - origin of the DSH browser.
   * @param single - whether to load the native single-document application.
   * @param docId - optional original document to open.
   * @param changed - whether the cited version differs from the current one.
   * @param release - closes the native generation when its browser connection ends.
   * @param signal - cancellation during listener setup.
   * @returns the owned editor view and its native origin.
   */
  async view(
    parent: string, single: boolean, docId: string | undefined, changed: boolean,
    release: (viewId: string) => Promise<void>, signal: AbortSignal,
  ): Promise<WikiSiyuanView> {
    if (!/^http:\/\/127\.0\.0\.1:[1-9]\d{0,4}$/u.test(parent)) throw new WikiError('siyuan_loopback_required')
    const origin = this.url()
    const cookie = this.cookie
    if (!cookie || this.relay) throw new WikiError('siyuan_auth_failed')
    const viewId = randomUUID()
    const ticket = '/' + randomBytes(32).toString('hex')
    const lease = '/' + randomBytes(32).toString('hex')
    const releaseView = () => {
      void release(viewId).catch((_error: unknown) => { /* The Host still owns the runtime on plugin teardown. */ })
    }
    const fragment = new URLSearchParams({ parent, view: viewId, mode: single ? 'single' : 'desktop' })
    if (docId) fragment.set('id', docId)
    let used = false
    let attached = false
    const relay = createServer((req, res) => {
      const address = relay.address()
      if (req.method !== 'GET' || !address || typeof address === 'string' || req.headers.host !== `127.0.0.1:${address.port}`) {
        res.writeHead(404, { 'cache-control': 'no-store' }).end()
        return
      }
      if (!attached && req.url === lease && req.headers.origin === parent) {
        attached = true
        clearTimeout(this.handoffDeadline)
        res.writeHead(200, {
          'access-control-allow-origin': parent, 'cache-control': 'no-store', 'content-type': 'text/event-stream',
        })
        res.write(': connected\n\n')
        res.once('close', releaseView)
        return
      }
      if (used || req.url !== ticket) {
        res.writeHead(404, { 'cache-control': 'no-store' }).end()
        return
      }
      used = true
      res.writeHead(303, {
        location: origin + '/stage/dsh-workbench.html#' + fragment.toString(),
        'set-cookie': cookie + '; SameSite=Strict',
        'cache-control': 'no-store',
        'referrer-policy': 'no-referrer',
      }).end()
    })
    this.relay = relay
    await new Promise<void>((resolve, reject) => {
      const cleanup = () => {
        relay.off('error', failed)
        signal.removeEventListener('abort', abort)
      }
      const failed = (error: Error) => { cleanup(); reject(error) }
      const abort = () => { failed(new WikiError('siyuan_auth_failed', { cause: signal.reason })) }
      if (signal.aborted) {
        abort()
        return
      }
      relay.once('error', failed)
      signal.addEventListener('abort', abort, { once: true })
      relay.listen({ port: 0, host: '127.0.0.1', signal }, () => { cleanup(); resolve() })
    })
    const address = relay.address()
    if (!address || typeof address === 'string') throw new WikiError('siyuan_auth_failed')
    this.handoffDeadline = setTimeout(releaseView, this.config.startupTimeoutMs)
    const relayOrigin = `http://127.0.0.1:${address.port}`
    return { origin, url: relayOrigin + ticket, lifetimeUrl: relayOrigin + lease, viewId, changed }
  }

  private url(): string {
    this.lifetime.signal.throwIfAborted()
    if (!this.origin) throw new WikiError('kernel_not_ready')
    return this.origin
  }

  /**
   * Request native shutdown and release the managed child and linked assets.
   * @returns completion of owned process and listener cleanup.
   */
  dispose(): Promise<void> {
    return this.disposal ??= this.shutdown()
  }

  private async shutdown(): Promise<void> {
    clearTimeout(this.handoffDeadline)
    const relay = this.relay
    if (relay?.listening) {
      const closed = new Promise<void>((resolve) => { relay.close(() => { resolve() }) })
      relay.closeAllConnections()
      await closed
    }
    try {
      if (this.origin && this.cookie && !this.lifetime.signal.aborted) {
        const response = await fetch(this.origin + '/api/system/exit', {
          method: 'POST', redirect: 'error', signal: AbortSignal.timeout(this.config.shutdownGraceMs),
          headers: { 'content-type': 'application/json', cookie: this.cookie.split(';', 1)[0] ?? '' },
          body: JSON.stringify({ force: false, execInstallPkg: 1 }),
        })
        await response.body?.cancel()
      }
    } catch (_error) {
      // A native exit can close the HTTP connection before its reply arrives.
    } finally {
      this.lifetime.abort()
      this.child?.terminate()
      await this.child?.waitForExit()
      if (this.assets) await rm(this.assets, { recursive: true, force: true })
    }
  }
}
