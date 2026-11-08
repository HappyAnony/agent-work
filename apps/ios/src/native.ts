/** Native methods keep the server credential in the iOS process. */
export { bytesToBase64 as encode } from '@deepseek-ai/dsh-util-crypto'
import type { Branded } from '@deepseek-ai/dsh-brand'
/** Identifier scoped to one native request or socket. */
export type NativeTransferId = Branded<'AgentWorkNativeTransferId'>
type Payload = { id: NativeTransferId } & Record<string, string | number | string[] | Record<string, string>>
export type NativeEvent = { type: 'head'; status: number; headers: Record<string, string> }
  | { type: 'data'; data: string } | { type: 'end' }
  | { type: 'open'; protocol: string }
  | { type: 'message'; text?: string; data?: string }
  | { type: 'close'; code: number; reason: string; clean: boolean }

/** Capacitor's callback error contains only a stable native failure code. */
export interface NativeError { message: string }

/** Methods exported by the registered AgentWork Capacitor plugin. */
export interface NativeAgentWork {
  state(): Promise<{ serverUrl: string }>
  connect(options: { serverUrl: string; token: string }): Promise<void>
  localBootstrap(): Promise<{ injections: Record<string, unknown>[] }>
  bootstrap(): Promise<{ injections: Record<string, unknown>[]; streamBaseUrl: string }>
  failed(options: { message: string }): Promise<void>
  request(options: Payload, callback: (event: NativeEvent | null, error?: NativeError) => void): string
  cancel(options: { id: NativeTransferId }): Promise<void>
  socket(options: Payload, callback: (event: NativeEvent | null, error?: NativeError) => void): string
  send(options: Payload): Promise<void>
  close(options: Payload): Promise<void>
}

type NativeWire = {
  [Method in keyof NativeAgentWork]: NativeAgentWork[Method] extends (options: infer Options, ...rest: infer Rest) => infer Result
    ? (options: Options & { nonce: string }, ...rest: Rest) => Result : never
}

/**
 * Resolve the plugin from the local main frame; Wiki iframes share its authority.
 * @returns Registered native plugin, or an error when the carrier is unavailable.
 */
export function nativeAgentWork(): NativeAgentWork {
  const host = window.top as Window & {
    __AGENTWORK_NATIVE_NONCE__?: string
    Capacitor?: { Plugins?: { AgentWork?: NativeWire } }
  }
  const plugin = host.Capacitor?.Plugins?.AgentWork
  const nonce = host.__AGENTWORK_NATIVE_NONCE__
  if (plugin === undefined || nonce === undefined) throw new Error('native_bridge')
  return {
    state: () => plugin.state({ nonce }), connect: options => plugin.connect({ ...options, nonce }),
    localBootstrap: () => plugin.localBootstrap({ nonce }),
    bootstrap: () => plugin.bootstrap({ nonce }), failed: options => plugin.failed({ ...options, nonce }),
    request: (options, callback) => plugin.request({ ...options, nonce }, callback),
    cancel: options => plugin.cancel({ ...options, nonce }),
    socket: (options, callback) => plugin.socket({ ...options, nonce }, callback), send: options => plugin.send({ ...options, nonce }),
    close: options => plugin.close({ ...options, nonce }),
  }
}

/** Decode a native transfer. @param value - Base64 data. @returns Transfer bytes. */
export function decode(value: string): Uint8Array<ArrayBuffer> {
  return Uint8Array.from(atob(value), char => char.charCodeAt(0))
}
