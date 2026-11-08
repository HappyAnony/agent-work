/** Browser entry for the Web client. */
import '@deepseek-ai/dsh-client-ui-theme/brand-font.css'
import { PackagedServer, PackagedServerError, type ServerFailure } from '@deepseek-ai/dsh-client-connection/packaged-server'
import type { ClientTransportHooks } from '@deepseek-ai/dsh-client-connection/client'
import { AppWebEntry, applyIndexInjections } from '@deepseek-ai/dsh-client-web'

type Injections = Parameters<typeof applyIndexInjections>[0]
interface NativeServerBridge {
  state(): Promise<{ serverUrl: string }>
  connect(address: string, token: string): Promise<{ ok: boolean; failure?: ServerFailure; injections?: Injections }>
  onConfigure?(listener: () => void): () => void
}
interface DesktopBootGlobal {
  agentworkServer?: NativeServerBridge
  dshDesktopBoot?: {
    failed(message: string): Promise<void>
    ready(): Promise<{ injections: Parameters<typeof applyIndexInjections>[0]; streamBaseUrl: string; ownsHost?: boolean }>
  }
}
const desktop = (globalThis as DesktopBootGlobal).dshDesktopBoot
const reportFailure = (reason: unknown): void => {
  if (desktop === undefined) throw reason
  void desktop.failed(reason instanceof Error ? reason.message : String(reason)).catch(console.error)
}

try {
  const el = document.getElementById('root')
  if (el === null) throw new Error('web app: missing #root')
  const entry = new AppWebEntry(el)
  const native = (globalThis as DesktopBootGlobal).agentworkServer
  const server = native === undefined ? undefined : new PackagedServer({
    readAddress: async () => (await native.state()).serverUrl,
    connect: async (address, token) => {
      try { await entry.preparePackagedConnection() }
      catch (_error) { throw new PackagedServerError('unsaved') }
      const result = await native.connect(address, token)
      if (!result.ok || result.injections === undefined) throw new PackagedServerError(result.failure ?? 'network')
      const globals: Record<string, unknown> = {}
      for (const row of result.injections) if (row.kind === 'global') globals[row.name] = row.value
      await entry.connectPackaged(globals, globals.__DSH_BOOT__)
      return { globals, graph: globals.__DSH_BOOT__ }
    },
  }, '')
  native?.onConfigure?.(() => { server?.requestConfiguration() })
  if (desktop !== undefined) {
    const gate = (globalThis as { __DSH_BOOT_READY__?: PromiseWithResolvers<void> }).__DSH_BOOT_READY__
    if (gate === undefined) throw new Error('desktop web: boot readiness is missing')
    void desktop.ready().then(async ({ injections, streamBaseUrl, ownsHost }) => {
      const transport = globalThis as { __DSH_TRANSPORT__?: ClientTransportHooks }
      transport.__DSH_TRANSPORT__ = { ownsHost: ownsHost ?? true,
        get streamBaseUrl() { return server?.getSnapshot().address || streamBaseUrl },
        ...server === undefined ? {} : { server },
      }
      await applyIndexInjections(injections, src => new Promise<void>((resolve, reject) => {
        const script = document.createElement('script')
        script.src = src
        script.onload = () => { resolve() }
        script.onerror = () => { reject(new Error(`desktop web: failed to load ${src}`)) }
        document.head.append(script)
      }))
      gate.resolve()
    }).catch((error: unknown) => { gate.reject(error) })
  }
  void entry.run(desktop === undefined ? undefined : reportFailure).then(() => server?.start())
} catch (reason) {
  reportFailure(reason)
}
