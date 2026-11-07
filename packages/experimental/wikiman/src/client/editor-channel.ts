/** Host subset of the portable editor's scoped handshake and drain protocol. */
import { randomUUID } from '@deepseek-ai/dsh-util-crypto'
import type { WikiLink, WikiPageEditor } from '../types.ts'

const deadlineMs = 15_000
const nativeId = /^\d{14}-[a-z0-9]{7}$/

interface Navigation {
  readonly focus: { readonly blockId: string; readonly hash: string } | undefined
  readonly resolve: (blockId: string, signal: AbortSignal) => Promise<WikiLink | null>
  readonly open: (link: WikiLink) => void
}

function isMessage(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

/** One iframe's ready signal and save-and-release command. */
export interface EditorChannel {
  readonly ready: Promise<void>
  /** Drain pending editor writes and detach the frame binding. */
  drain(): Promise<void>
  /** Revoke this local channel and reject outstanding commands. */
  close(): void
}

/**
 * Admit one iframe at the kernel origin and send its view on a private port.
 * @param frame - browser-owned iframe, still at its initial blank page.
 * @param editor - capability issued for this document.
 * @param onDirty - receives the editor's unsaved state.
 * @param navigation - host-owned link resolution and activation.
 * @returns commands scoped to this frame and document.
 */
export function mountEditor(
  frame: HTMLIFrameElement, editor: WikiPageEditor, onDirty: (dirty: boolean) => void, navigation: Navigation,
): EditorChannel {
  const origin = new URL(editor.baseUrl)
  if (origin.protocol !== 'http:' || origin.hostname !== '127.0.0.1' || origin.origin !== editor.baseUrl) throw new Error('editor_origin')
  const channelId = randomUUID()
  const { pageId, storeId, sessionId } = editor.view
  const identity = { channelId, pageId, storeId, sessionId }
  const ready = Promise.withResolvers<void>()
  const ports = new MessageChannel()
  let pending: { id: string; resolve(): void; reject(error: Error): void; timer: ReturnType<typeof setTimeout> } | undefined
  let closed = false
  let commandSequence = 0
  const lifetime = new AbortController()
  let navigating = false
  const stop = () => {
    if (closed) return
    closed = true
    lifetime.abort()
    window.removeEventListener('message', boot)
    clearTimeout(timer)
    ports.port1.close()
    if (!transferred) ports.port2.close()
    ready.reject(new Error('editor_channel_closed'))
    if (pending) {
      clearTimeout(pending.timer)
      pending.reject(new Error('editor_channel_closed'))
      pending = undefined
    }
  }
  let transferred = false
  const timer = setTimeout(stop, deadlineMs)
  const boot = (event: MessageEvent<unknown>) => {
    const target = frame.contentWindow
    if (!isMessage(event.data) || event.source !== target || event.origin !== origin.origin || event.data.type !== 'wiki:editor:booted'
      || event.data.channelId !== channelId || !target) return
    window.removeEventListener('message', boot)
    transferred = true
    target.postMessage({
      type: 'wiki:editor:init', channelId,
      payload: {
        baseUrl: editor.baseUrl,
        view: editor.view,
        document: editor.document && { boxId: editor.document.boxId, docId: editor.document.docId },
        ...(navigation.focus && { focus: navigation.focus }),
      },
    }, origin.origin, [ports.port2])
  }
  const navigate = async (requestId: string, blockId: string) => {
    const respond = (ok: boolean) => {
      if (!closed) ports.port1.postMessage({ ...identity, type: 'navigationResult', requestId, ok, code: 'reference_navigation_unavailable' })
    }
    if (navigating) { respond(false); return }
    navigating = true
    try {
      const link = await navigation.resolve(blockId, lifetime.signal)
      if (closed) return
      respond(link !== null)
      if (link) navigation.open(link)
    } catch (_error) {
      respond(false)
    } finally {
      navigating = false
    }
  }
  ports.port1.onmessage = (event: MessageEvent<unknown>) => {
    const reply = event.data
    if (!isMessage(reply) || Object.entries(identity).some(([key, value]) => reply[key] !== value)) return
    if (reply.type === 'ready') {
      clearTimeout(timer)
      ready.resolve()
    } else if (reply.type === 'error') {
      stop()
    } else if (reply.type === 'navigate' && typeof reply.requestId === 'string' && reply.requestId.length <= 128
      && typeof reply.blockId === 'string' && nativeId.test(reply.blockId)) {
      void navigate(reply.requestId, reply.blockId)
    } else if (reply.type === 'dirty' && typeof reply.dirty === 'boolean') {
      onDirty(reply.dirty)
    } else if (reply.type === 'result' && pending && reply.requestId === pending.id) {
      clearTimeout(pending.timer)
      const request = pending
      pending = undefined
      if (reply.ok === true && reply.dirty === false) request.resolve()
      else request.reject(new Error(typeof reply.code === 'string' ? reply.code : 'editor_unsaved'))
    }
  }
  window.addEventListener('message', boot)
  frame.src = `${origin.origin}/stage/build/portable/#${new URLSearchParams({ parentOrigin: location.origin, channelId })}`
  return {
    ready: ready.promise,
    close: stop,
    async drain() {
      await ready.promise
      if (closed || pending) throw new Error('editor_channel_unavailable')
      const result = Promise.withResolvers<void>()
      const id = `command-${++commandSequence}`
      pending = { id, ...result, timer: setTimeout(() => { pending = undefined; result.reject(new Error('editor_drain_timeout')) }, deadlineMs) }
      ports.port1.postMessage({ ...identity, type: 'dispose', requestId: id })
      await result.promise
      stop()
    },
  }
}
