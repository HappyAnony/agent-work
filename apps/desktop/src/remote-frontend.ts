/** Verified local browser resources for a desktop client of an independently running Host. */
import { createHash } from 'node:crypto'
import { readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { z } from 'zod'
import { serveWebDocument } from './web-document.ts'

const digest = z.string().regex(/^[a-f0-9]{64}$/u)
const packageName = /^@[a-z0-9._-]+\/[a-z0-9._-]+$/u
const manifestSchema = z.object({
  protocolVersion: z.literal(1), frontendHash: digest,
  bundles: z.record(z.string().regex(packageName), digest),
  wikiHash: digest,
})
/** Server startup inputs admitted after resource compatibility verification. */
export const remoteBootstrapSchema = z.object({
  protocolVersion: z.literal(1), frontendHash: digest,
  bundles: z.record(z.string().regex(packageName), digest), injections: z.array(z.unknown()),
})

/** Failed remote startup operation; the client must remain on its connection form. */
export class RemoteClientError extends Error {
  /**
   * Retain a stable failure code and optional resource identifier without credentials.
   * @param code - Failed validation or connection operation.
   * @param resource - Package identifier or HTTP status, when relevant.
   */
  constructor(readonly code: 'remote-server-address' | 'remote-frontend-integrity' | 'remote-frontend-version' |
    'remote-plugin-version' | 'remote-plugin-integrity' | 'remote-wiki-version' | 'remote-bootstrap-limit' |
    'remote-bootstrap-invalid' | 'remote-auth-token' | 'remote-authentication' | 'remote-authentication-required' |
    'remote-bootstrap' | 'remote-credential-storage-unavailable', readonly resource?: string) {
    super(code + (resource === undefined ? '' : ':' + resource))
  }
}

/**
 * Read a bounded, validated startup response before accepting server-provided inputs.
 * @param response - Successful authenticated bootstrap response.
 * @returns Validated startup inputs; oversized or malformed responses cannot activate the client.
 */
export async function readRemoteBootstrap(response: Response): Promise<z.infer<typeof remoteBootstrapSchema>> {
  const reader = response.body?.getReader()
  if (!reader) throw new RemoteClientError('remote-bootstrap-invalid')
  const chunks: Uint8Array[] = []
  let bytes = 0
  try {
    for (;;) {
      const chunk = await reader.read()
      if (chunk.done) break
      bytes += chunk.value.byteLength
      if (bytes > 8 * 1024 * 1024) {
        await reader.cancel()
        throw new RemoteClientError('remote-bootstrap-limit')
      }
      chunks.push(chunk.value)
    }
  } finally { reader.releaseLock() }
  let value: unknown
  try { value = JSON.parse(Buffer.concat(chunks).toString('utf8')) }
  catch (_error) { throw new RemoteClientError('remote-bootstrap-invalid') }
  const parsed = remoteBootstrapSchema.safeParse(value)
  if (!parsed.success) throw new RemoteClientError('remote-bootstrap-invalid')
  return parsed.data
}

/**
 * Accept a root HTTPS server URL, or HTTP on an exact loopback hostname.
 * @param input - Configured server address, without credentials, paths, query, or fragment.
 * @returns Canonical server origin; rejects unsupported or insecure remote addresses.
 */
export function remoteServerOrigin(input: string): string {
  let url: URL
  try { url = new URL(input) }
  catch (_error) { throw new RemoteClientError('remote-server-address') }
  if (url.username || url.password || url.pathname !== '/' || url.search || url.hash ||
    !(url.protocol === 'https:' || url.protocol === 'http:' && ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname))) {
    throw new RemoteClientError('remote-server-address')
  }
  return url.origin
}

/** Local resources whose fingerprints must match the selected server before client code starts. */
export class RemoteFrontend {
  private constructor(readonly root: string, private readonly manifest: z.infer<typeof manifestSchema>) {}

  /**
   * Read and validate the packaged resource inventory.
   * @param root - Packaged frontend directory.
   * @returns Local resource owner; rejects missing or invalid files.
   */
  static async load(root: string): Promise<RemoteFrontend> {
    const manifest = manifestSchema.parse(JSON.parse(await readFile(join(root, 'manifest.json'), 'utf8')))
    const frontend = new RemoteFrontend(root, manifest)
    if (createHash('sha256').update(await readFile(join(root, 'web/index.html'))).digest('hex') !== manifest.frontendHash) throw new RemoteClientError('remote-frontend-integrity')
    return frontend
  }

  /**
   * Refuse mixed frontend builds before executing server-provided startup inputs.
   * @param bootstrap - Authenticated server startup response.
   * @returns Completion of verification of every required local browser entry.
   */
  async verify(bootstrap: z.infer<typeof remoteBootstrapSchema>): Promise<void> {
    if (bootstrap.frontendHash !== this.manifest.frontendHash) throw new RemoteClientError('remote-frontend-version')
    for (const [id, hash] of Object.entries(bootstrap.bundles)) {
      if (this.manifest.bundles[id] !== hash) throw new RemoteClientError('remote-plugin-version', id)
      const bytes = await readFile(join(this.root, 'plugins', id, 'client.js'))
      if (createHash('sha256').update(bytes).digest('hex') !== hash) throw new RemoteClientError('remote-plugin-integrity', id)
    }
    const wiki = bootstrap.injections.find(row => typeof row === 'object' && row !== null && 'name' in row && row.name === '__DSH_WIKIMAN_ASSETS__')
    if (typeof wiki === 'object' && wiki !== null && 'value' in wiki && wiki.value !== this.manifest.wikiHash) throw new RemoteClientError('remote-wiki-version')
    if (wiki !== undefined) {
      const bytes = await readFile(join(this.root, 'wiki/stage/build/portable/boot.js'))
      if (createHash('sha256').update(bytes).digest('hex') !== this.manifest.wikiHash) throw new RemoteClientError('remote-wiki-version')
    }
  }

  /**
   * Serve packaged UI, editor and plugin resources; API and generated widget resources delegate to the Host.
   * @param request - Request from the owned application origin.
   * @returns Local response, or undefined for a backend-owned path.
   */
  async resource(request: Request): Promise<Response | undefined> {
    const url = new URL(request.url)
    if (url.pathname === '/' || url.pathname === '/index.html' || url.pathname.startsWith('/assets/') ||
      ['/manifest.webmanifest', '/favicon.svg'].includes(url.pathname)) return serveWebDocument(request, join(this.root, 'web'))
    if (['/stage/', '/appearance/', '/emojis/'].some(path => url.pathname.startsWith(path))) {
      if (url.pathname.startsWith('/stage/protyle/js/widget/resources/')) return undefined
      if (url.pathname.endsWith('/')) url.pathname += 'index.html'
      return serveWebDocument(new Request(url, request), join(this.root, 'wiki'))
    }
    if (!url.pathname.startsWith('/plugins/')) return undefined
    if (request.method !== 'GET' && request.method !== 'HEAD') return new Response(null, { status: 405 })
    const combo = url.pathname === '/plugins/' && url.search.startsWith('??')
    if (combo) {
      const entries = decodeURIComponent(url.search.slice(2).split('&')[0] ?? '').split(',')
      const scripts: string[] = []
      for (const entry of entries) {
        const id = entry.replace(/\/client\.js$/u, '')
        if (entry !== id + '/client.js' || !packageName.test(id) || !this.manifest.bundles[id]) return new Response(null, { status: 404 })
        scripts.push((await readFile(join(this.root, 'plugins', id, 'client.js'), 'utf8')).replace(/\/\/[#@] sourceMappingURL=[^\n]*(?:\n|$)/gu, '') + '\n;\n')
      }
      return new Response(request.method === 'HEAD' ? null : scripts.join(''), { headers: { 'content-type': 'text/javascript; charset=utf-8' } })
    }
    const parts = decodeURIComponent(url.pathname.slice('/plugins/'.length)).split('/')
    const id = parts.slice(0, 2).join('/')
    if (!packageName.test(id) || !this.manifest.bundles[id] ||
      parts.some(part => !part || part === '.' || part === '..' || part.includes('\\'))) return new Response(null, { status: 404 })
    const relative = parts.slice(2).join('/')
    if (!/^client(?:\.[A-Za-z0-9_-]+)*\.js$/u.test(relative) && !/^assets\/.+\.(?:svg|png|webp|jpg|jpeg)$/u.test(relative)) {
      return new Response(null, { status: 404 })
    }
    const local = new URL(request.url)
    local.pathname = '/' + relative
    return serveWebDocument(new Request(local, request), join(this.root, 'plugins', id))
  }
}
