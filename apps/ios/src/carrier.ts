/** Local WKWebView transport: streaming HTTP and WebSockets use system-verified TLS. */
import { decode, encode, nativeAgentWork, type NativeTransferId } from './native.ts'
import { randomUUID } from '@deepseek-ai/dsh-util-crypto'
import { brandString } from '@deepseek-ai/dsh-brand'

if (location.protocol === 'dsh-app:' && location.hostname === 'app') {
  const originalFetch = window.fetch.bind(window)
  window.fetch = async (input, init) => {
    const request = new Request(input, init)
    const url = new URL(request.url)
    if (url.protocol !== location.protocol || url.host !== location.host || !url.pathname.startsWith('/api/')) {
      return originalFetch(input, init)
    }
    request.signal.throwIfAborted()
    const plugin = nativeAgentWork()
    const id = brandString<NativeTransferId>('http-' + randomUUID())
    const headers = Object.fromEntries(request.headers.entries())
    const bytes = request.body === null ? undefined : new Uint8Array(await request.arrayBuffer())
    if (bytes !== undefined && bytes.byteLength > 32 * 1024 * 1024) throw new Error('transfer_limit')
    return new Promise<Response>((resolve, reject) => {
      let stream: ReadableStreamDefaultController<Uint8Array> | undefined
      let ended = false
      const finish = (error?: Error) => {
        if (ended) return
        ended = true
        request.signal.removeEventListener('abort', abort)
        if (error === undefined) stream?.close()
        else { stream?.error(error); reject(error) }
      }
      const abort = () => { finish(new DOMException('Aborted', 'AbortError')); void plugin.cancel({ id }).catch(console.error) }
      request.signal.addEventListener('abort', abort, { once: true })
      plugin.request({ id, path: url.pathname + url.search, method: request.method, headers,
        ...(bytes === undefined ? {} : { body: encode(bytes) }) }, (event, error) => {
        if (ended) return
        if (error !== undefined) { finish(new Error(error.message)); return }
        if (event?.type === 'head') {
          const body = new ReadableStream<Uint8Array>({ start(controller) { stream = controller }, cancel() { abort() } })
          resolve(new Response([101, 204, 205, 304].includes(event.status) || request.method === 'HEAD' ? null : body,
            { status: event.status, headers: event.headers }))
        } else if (event?.type === 'data') stream?.enqueue(decode(event.data))
        else if (event?.type === 'end') finish()
      })
      if (request.signal.aborted) abort()
    })
  }

  class NativeSocket extends EventTarget {
    static readonly CONNECTING = 0
    static readonly OPEN = 1
    static readonly CLOSING = 2
    static readonly CLOSED = 3
    readonly CONNECTING = 0
    readonly OPEN = 1
    readonly CLOSING = 2
    readonly CLOSED = 3
    readonly url: string
    protocol = ''
    extensions = ''
    readyState = 0
    bufferedAmount = 0
    binaryType: BinaryType = 'blob'
    onopen: ((event: Event) => void) | null = null
    onerror: ((event: Event) => void) | null = null
    onclose: ((event: CloseEvent) => void) | null = null
    onmessage: ((event: MessageEvent) => void) | null = null
    private readonly id = brandString<NativeTransferId>('ws-' + randomUUID())
    private sending = Promise.resolve()

    constructor(url: string | URL, protocols: string | string[] = []) {
      super()
      this.url = String(url)
      nativeAgentWork().socket({ id: this.id, url: this.url, protocols: typeof protocols === 'string' ? [protocols] : protocols }, (event, error) => {
        if (this.readyState === 3) return
        if (error !== undefined) { this.fail(); return }
        if (event?.type === 'open') {
          if (this.readyState !== 0) return
          this.readyState = 1; this.protocol = event.protocol
          const value = new Event('open'); this.dispatchEvent(value); this.onopen?.(value)
        } else if (event?.type === 'message') {
          const data = event.text ?? (this.binaryType === 'blob' ? new Blob([decode(event.data ?? '')]) : decode(event.data ?? '').buffer)
          const value = new MessageEvent('message', { data }); this.dispatchEvent(value); this.onmessage?.(value)
        } else if (event?.type === 'close') this.ended(event.code, event.reason, event.clean)
      })
    }

    send(data: string | ArrayBufferLike | Blob | ArrayBufferView): void {
      if (this.readyState === 0) throw new DOMException('Connecting', 'InvalidStateError')
      if (this.readyState !== 1) return
      const size = typeof data === 'string' ? new TextEncoder().encode(data).byteLength : data instanceof Blob ? data.size : data.byteLength
      if (size > 32 * 1024 * 1024) throw new Error('transfer_limit')
      this.bufferedAmount += size
      this.sending = this.sending.then(async () => {
        if (typeof data === 'string') await nativeAgentWork().send({ id: this.id, text: data })
        else {
          const bytes = data instanceof Blob ? new Uint8Array(await data.arrayBuffer())
            : ArrayBuffer.isView(data) ? new Uint8Array(data.buffer, data.byteOffset, data.byteLength) : new Uint8Array(data)
          await nativeAgentWork().send({ id: this.id, data: encode(bytes) })
        }
      }).catch(() => { this.fail() }).finally(() => { this.bufferedAmount -= size })
    }

    close(code = 1000, reason = ''): void {
      if (code !== 1000 && (code < 3000 || code > 4999)) throw new DOMException('Invalid close code', 'InvalidAccessError')
      if (new TextEncoder().encode(reason).length > 123) throw new DOMException('Close reason too long', 'SyntaxError')
      if (this.readyState >= 2) return
      this.readyState = 2
      void this.sending.then(() => nativeAgentWork().close({ id: this.id, code, reason })).catch(() => { this.fail() })
    }

    private fail(): void {
      if (this.readyState === 3) return
      const event = new Event('error'); this.dispatchEvent(event); this.onerror?.(event)
      this.ended(1006, '', false)
      void nativeAgentWork().close({ id: this.id, code: 1000, reason: '' }).catch(console.error)
    }
    private ended(code: number, reason: string, wasClean: boolean): void {
      if (this.readyState === 3) return
      this.readyState = 3
      const event = new CloseEvent('close', { code, reason, wasClean }); this.dispatchEvent(event); this.onclose?.(event)
    }
  }
  // The class implements the browser operations consumed by the shared client.
  window.WebSocket = NativeSocket as typeof WebSocket

  if (window === window.top && location.pathname !== '/connect.html') {
    Object.assign(window, {
      __DSH_BOOT_READY__: Promise.withResolvers<void>(),
      dshDesktopBoot: {
        ready: async () => ({ ...await nativeAgentWork().bootstrap(), ownsHost: false }),
        failed: (message: string) => nativeAgentWork().failed({ message }),
      },
    })
  }
}
