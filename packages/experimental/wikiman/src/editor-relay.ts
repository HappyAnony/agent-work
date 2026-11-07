/** Authenticated browser transport to the private portable kernel; no management credential crosses it. */
import { request as httpRequest } from 'node:http'
import type { Duplex } from 'node:stream'
import type { Context } from '@deepseek-ai/cordis'
import type {} from '@deepseek-ai/dsh-client-connection'
import type {} from '@deepseek-ai/dsh-host-webserver'
import type { WikiKernel } from './kernel.ts'

/** Public path reserved for page-ticket editor operations. */
export const EDITOR_API_PATH = '/api/wikiman'
const endpoints = ['/v1/content', '/v1/ui', '/v1/resource'] as const
const assetPaths = ['/stage', '/appearance', '/emojis'] as const

function bounded(body: ReadableStream<Uint8Array> | null, limit: number): ReadableStream<Uint8Array> | null {
  if (!body) return null
  let bytes = 0
  return body.pipeThrough(new TransformStream<Uint8Array, Uint8Array>({
    transform(chunk, controller) {
      bytes += chunk.byteLength
      if (bytes > limit) throw new Error('wikiman editor transfer exceeds configured byte limit')
      controller.enqueue(chunk)
    },
  }))
}

/**
 * Register editor HTTP, assets and WebSocket routes for one plugin lifetime.
 * @param ctx - Web composition with Connection admission.
 * @param kernel - Current kernel; maintenance may temporarily make it unavailable.
 * @param config - Deployment transfer limits and request timeout.
 * @returns Cleanup that aborts transfers and waits for upgraded sockets to close.
 */
export function installEditorRelay(ctx: Context, kernel: () => WikiKernel, config: {
  readonly maxEditorBytes: number
  readonly requestTimeoutMs: number
}): () => Promise<void> {
  const lifetime = new AbortController()
  const sockets = new Set<Duplex>()
  const completions = new Set<Promise<void>>()
  const track = (socket: Duplex) => {
    sockets.add(socket)
    const done = new Promise<void>(resolve => socket.once('close', resolve)).finally(() => {
      sockets.delete(socket)
      completions.delete(done)
    })
    completions.add(done)
    socket.on('error', (_error: Error) => { socket.destroy() })
  }
  const forward = async (incoming: Request, path: string, assets: boolean): Promise<Response> => {
    const current = kernel()
    const upstream = new URL(path, current.identity().baseUrl)
    upstream.search = new URL(incoming.url).search
    const headers = new Headers()
    for (const name of ['content-type', 'x-wiki-editor', 'x-wiki-session', 'range', 'if-none-match']) {
      const value = incoming.headers.get(name)
      if (value !== null) headers.set(name, value)
    }
    headers.set('origin', upstream.origin)
    const init = {
      method: incoming.method, headers, redirect: 'error' as const,
      body: bounded(incoming.body, config.maxEditorBytes), duplex: 'half',
      signal: AbortSignal.any([incoming.signal, lifetime.signal, current.signal, AbortSignal.timeout(config.requestTimeoutMs)]),
    }
    const result = await fetch(upstream, init)
    const outgoing = new Headers(result.headers)
    for (const name of ['set-cookie', 'content-encoding', 'content-length', 'connection', 'transfer-encoding']) outgoing.delete(name)
    if (assets) {
      const authority = incoming.headers.get('host')
      const publicOrigin = authority === null ? ctx.webServer.getOrigin() : new URL(`${ctx.webServer.protocol}//${authority}`).origin
      const policy = outgoing.get('content-security-policy')
      if (policy !== null) outgoing.set('content-security-policy', policy
        .replace(/frame-ancestors[^;]*;/u, "frame-ancestors 'self' dsh-app://app;")
        .replaceAll(upstream.origin, publicOrigin)
        .replaceAll(`ws://${upstream.host}`, `${publicOrigin.startsWith('https:') ? 'wss:' : 'ws:'}//${new URL(publicOrigin).host}`))
    }
    return new Response(bounded(result.body, config.maxEditorBytes), { status: result.status, headers: outgoing })
  }
  for (const endpoint of endpoints) ctx.effect(() => ctx.connection.fetch.register({
    path: EDITOR_API_PATH + endpoint,
    methods: endpoint === '/v1/resource' ? ['GET'] : ['POST'], requestBody: 'streaming',
    fetch: request => forward(request, endpoint, false),
  }), `wikiman: ${endpoint}`)
  for (const path of assetPaths) ctx.effect(() => ctx.webServer.register({
    kind: 'prefix', path,
    resource: request => forward(request, new URL(request.url).pathname, true),
  }), `wikiman: editor assets ${path}`)

  ctx.effect(() => ctx.webServer.registerUpgrade({
    path: EDITOR_API_PATH + '/v1/events',
    handler: (incoming, socket, head) => {
      const admitted = ctx.connection.admit(incoming)
      if ('rejection' in admitted) {
        socket.end('HTTP/1.1 403 Forbidden\r\nConnection: close\r\n\r\n')
        return
      }
      const current = kernel()
      const target = new URL('/v1/events', current.identity().baseUrl)
      target.search = new URL(incoming.url ?? '/', 'http://relay.invalid').search
      const headers: Record<string, string | string[]> = { host: target.host, origin: target.origin, connection: 'Upgrade', upgrade: 'websocket' }
      for (const name of ['sec-websocket-key', 'sec-websocket-version', 'sec-websocket-protocol']) {
        const value = incoming.headers[name]
        if (value !== undefined) headers[name] = value
      }
      track(socket)
      const upstream = httpRequest(target, { headers,
        signal: AbortSignal.any([lifetime.signal, current.signal]), timeout: config.requestTimeoutMs })
      socket.once('close', () => { upstream.destroy() })
      upstream.once('timeout', () => { upstream.destroy(new Error('wikiman editor upgrade timed out')) })
      upstream.once('error', (_error: Error) => { socket.destroy() })
      upstream.once('response', (response) => {
        response.resume()
        socket.end(`HTTP/1.1 ${response.statusCode ?? 502} Rejected\r\nConnection: close\r\n\r\n`)
      })
      upstream.once('upgrade', (response, remote, initial) => {
        upstream.setTimeout(0)
        remote.setTimeout(0)
        if (lifetime.signal.aborted || current.signal.aborted || socket.destroyed) { remote.destroy(); return }
        track(remote)
        remote.once('close', () => { socket.destroy() })
        socket.once('close', () => { remote.destroy() })
        const fields = ['upgrade', 'connection', 'sec-websocket-accept', 'sec-websocket-protocol']
          .flatMap(name => typeof response.headers[name] === 'string' ? [`${name}: ${response.headers[name]}`] : [])
        socket.write('HTTP/1.1 101 Switching Protocols\r\n' + fields.join('\r\n') + '\r\n\r\n')
        if (initial.length) socket.write(initial)
        if (head.length) remote.write(head)
        socket.pipe(remote).pipe(socket)
      })
      upstream.end()
    },
  }), 'wikiman: editor events')
  return async () => {
    lifetime.abort()
    for (const socket of sockets) socket.destroy()
    await Promise.all(completions)
  }
}
