/** Source-built carrier tests cover byte streaming, cancellation and socket ordering. */
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { randomUUID } from 'node:crypto'
import { JSDOM } from 'jsdom'
import { build } from 'tsdown'
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest'
import type { NativeAgentWork, NativeError, NativeEvent } from '../src/native.ts'

let source: string
let directory: string
beforeAll(async () => {
  directory = await mkdtemp(join(tmpdir(), 'dsh-ios-carrier-'))
  await build({ entry: ['apps/ios/src/carrier.ts'], outDir: directory, format: 'iife', platform: 'browser', deps: { alwaysBundle: ['@deepseek-ai/dsh-util-crypto', '@deepseek-ai/dsh-brand'] }, config: false, logLevel: 'silent' })
  source = await readFile(join(directory, 'carrier.iife.js'), 'utf8')
})
afterAll(async () => { if (directory !== undefined) await rm(directory, { recursive: true, force: true }) })

function fixture(withProof = true) {
  const dom = new JSDOM('', { url: 'dsh-app://app/index.html', runScripts: 'outside-only' })
  const window = dom.window
  let http: ((event: NativeEvent | null, error?: NativeError) => void) | undefined
  let socket: typeof http
  const cancel = vi.fn(async () => {})
  const send = vi.fn<NativeAgentWork['send']>(async () => {})
  const close = vi.fn<NativeAgentWork['close']>(async () => {})
  const request = vi.fn<NativeAgentWork['request']>((_options, callback) => { http = callback; return 'http' })
  const open = vi.fn<NativeAgentWork['socket']>((_options, callback) => { socket = callback; return 'ws' })
  const plugin: NativeAgentWork = {
    request, socket: open, cancel, send, close,
    state: async () => ({ serverUrl: '' }), connect: async () => {}, failed: async () => {},
    bootstrap: async () => ({ injections: [], streamBaseUrl: 'https://server.test' }),
  }
  const originalFetch = vi.fn(async () => new Response('static'))
  class LocalRequest extends Request {
    constructor(input: RequestInfo | URL, init?: RequestInit) {
      super(typeof input === 'string' ? new URL(input, window.location.href) : input, init)
    }
  }
  Object.assign(window, { Request: LocalRequest, Response, ReadableStream, TextEncoder, TextDecoder, fetch: originalFetch,
    ...(withProof ? { __AGENTWORK_NATIVE_NONCE__: 'fixture-native-proof' } : {}),
    Capacitor: { Plugins: { AgentWork: plugin } } })
  Object.defineProperty(window.crypto, 'randomUUID', { value: randomUUID })
  window.eval(source)
  return { window, cancel, send, close, request, open, originalFetch, dispose: () => { dom.window.close() },
    http: (event: NativeEvent) => { if (http === undefined) throw new Error('HTTP not started'); http(event) },
    socket: (event: NativeEvent) => { if (socket === undefined) throw new Error('socket not started'); socket(event) },
  }
}

describe('iOS local transport', () => {
  it('does not forward a request from a document without the native proof', async () => {
    const app = fixture(false)
    try {
      await expect(app.window.fetch('/api/rpc')).rejects.toThrow('native_bridge')
      expect(app.request).not.toHaveBeenCalled()
    } finally { app.dispose() }
  })
  it('streams backend bytes and leaves local assets with WebKit', async () => {
    const app = fixture()
    try {
      const pending = app.window.fetch('/api/rpc')
      expect(app.request).toHaveBeenCalledWith(expect.objectContaining({ path: '/api/rpc', method: 'GET' }), expect.any(Function))
      app.http({ type: 'head', status: 200, headers: { 'content-type': 'text/plain' } })
      const response = await pending
      const reader = response.body!.getReader()
      app.http({ type: 'data', data: btoa('saved ') })
      expect(new TextDecoder().decode((await reader.read()).value)).toBe('saved ')
      app.http({ type: 'data', data: btoa('revision') })
      expect(new TextDecoder().decode((await reader.read()).value)).toBe('revision')
      app.http({ type: 'end' })
      expect((await reader.read()).done).toBe(true)
      expect(await (await app.window.fetch('/assets/main.js')).text()).toBe('static')
      expect(app.originalFetch).toHaveBeenCalledOnce()
    } finally { app.dispose() }
  })

  it('aborts the native request and discards late bytes', async () => {
    const app = fixture()
    try {
      const controller = new AbortController()
      const pending = app.window.fetch('/api/rpc', { signal: controller.signal })
      app.http({ type: 'head', status: 200, headers: {} })
      const response = await pending
      const reading = response.text()
      controller.abort()
      await expect(reading).rejects.toMatchObject({ name: 'AbortError' })
      expect(app.cancel).toHaveBeenCalledOnce()
      app.http({ type: 'data', data: btoa('late') })
      app.http({ type: 'end' })
    } finally { app.dispose() }
  })

  it('preserves WebSocket protocol, binary delivery, send order and abnormal closure', async () => {
    const app = fixture()
    try {
      const ws = new app.window.WebSocket('wss://server.test/api/remote.mux', ['dsh-stream.v1'])
      expect(() => { ws.send('early') }).toThrow()
      const opened = vi.fn(), message = vi.fn<(event: MessageEvent<unknown>) => void>(), closed = vi.fn<(event: CloseEvent) => void>()
      ws.onopen = opened; ws.onmessage = message; ws.onclose = closed
      app.socket({ type: 'open', protocol: 'dsh-stream.v1' })
      expect(ws.protocol).toBe('dsh-stream.v1'); expect(opened).toHaveBeenCalledOnce()
      ws.binaryType = 'arraybuffer'
      app.socket({ type: 'message', data: btoa('bytes') })
      const received = message.mock.calls[0]![0].data
      expect(Object.prototype.toString.call(received)).toBe('[object ArrayBuffer]')
      expect(new TextDecoder().decode(received as ArrayBuffer)).toBe('bytes')
      ws.send('first'); ws.send('second'); ws.close()
      await vi.waitFor(() => { expect(app.close).toHaveBeenCalledOnce() })
      expect(app.send.mock.calls.map(call => call[0])).toEqual([
        expect.objectContaining({ text: 'first' }), expect.objectContaining({ text: 'second' }),
      ])
      app.socket({ type: 'close', code: 1006, reason: '', clean: false })
      expect(ws.readyState).toBe(3); expect(closed.mock.calls[0]![0].wasClean).toBe(false)
    } finally { app.dispose() }
  })
})
