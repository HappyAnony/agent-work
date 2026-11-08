/** Device-owned server selection for a packaged client; credentials remain in its native carrier. */

/** Failure categories that the connection settings can explain without private diagnostics. */
export type ServerFailure = 'address' | 'authentication' | 'certificate' | 'version' | 'network' | 'unsaved' | 'storage'

/** Authenticated public startup data, after the native carrier verifies packaged resources. */
export interface PackagedServerBootstrap {
  readonly globals: Readonly<Record<string, unknown>>
  readonly graph: unknown
}

/** Local connection state, available before a Host connection exists. */
export interface PackagedServerSnapshot {
  readonly address: string
  readonly phase: 'unconfigured' | 'connecting' | 'connected' | 'unavailable'
  readonly failure?: ServerFailure
  readonly configurationRequested: boolean
  readonly bootstrap?: PackagedServerBootstrap
}

/** Native operations used by the device-local server selection owner. */
export interface PackagedServerDriver {
  /** Read the saved selection; an empty selection uses the deployment's default address. */
  readAddress(): Promise<string>
  /** Authenticate and verify the selected Host; a blank token reuses its saved native credential. */
  connect(address: string, token: string): Promise<PackagedServerBootstrap>
}

/** A failed native connection; the category is independent of its display message. */
export class PackagedServerError extends Error {
  /** @param category - Actionable failure category. */
  constructor(readonly category: ServerFailure) { super(category) }
}

/** Device-local service settings and their observable state. */
export interface PackagedServerConnection {
  /** @returns Current selection and connection outcome. */
  getSnapshot(): PackagedServerSnapshot
  /** @param listener - State-change callback. @returns Unsubscribe function. */
  subscribe(listener: () => void): () => void
  /** @param address - Root server URL. @param token - Optional first-connection credential. @returns Connection settlement. */
  configure(address: string, token: string): Promise<void>
  /** @returns Reconnection settlement for the selected server. */
  retry(this: void): Promise<void>
  /** Request one configuration prompt without changing the current selection. */
  requestConfiguration(this: void): void
  /** Dismiss the current configuration prompt. */
  dismissConfiguration(this: void): void
}

/** One packaged window's selected server; concurrent attempts share one settlement. */
export class PackagedServer implements PackagedServerConnection {
  private snapshot: PackagedServerSnapshot
  private readonly listeners = new Set<() => void>()
  private pending: Promise<void> | undefined
  private stopped = false
  private selection = 0

  /**
   * @param driver - Native credential and network owner.
   * @param defaultAddress - Deployment default, used only without a saved selection.
   */
  constructor(private readonly driver: PackagedServerDriver, private readonly defaultAddress: string) {
    this.snapshot = { address: defaultAddress, phase: defaultAddress ? 'connecting' : 'unconfigured', configurationRequested: false }
  }

  /** @returns Current device-local selection. */
  getSnapshot = (): PackagedServerSnapshot => this.snapshot

  /** @param listener - State-change callback. @returns Unsubscribe function. */
  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener)
    return () => { this.listeners.delete(listener) }
  }

  /** Read the selected server and attempt connection without opening a setup screen. @returns Initial attempt settlement. */
  async start(): Promise<void> {
    const selection = this.selection
    try {
      const saved = await this.driver.readAddress()
      if (!this.stopped && selection === this.selection) await this.configure(saved || this.defaultAddress, '')
    } catch (error) { if (selection === this.selection) this.fail(error) }
  }

  /** @param address - Selected root URL. @param token - Native authentication input. @returns Attempt settlement. */
  configure(address: string, token: string): Promise<void> {
    if (this.pending) return this.pending
    if (this.stopped) return Promise.resolve()
    this.selection++
    const selected = address.trim()
    if (!selected) {
      this.publish({ address: '', phase: 'unconfigured', configurationRequested: this.snapshot.configurationRequested })
      return Promise.resolve()
    }
    const pending = Promise.resolve().then(() => this.driver.connect(selected, token)).then((bootstrap) => {
      this.publish({ address: selected, phase: 'connected', configurationRequested: false, bootstrap })
    }, (error: unknown) => { this.fail(error) }).finally(() => { if (this.pending === pending) this.pending = undefined })
    this.pending = pending
    this.publish({ address: selected, phase: 'connecting', configurationRequested: this.snapshot.configurationRequested })
    return pending
  }

  /** @returns Reconnection settlement; never substitutes another server after failure. */
  retry = (): Promise<void> => this.configure(this.snapshot.address, '')
  /** Request one connection prompt. */
  requestConfiguration = (): void => { this.publish({ ...this.snapshot, configurationRequested: true }) }
  /** Dismiss the current connection prompt. */
  dismissConfiguration = (): void => { this.publish({ ...this.snapshot, configurationRequested: false }) }
  /** Stop notifications and ignore late native completions. */
  dispose(): void { this.stopped = true; this.listeners.clear() }

  private fail(error: unknown): void {
    this.publish({ address: this.snapshot.address, phase: 'unavailable', configurationRequested: this.snapshot.configurationRequested,
      failure: error instanceof PackagedServerError ? error.category : 'network' })
  }
  private publish(snapshot: PackagedServerSnapshot): void {
    if (this.stopped) return
    this.snapshot = snapshot
    for (const listener of [...this.listeners]) {
      try { listener() }
      catch (error) { console.error('[packaged-server] state listener failed', error) }
    }
  }
}
