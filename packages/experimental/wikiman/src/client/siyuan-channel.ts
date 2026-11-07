/** Browser-side lifecycle of the original SiYuan desktop frame. */
import type { WikiSiyuanView } from '../types.ts'
import type { EditorChannel } from './editor-channel.ts'

/**
 * Mount the original application and wait for its native save queue before navigation.
 * @param frame - anchored workbench frame.
 * @param view - owner-authorized native editor.
 * @returns the bridge lifecycle.
 */
export function mountSiyuan(frame: HTMLIFrameElement, view: WikiSiyuanView): EditorChannel {
  const ready = Promise.withResolvers<void>()
  const lifetime = new EventSource(view.lifetimeUrl)
  let draining: ReturnType<typeof Promise.withResolvers<void>> | undefined
  let closed = false
  let drainTimer: ReturnType<typeof setTimeout> | undefined
  const bootTimer = setTimeout(() => { ready.reject(new Error('siyuan_boot_timeout')) }, 20_000)
  const receive = (event: MessageEvent<unknown>) => {
    const reply = event.data
    if (event.source !== frame.contentWindow || event.origin !== view.origin || typeof reply !== 'object' || reply === null
      || !('viewId' in reply) || reply.viewId !== view.viewId || !('type' in reply)) return
    if (reply.type === 'siyuan:ready' || reply.type === 'siyuan:error') {
      clearTimeout(bootTimer)
      if (reply.type === 'siyuan:ready') ready.resolve()
      else ready.reject(new Error('siyuan_boot_failed'))
    }
    if (reply.type === 'siyuan:drained' && draining) {
      clearTimeout(drainTimer)
      if ('ok' in reply && reply.ok === true) draining.resolve()
      else draining.reject(new Error('siyuan_save_failed'))
      draining = undefined
    }
  }
  window.addEventListener('message', receive)
  frame.src = view.url
  return {
    ready: ready.promise,
    close() {
      if (closed) return
      closed = true
      lifetime.close()
      clearTimeout(bootTimer)
      clearTimeout(drainTimer)
      window.removeEventListener('message', receive)
      ready.reject(new Error('siyuan_closed'))
      draining?.reject(new Error('siyuan_closed'))
    },
    async drain() {
      await ready.promise
      const target = frame.contentWindow
      if (!target || closed || draining) throw new Error('siyuan_unavailable')
      const request = Promise.withResolvers<void>()
      draining = request
      drainTimer = setTimeout(() => {
        draining = undefined
        request.reject(new Error('siyuan_save_timeout'))
      }, 15_000)
      target.postMessage({ type: 'siyuan:drain', viewId: view.viewId }, view.origin)
      await request.promise
    },
  }
}
