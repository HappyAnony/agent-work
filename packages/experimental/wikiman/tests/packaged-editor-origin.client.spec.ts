// @vitest-environment jsdom
/** The packaged iOS address must complete the shared editor's private-channel handshake. */
import { once } from 'node:events'
import { MessageChannel } from 'node:worker_threads'
import { expect, it, vi } from 'vitest'
import iosConfig from '../../../../apps/ios/capacitor.config.json' with { type: 'json' }
import { connectEditorFrame } from '../src/client/portable-channel.ts'

it('admits the configured iOS window without relying on URL.origin for custom schemes', async () => {
  const origin = `${iosConfig.server.iosScheme}://${iosConfig.server.hostname}`
  const native = new MessageChannel()
  const closed = Promise.all([once(native.port1, 'close'), once(native.port2, 'close')])
  vi.stubGlobal('MessageChannel', function () { return native })
  const posted = vi.spyOn(window, 'postMessage').mockImplementation(() => {})
  let editor: ReturnType<typeof connectEditorFrame> | undefined
  try {
    editor = connectEditorFrame({
      frameWindow: window, frameOrigin: origin, channelId: 'fixture-channel', dirty: () => {},
      payload: { baseUrl: origin + '/api/wikiman', document: null, view: {
        storeId: 'fixture-store', pageId: '1760000000000000001', sessionId: 'fixture-session',
        viewId: 'fixture-view', ticket: 'fixture-ticket', mode: 'wikiDesktop',
      } },
    })
    native.port2.postMessage({ type: 'ready', channelId: 'fixture-channel', storeId: 'fixture-store',
      pageId: '1760000000000000001', sessionId: 'fixture-session' })
    await editor.ready
    expect(posted).toHaveBeenCalledWith(expect.objectContaining({ type: 'wiki:editor:init' }), origin, [native.port2])
  } finally {
    editor?.close()
    native.port1.close(); native.port2.close()
    await closed
    vi.restoreAllMocks(); vi.unstubAllGlobals()
  }
})
