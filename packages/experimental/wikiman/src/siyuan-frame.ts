/** Same-origin bridge for waiting on the original SiYuan desktop's save queue. */

/** Served beside the unmodified SiYuan assets on the kernel origin. */
export const siyuanFrame = String.raw`<!doctype html>
<meta charset="utf-8">
<meta name="referrer" content="no-referrer">
<style>
html,body,iframe {width:100%;height:100%;margin:0;border:0}
iframe {display:block}
</style>
<script type="module">
const params = new URLSearchParams(location.hash.slice(1))
const parentOrigin = params.get('parent')
const viewId = params.get('view')
const mode = params.get('mode')
const id = params.get('id')
const nativeId = /^\d{14}-[a-z0-9]{7}$/
if (parentOrigin && /^http:\/\/127\.0\.0\.1:\d+$/.test(parentOrigin) &&
    viewId && /^[a-f0-9-]{36}$/.test(viewId) && (mode === 'desktop' || mode === 'single') &&
    (!id || nativeId.test(id))) {
  history.replaceState(null, '', location.pathname)
  const send = (type, fields = {}) => parent.postMessage({ type, viewId, ...fields }, parentOrigin)
  const frame = document.createElement('iframe')
  frame.title = 'SiYuan'
  const url = new URL('/stage/build/' + mode + '/', location.origin)
  url.searchParams.set('wikiman_reload', '0')
  if (id) url.searchParams.set('id', id)
  frame.src = url.href
  let editor, pending = 0, failed = false, switching = false
  frame.addEventListener('load', () => {
    try {
      editor = frame.contentWindow
      const original = editor.fetch
      editor.fetch = function (input, init) {
        const resource = typeof input === 'string' || input instanceof editor.URL ? input : input.url
        const transaction = new URL(resource, editor.location.href).pathname === '/api/transactions'
        if (!transaction) return original.call(this, input, init)
        pending++
        return original.call(this, input, init).then(async response => {
          const result = await response.clone().json()
          failed ||= !response.ok || result.code !== 0
          return response
        }).catch(error => {
          failed = true
          throw error
        }).finally(() => { pending-- })
      }
      send('siyuan:ready')
    } catch {
      send('siyuan:error')
    }
  })
  addEventListener('message', async event => {
    if (event.origin !== parentOrigin || event.source !== parent || event.data?.viewId !== viewId) return
    if (event.data.type === 'siyuan:drain' && !switching) {
      switching = true
      frame.inert = true
      editor?.document.activeElement?.blur()
      const deadline = performance.now() + 12000
      try {
        while (pending || editor?.siyuan?.transactions?.length) {
          if (performance.now() > deadline) throw new Error('save timeout')
          await new Promise(resolve => setTimeout(resolve, 50))
        }
        send('siyuan:drained', { ok: !failed })
      } catch {
        send('siyuan:drained', { ok: false })
      } finally {
        frame.inert = false
        switching = false
      }
    }
  })
  document.body.append(frame)
}
</script>`
