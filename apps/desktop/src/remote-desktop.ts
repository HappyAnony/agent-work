/** Remote-only desktop carrier with device-owned credentials and no local Host process. */
import { mkdir, readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { app, BrowserWindow, dialog, ipcMain, Menu, nativeTheme, session, shell } from 'electron'
import { z } from 'zod'
import { writeFileAtomic } from '@deepseek-ai/dsh-atomic-write'
import { RemoteClientError, RemoteFrontend, decryptRemoteCookie, readRemoteBootstrap, remoteBootstrapSchema, remoteServerOrigin } from './remote-frontend.ts'
import { remoteCopy } from './remote-copy.ts'
import { forwardWebRequest } from './web-document.ts'
import { installDesktopShortcuts } from './keyboard.ts'
import { resolveDesktopLocale } from './locale.ts'
import { DESKTOP_IPC } from './ipc.ts'
import { configureRemoteTls } from './remote-tls.ts'
import { RemoteCredentialFileError, RemoteFileCredentials, remoteFileCredential,
  type RemoteFileCredential } from './remote-file-credentials.ts'

const settingsSchema = z.object({ serverUrl: z.string(), selectedUrl: z.string().optional(), encryptedCookie: z.string().optional() })

/**
 * Open the packaged interface before authenticating the selected server. Closing it never stops server work.
 * @param frontendRoot - Local resource inventory and browser assets.
 * @returns A callback that focuses the surviving application window.
 */
export async function startRemoteDesktop(frontendRoot: string): Promise<() => void> {
  configureRemoteTls()
  const frontend = await RemoteFrontend.load(frontendRoot)
  const localBootstrap = await frontend.localBootstrap()
  const copy = remoteCopy(app.getLocale())
  const directory = app.getPath('userData')
  const settingsPath = join(directory, 'remote-server.json')
  const fileCredentials = process.platform === 'darwin'
    ? new RemoteFileCredentials(join(app.getPath('home'), '.agent-work/client'), settingsPath) : undefined
  const partition = session.fromPartition('persist:agentwork-remote')
  let saved: z.infer<typeof settingsSchema> = { serverUrl: '' }
  if (!fileCredentials) {
    try { saved = settingsSchema.parse(JSON.parse(await readFile(settingsPath, 'utf8'))) }
    catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error }
  }
  let active: { serverUrl: string; cookie: string; bootstrap: z.infer<typeof remoteBootstrapSchema> } | undefined
  let connecting: Promise<void> | undefined
  let requests = new AbortController()
  const window = new BrowserWindow({ width: 1280, height: 820, minWidth: 390, minHeight: 560, title: 'AgentWork',
    backgroundColor: nativeTheme.shouldUseDarkColors ? '#202020' : '#fafafa',
    webPreferences: { preload: fileURLToPath(new URL('./preload-remote.cjs', import.meta.url)),
      session: partition, nodeIntegration: false, contextIsolation: true, sandbox: true, webSecurity: true },
  })
  const focus = () => { if (!window.isDestroyed()) { window.show(); window.focus() } }
  const assertSender = (event: Electron.IpcMainInvokeEvent) => {
    if (event.sender !== window.webContents || event.senderFrame !== window.webContents.mainFrame ||
      !event.senderFrame.url.startsWith('dsh-app://app/')) throw new Error('remote-client-sender')
  }
  const setup = () => {
    window.webContents.send('dsh-remote:configure')
    focus()
  }
  const connect = async (address: string, token?: string) => {
    const serverUrl = remoteServerOrigin(address)
    requests.abort()
    requests = new AbortController()
    active = undefined
    if (fileCredentials) await fileCredentials.select(serverUrl)
    else {
      const selection = { ...saved, selectedUrl: serverUrl }
      await mkdir(directory, { recursive: true })
      await writeFileAtomic(settingsPath, JSON.stringify(selection) + '\n', { mode: 0o600 })
      saved = selection
    }
    const signal = AbortSignal.any([requests.signal, AbortSignal.timeout(15000)])
    let cookie: string
    let credential: RemoteFileCredential | undefined
    if (token) {
      if (token.length > 4096 || /[\r\n]/u.test(token)) throw new RemoteClientError('remote-auth-token')
      const response = await fetch(serverUrl + '/?token=' + encodeURIComponent(token), { redirect: 'manual', signal })
      const header = response.headers.get('set-cookie')
      await response.body?.cancel()
      if (response.status !== 303 || !header) throw new RemoteClientError('remote-authentication')
      cookie = header.split(';')[0] ?? ''
      if (fileCredentials) credential = remoteFileCredential(serverUrl, header, Date.now())
    } else if (fileCredentials) {
      credential = await fileCredentials.credential(serverUrl, Date.now())
      cookie = credential?.cookie ?? ''
    } else {
      const { safeStorage } = await import('electron')
      cookie = saved.serverUrl === serverUrl && saved.encryptedCookie && safeStorage.isEncryptionAvailable()
        ? decryptRemoteCookie(saved.encryptedCookie, bytes => safeStorage.decryptString(bytes)) : ''
    }
    const response = await fetch(serverUrl + '/api/client/bootstrap', { headers: { cookie }, redirect: 'error', signal })
    if (!response.ok) { await response.body?.cancel(); throw new RemoteClientError('remote-bootstrap', String(response.status)) }
    const bootstrap = await readRemoteBootstrap(response)
    await frontend.verify(bootstrap)
    signal.throwIfAborted()
    if (fileCredentials) {
      if (credential) await fileCredentials.remember(credential)
    } else {
      const { safeStorage } = await import('electron')
      if (!safeStorage.isEncryptionAvailable()) throw new RemoteClientError('remote-credential-storage-unavailable')
      const next = { serverUrl, selectedUrl: serverUrl, encryptedCookie: safeStorage.encryptString(cookie).toString('base64') }
      await mkdir(directory, { recursive: true })
      await writeFileAtomic(settingsPath, JSON.stringify(next) + '\n', { mode: 0o600 })
      saved = next
    }
    active = { serverUrl, cookie, bootstrap }
    focus()
  }
  partition.protocol.handle('dsh-app', async (request) => {
    const url = new URL(request.url)
    if (url.hostname !== 'app') return new Response(null, { status: 404 })
    const local = await frontend.resource(request)
    if (local) return local
    if (!active) return new Response(null, { status: 503 })
    const forwarded = new Request(request, { signal: AbortSignal.any([request.signal, requests.signal]) })
    return forwardWebRequest(forwarded, active.serverUrl, active.cookie)
  })
  partition.webRequest.onBeforeSendHeaders({ urls: ['ws://*/*', 'wss://*/*'] }, (details, callback) => {
    if (!active || details.webContentsId !== window.webContents.id) { callback({}); return }
    const target = new URL(active.serverUrl)
    const requested = new URL(details.url)
    if (requested.host !== target.host || requested.protocol !== (target.protocol === 'https:' ? 'wss:' : 'ws:')) { callback({}); return }
    const headers = Object.fromEntries(Object.entries(details.requestHeaders).map(([key, value]) => [key.toLowerCase(), value]))
    if (headers.origin !== 'dsh-app://app') { callback({ cancel: true }); return }
    callback({ requestHeaders: { ...headers, origin: target.origin, cookie: active.cookie, 'sec-fetch-site': 'same-origin' } })
  })
  partition.setPermissionRequestHandler((_contents, _permission, callback) => { callback(false) })
  ipcMain.handle('dsh-remote:state', async (event) => {
    assertSender(event)
    try {
      return { serverUrl: fileCredentials
        ? await fileCredentials.readAddress() || localBootstrap.defaultServerUrl
        : saved.selectedUrl ?? (saved.serverUrl || localBootstrap.defaultServerUrl) }
    } catch (error) { return { serverUrl: '', failure: connectionFailure(error) } }
  })
  ipcMain.handle('dsh-remote:connect', async (event, address: unknown, token: unknown) => {
    assertSender(event)
    if (typeof address !== 'string' || typeof token !== 'string' || connecting) return { ok: false }
    connecting = connect(address, token)
    try { await connecting; return { ok: true, injections: active?.bootstrap.injections } }
    catch (error) { console.warn('Remote connection failed:', error instanceof Error ? error.message.replace(/\?.*$/u, '') : 'remote-connection'); return { ok: false, failure: connectionFailure(error) } }
    finally { connecting = undefined }
  })
  ipcMain.handle('dsh-remote:boot', (event) => {
    assertSender(event)
    return { injections: localBootstrap.injections, streamBaseUrl: '', ownsHost: false }
  })
  ipcMain.handle('dsh-remote:failed', async (event, message: unknown) => {
    assertSender(event)
    const diagnostic = typeof message === 'string' ? message.slice(0, 8192).replace(/([?&]token=)[^\s&]+/gu, '$1[redacted]') : 'remote-frontend-failed'
    await writeFileAtomic(join(directory, 'remote-client-error.json'), JSON.stringify({ error: diagnostic }) + '\n', { mode: 0o600 })
    await dialog.showMessageBox(window, { type: 'error', title: copy.title, message: copy.failed, detail: diagnostic })
  })
  ipcMain.on(DESKTOP_IPC.nativeThemeSet, (event, value: unknown) => {
    if (event.sender === window.webContents && ['system', 'light', 'dark'].includes(String(value))) nativeTheme.themeSource = value as 'system' | 'light' | 'dark'
  })
  window.webContents.on('will-navigate', (event, address) => {
    const url = new URL(address)
    if (url.protocol !== 'dsh-app:' || url.hostname !== 'app') event.preventDefault()
  })
  window.webContents.on('page-title-updated', (event, title) => {
    event.preventDefault()
    window.setTitle(active ? `${title} · ${new URL(active.serverUrl).host}` : 'AgentWork')
  })
  window.webContents.setWindowOpenHandler(({ url }) => {
    if (new URL(url).protocol === 'https:') void shell.openExternal(url)
    return { action: 'deny' }
  })
  const shellLocale = resolveDesktopLocale(app.getLocale())
  let refreshMenu = () => {}
  const shortcuts = installDesktopShortcuts(() => window.isDestroyed() ? undefined : window, directory,
    process.platform === 'darwin' ? 'macos' : process.platform === 'win32' ? 'windows' : 'linux',
    () => { refreshMenu() }, () => ({ revision: 0, blocked: false }))
  shortcuts.attach(window)
  refreshMenu = () =>{  Menu.setApplicationMenu(Menu.buildFromTemplate([
    { label: 'AgentWork', submenu: [{ label: copy.reconnect, click: () => {  setup() } }, { type: 'separator' }, { label: copy.quit, role: 'quit' }] },
    shortcuts.fileMenu(shellLocale.messages), { role: 'editMenu' }, { role: 'viewMenu' }, { role: 'windowMenu' },
  ])) }
  refreshMenu()
  window.once('closed', () => { requests.abort(); app.quit() })
  app.once('will-quit', () => {
    requests.abort()
    shortcuts.dispose()
    partition.webRequest.onBeforeSendHeaders(null)
    partition.protocol.unhandle('dsh-app')
    for (const key of ['state', 'connect', 'boot', 'failed']) ipcMain.removeHandler('dsh-remote:' + key)
  })
  await window.loadURL('dsh-app://app/')
  focus()
  return focus
}

/** Categorize native failures without exposing server details or credentials. */
function connectionFailure(error: unknown): 'address' | 'authentication' | 'certificate' | 'version' | 'network' | 'storage' {
  if (error instanceof RemoteCredentialFileError) return 'storage'
  if (error instanceof RemoteClientError) {
    if (error.code === 'remote-credential-storage-unavailable') return 'storage'
    if (error.code === 'remote-server-address') return 'address'
    if (error.code === 'remote-auth-token' || error.code === 'remote-authentication' || error.code === 'remote-authentication-required' || error.code === 'remote-bootstrap' && ['401', '403'].includes(error.resource ?? '')) return 'authentication'
    if (['remote-frontend-version', 'remote-plugin-version', 'remote-plugin-integrity', 'remote-wiki-version', 'remote-frontend-integrity', 'remote-bootstrap-invalid'].includes(error.code)) return 'version'
  }
  const cause = error instanceof Error && error.cause !== null && typeof error.cause === 'object' && 'code' in error.cause ? error.cause.code : undefined
  if (typeof cause === 'string' && ['CERT_HAS_EXPIRED', 'DEPTH_ZERO_SELF_SIGNED_CERT', 'UNABLE_TO_VERIFY_LEAF_SIGNATURE', 'SELF_SIGNED_CERT_IN_CHAIN', 'ERR_TLS_CERT_ALTNAME_INVALID'].includes(cause)) return 'certificate'
  return 'network'
}
