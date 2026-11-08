/** Native Mac connection startup uses private files, never safeStorage, and commits before activation. */
import { createHash } from 'node:crypto'
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, beforeEach, describe, expect, it, onTestFinished, vi } from 'vitest'
import { startRemoteDesktop } from '../src/remote-desktop.ts'
import { RemoteCredentialFileError, RemoteFileCredentials } from '../src/remote-file-credentials.ts'

const h = vi.hoisted(() => {
  const handlers = new Map<string, (...args: unknown[]) => unknown>()
  const events = new Map<string, () => void>()
  let root = ''
  const safeStorage = { isEncryptionAvailable: vi.fn(() => { throw new Error('Keychain must not be accessed') }),
    encryptString: vi.fn(() => { throw new Error('Keychain must not be accessed') }),
    decryptString: vi.fn(() => { throw new Error('Keychain must not be accessed') }) }
  const partition = { protocol: { handle: vi.fn<(scheme: string, handler: (request: Request) => Promise<Response>) => void>(),
    unhandle: vi.fn() },
  webRequest: { onBeforeSendHeaders: vi.fn() }, setPermissionRequestHandler: vi.fn() }
  class Window {
    static latest: Window
    readonly webContents = { id: 1, mainFrame: { url: 'dsh-app://app/' }, send: vi.fn(), on: vi.fn(),
      setWindowOpenHandler: vi.fn() }
    constructor() { Window.latest = this }
    loadURL = vi.fn(async () => {})
    once = vi.fn()
    isDestroyed = () => false
    show = vi.fn()
    focus = vi.fn()
    setTitle = vi.fn()
  }
  const electron = {
    app: { getPath: (name: string) => `${root}/${name === 'home' ? 'home' : 'userData'}`, getLocale: () => 'zh-CN',
      once: (name: string, listener: () => void) => { events.set(name, listener) }, quit: vi.fn() },
    BrowserWindow: Window, dialog: { showMessageBox: vi.fn() }, nativeTheme: { shouldUseDarkColors: true },
    ipcMain: { handle: (name: string, handler: (...args: unknown[]) => unknown) => { handlers.set(name, handler) },
      removeHandler: (name: string) => { handlers.delete(name) }, on: vi.fn() },
    Menu: { setApplicationMenu: vi.fn(), buildFromTemplate: vi.fn((value: unknown) => value) }, safeStorage,
    session: { fromPartition: vi.fn(() => partition) }, shell: { openExternal: vi.fn() },
  }
  return { electron, safeStorage, handlers, partition, Window,
    reset: (value: string) => { root = value; handlers.clear(); events.clear() },
    stop: () => { events.get('will-quit')?.(); events.clear() },
    invoke: (name: string, ...args: string[]) => {
      const handler = handlers.get('dsh-remote:' + name)
      if (!handler) throw new Error('Missing native handler: ' + name)
      return handler({ sender: Window.latest.webContents, senderFrame: Window.latest.webContents.mainFrame }, ...args)
    },
  }
})
vi.mock('electron', () => h.electron)
vi.mock('../src/remote-tls.ts', () => ({ configureRemoteTls: vi.fn() }))
vi.mock('../src/keyboard.ts', () => ({ installDesktopShortcuts: () => ({ attach: vi.fn(), dispose: vi.fn(), fileMenu: vi.fn() }) }))
vi.mock('../src/locale.ts', () => ({ resolveDesktopLocale: () => ({ messages: {} }) }))

beforeEach(() => { vi.clearAllMocks() })
afterEach(() => { h.stop(); vi.restoreAllMocks(); vi.unstubAllGlobals() })

async function fixture() {
  const root = await mkdtemp(join(tmpdir(), 'agentwork-native-files-'))
  onTestFinished(() => rm(root, { recursive: true, force: true }))
  h.reset(root)
  const frontend = join(root, 'frontend')
  await mkdir(join(frontend, 'web'), { recursive: true })
  await mkdir(join(root, 'userData'), { recursive: true })
  const index = '<html><head></head><body>AgentWork local fixture</body></html>'
  const manifest = { protocolVersion: 1, frontendHash: createHash('sha256').update(index).digest('hex'),
    bundles: {}, wikiHash: 'a'.repeat(64) }
  const origin = 'https://file-credentials.test'
  await writeFile(join(frontend, 'web/index.html'), index)
  await writeFile(join(frontend, 'manifest.json'), JSON.stringify(manifest))
  await writeFile(join(frontend, 'bootstrap.json'), JSON.stringify({ ...manifest, injections: [], defaultServerUrl: origin }))
  const cookie = 'dsh-auth-fixture=v1.fixture.signature'
  const fetcher = vi.fn<typeof fetch>(async (input, init) => {
    const url = new URL(typeof input === 'string' ? input : input instanceof URL ? input.href : input.url)
    if (url.origin !== origin) throw new Error('Unexpected fixture request')
    if (url.pathname === '/' && url.searchParams.get('token') === 'fixture-launch-token') {
      return new Response(null, { status: 303, headers: { 'set-cookie': cookie + '; Max-Age=3600; HttpOnly' } })
    }
    if (url.pathname === '/api/client/bootstrap') return new Headers(init?.headers).get('cookie') === cookie
      ? Response.json({ ...manifest, injections: [] }) : new Response(null, { status: 401 })
    throw new Error('Unexpected fixture path')
  })
  vi.stubGlobal('fetch', fetcher)
  return { root, frontend, origin, cookie, fetcher, directory: join(root, 'home/.agent-work/client') }
}

// The platform branch must run on a real Mac; the file store itself is also tested on Linux.
describe.skipIf(process.platform !== 'darwin')('Mac native file credentials', () => {
  it('connects once and restores authentication after restart without storing the launch token or using Keychain', async () => {
    const f = await fixture()
    await startRemoteDesktop(f.frontend)
    expect(await h.invoke('state')).toEqual({ serverUrl: f.origin })
    expect(await h.invoke('connect', f.origin, '')).toMatchObject({ ok: false, failure: 'authentication' })
    expect(await h.invoke('connect', f.origin, 'fixture-launch-token')).toMatchObject({ ok: true })
    const saved = await readFile(join(f.directory, 'credentials.json'), 'utf8')
    expect(saved).toContain(f.cookie)
    expect(saved).not.toContain('fixture-launch-token')
    h.stop(); h.reset(f.root)
    await startRemoteDesktop(f.frontend)
    expect(await h.invoke('state')).toEqual({ serverUrl: f.origin })
    expect(await h.invoke('connect', f.origin, '')).toMatchObject({ ok: true })
    expect(f.fetcher.mock.calls.filter(([input]) => typeof input === 'string' && input.includes('?token='))).toHaveLength(1)
    for (const method of Object.values(h.safeStorage)) expect(method).not.toHaveBeenCalled()
  })

  it('migrates the legacy address while requiring new authentication instead of decrypting old credentials', async () => {
    const f = await fixture()
    const original = JSON.stringify({ serverUrl: f.origin, encryptedCookie: 'legacy-ciphertext' })
    await writeFile(join(f.root, 'userData/remote-server.json'), original)
    await startRemoteDesktop(f.frontend)
    expect(await h.invoke('state')).toEqual({ serverUrl: f.origin })
    expect(await h.invoke('connect', f.origin, '')).toMatchObject({ ok: false, failure: 'authentication' })
    expect(await h.invoke('connect', f.origin, 'fixture-launch-token')).toMatchObject({ ok: true })
    expect(await readFile(join(f.root, 'userData/remote-server.json'), 'utf8')).toBe(original)
    for (const method of Object.values(h.safeStorage)) expect(method).not.toHaveBeenCalled()
  })

  it('does not activate forwarding when the validated credential cannot be saved', async () => {
    const f = await fixture()
    await startRemoteDesktop(f.frontend)
    vi.spyOn(RemoteFileCredentials.prototype, 'remember').mockRejectedValueOnce(new RemoteCredentialFileError('io'))
    expect(await h.invoke('connect', f.origin, 'fixture-launch-token')).toMatchObject({ ok: false, failure: 'storage' })
    const handler = h.partition.protocol.handle.mock.calls[0]?.[1]
    expect(handler).toBeTypeOf('function')
    if (typeof handler !== 'function') throw new Error('Protocol handler was not registered')
    expect((await handler(new Request('dsh-app://app/api/test'))).status).toBe(503)
  })

  it('opens the local interface and reports damaged storage without fetching any backend', async () => {
    const f = await fixture()
    await mkdir(f.directory, { recursive: true, mode: 0o700 })
    await writeFile(join(f.directory, 'connection.json'), 'invalid-private-fixture', { mode: 0o600 })
    await startRemoteDesktop(f.frontend)
    expect(h.Window.latest.loadURL).toHaveBeenCalledWith('dsh-app://app/')
    expect(await h.invoke('state')).toEqual({ serverUrl: '', failure: 'storage' })
    expect(await h.invoke('boot')).toMatchObject({ injections: [], ownsHost: false })
    expect(f.fetcher).not.toHaveBeenCalled()
  })
})
