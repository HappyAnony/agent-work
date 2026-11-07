/** Opt-in WikiMan Host, editor Remote and read-only model tools. */
import { realpath } from 'node:fs/promises'
import { isAbsolute } from 'node:path'
import { Context, Service } from '@deepseek-ai/cordis'
import z from '@deepseek-ai/schemastery'
import type { Agent } from '@deepseek-ai/dsh-agent'
import type {} from '@deepseek-ai/dsh-subprocess'
import { defineTool } from '@deepseek-ai/dsh-tools'
import { Remote, RemoteError, TypertRemoteService } from '@deepseek-ai/dsh-typert-protocol'
import { importLegacy } from './legacy-import.ts'
import { NativeValidation } from './native-validation.ts'
import { PortableReplica, type ReplicaConfig } from './replica.ts'
import { WikiError, WikiKernel, type KernelConfig } from './kernel.ts'
import type { WikiCreatePage, WikiDocument, WikiEditor, WikiImportConfirmation, WikiImportReport, WikiLegacyImport, WikiLink, WikiListing, WikiPage, WikiPageChange, WikiPageEditor, WikiRuntime, WikiSiyuanView, WikiSyncResult } from './types.ts'
import { SiyuanWiki } from './siyuan-wiki.ts'
import { Wiki } from './wiki.ts'

/** WikiMan runtime, local library and the workspace allowed to query it. */
export interface Config extends KernelConfig, ReplicaConfig {
  /** Select the original SiYuan workspace or the portable WikiMan source and application. */
  readonly runtime: WikiRuntime
  /** Historical native store IDs belonging to this imported library; citations resolve against the current store. */
  readonly legacyCitationStoreIds: string[]
  /** Time budget for one legacy import or portable sync operation in milliseconds. */
  readonly sourceOperationTimeoutMs: number
  /** Absolute directory recorded when a Session starts in this workspace. */
  readonly workspaceRoot: string
  /** Maximum pages or document rows visited per list call. */
  readonly pageSize: number
  /** Maximum document references returned by one search. */
  readonly maxSearchHits: number
  /** Maximum UTF-16 text length returned by one document read. */
  readonly maxReadChars: number
}

declare module '@deepseek-ai/dsh-typert-protocol' {
  interface RemoteErrorDetailsMap {
    /** Browser invocation belongs to a non-owner peer. */
    'wikiman/forbidden': Record<string, never>
    /** Portable source or editor operation failed. */
    'wikiman/failed': { reason: string }
  }
}

type LiveSource = { readonly root: string; readonly kernel: WikiKernel; readonly wiki: Wiki }

const documentProperties = {
  pageId: { type: 'string', required: true },
  boxId: { type: 'string', required: true },
  docId: { type: 'string', required: true },
  storeId: { type: 'string', required: true },
  title: { type: 'string', required: true },
  hash: { type: 'string', required: true },
} as const
const documentSchema = {
  type: 'object',
  properties: documentProperties,
  additionalProperties: false,
} as const

/** Hosts one WikiMan library for the local DSH owner and one configured workspace. */
export default class WikiMan extends TypertRemoteService {
  static inject = ['typert', 'subprocess', 'tools']

  static Config: z<Config> = z.object({
    runtime: z.union(['portable', 'siyuan'] as const).default('portable'),
    legacyCitationStoreIds: z.array(z.string().pattern(/^[a-f0-9]{64}$/u)).default([]),
    kernelPath: z.string().required(),
    appDir: z.string().required(),
    sourceRoot: z.string().required(),
    workspaceRoot: z.string().required(),
    syncDirectory: z.string(),
    maxSourceBytes: z.number().step(1).min(1024).max(Number.MAX_SAFE_INTEGER).default(512 * 1024 * 1024),
    maxSourceFiles: z.number().step(1).min(1).max(1_000_000).default(20_000),
    sourceOperationTimeoutMs: z.number().step(1).min(1).max(3_600_000).default(120_000),
    startupTimeoutMs: z.number().step(1).min(1).max(120_000).default(15_000),
    shutdownGraceMs: z.number().step(1).min(1).max(120_000).default(3000),
    requestTimeoutMs: z.number().step(1).min(1).max(120_000).default(15_000),
    maxResponseBytes: z.number().step(1).min(1024).max(32 * 1024 * 1024).default(8 * 1024 * 1024),
    pageSize: z.number().step(1).min(1).max(100).default(24),
    maxSearchHits: z.number().step(1).min(1).max(32).default(6),
    maxReadChars: z.number().step(1).min(256).max(32_000).default(6000),
  })

  private readonly lifetime = new AbortController()
  private readonly viewPeers = new Map<string, () => void>()
  private readonly closingViews = new Set<Promise<void>>()
  private readonly validation: NativeValidation
  private replica?: PortableReplica
  private source: LiveSource | undefined
  private siyuan?: SiyuanWiki
  private sourceOperation: Promise<unknown> | undefined
  private liveOperations = 0
  private workspaceRoot?: string

  constructor(ctx: Context, private readonly config: Config) {
    super(ctx, 'wikiman', { namespace: 'wikiman' })
    this.validation = new NativeValidation(ctx.subprocess, config)
    ctx.effect(() => async () => {
      this.lifetime.abort()
      await this.sourceOperation?.catch((_error: unknown) => { /* The caller receives the operation error. */ })
      for (const detach of this.viewPeers.values()) detach()
      this.viewPeers.clear()
      await Promise.allSettled(this.closingViews)
      await this.siyuan?.dispose()
      await this.stopSource()
    }, 'wikiman: stop kernel')
  }

  protected async [Service.init](): Promise<void> {
    if (!isAbsolute(this.config.workspaceRoot)) throw new WikiError('absolute_workspace_required')
    if (!isAbsolute(this.config.sourceRoot) || this.config.syncDirectory !== undefined && !isAbsolute(this.config.syncDirectory)) {
      throw new WikiError('absolute_paths_required')
    }
    this.workspaceRoot = await realpath(this.config.workspaceRoot)
    const sourceRoot = await realpath(this.config.sourceRoot)
    if (this.config.runtime === 'siyuan') {
      if (this.config.syncDirectory) throw new WikiError('portable_sync_required')
      if (![this.config.kernelPath, this.config.appDir].every(isAbsolute)) throw new WikiError('absolute_paths_required')
      const [kernelPath, appDir] = await Promise.all([realpath(this.config.kernelPath), realpath(this.config.appDir)])
      const wiki = await SiyuanWiki.load(this.ctx.subprocess, { ...this.config, sourceRoot, kernelPath, appDir })
      if (this.lifetime.signal.aborted) {
        await wiki.dispose()
        this.lifetime.signal.throwIfAborted()
      }
      this.siyuan = wiki
    } else {
      this.replica = new PortableReplica(sourceRoot, this.config, this.validation)
      this.source = await this.openSource(await this.replica.activeRoot())
    }
    this.registerTools()
  }

  private async openSource(root: string): Promise<LiveSource> {
    this.lifetime.signal.throwIfAborted()
    const kernel = new WikiKernel(this.ctx.subprocess, { ...this.config, sourceRoot: root })
    try {
      await kernel.start()
      this.lifetime.signal.throwIfAborted()
    } catch (error) {
      await kernel.dispose()
      throw error
    }
    return { root, kernel, wiki: new Wiki(kernel, this.config.pageSize, this.config.maxSourceFiles, this.config.legacyCitationStoreIds) }
  }

  private async stopSource(): Promise<void> {
    const source = this.source
    if (!source) return
    await source.kernel.dispose()
    this.source = undefined
  }

  private get wiki(): Wiki | SiyuanWiki {
    return this.siyuan ?? this.portable
  }

  private get portable(): Wiki {
    if (!this.source) throw new WikiError('kernel_unavailable')
    return this.source.wiki
  }

  /**
   * Read the configured application and storage format.
   * @returns selected WikiMan runtime.
   */
  @Remote
  runtime(): Promise<WikiRuntime> {
    return Promise.resolve(this.config.runtime)
  }

  /**
   * Issue the original SiYuan desktop view for a page or a cited document.
   * @param pageId - original WikiMan page.
   * @param parent - DSH browser origin.
   * @param reference - optional document citation.
   * @param locale - active DSH interface language for the native workspace.
   * @param signal - remote cancellation before the view is issued.
   * @returns the native view and one-use browser login URL.
   */
  @Remote
  async openSiyuan(
    pageId: string, parent: string, reference: WikiDocument | null, locale: string, signal: AbortSignal,
  ): Promise<WikiSiyuanView> {
    return await this.owner(async () => {
      const wiki = this.siyuan
      const peer = this.ctx.invocation?.peer
      if (!wiki || !peer) throw new WikiError('siyuan_mode_required')
      const lifetime = AbortSignal.any([signal, peer.signal, this.lifetime.signal])
      const view = await wiki.open(pageId, parent, reference, locale, lifetime)
      if (lifetime.aborted) {
        await wiki.close(view.viewId)
        lifetime.throwIfAborted()
      }
      return view
    })
  }

  /**
   * List active pages for the local owner.
   * @param cursor - previous list continuation, or an empty string.
   * @param signal - remote cancellation.
   * @returns one batch of visible pages.
   */
  @Remote
  async pages(cursor: string, signal: AbortSignal): Promise<WikiListing<WikiPage>> {
    return await this.owner(() => this.wiki.pages(cursor, signal))
  }

  /**
   * Search page names in the portable source.
   * @param query - title text.
   * @param cursor - previous continuation for that query.
   * @param signal - remote cancellation.
   * @returns a batch of matching pages.
   */
  @Remote
  async searchPages(query: string, cursor: string, signal: AbortSignal): Promise<WikiListing<WikiPage>> {
    return await this.owner(() => this.wiki.pages(cursor, signal, query))
  }

  /**
   * Create a Wiki, Docx or table page from the local owner's workbench.
   * @param request - stable creation request for the portable kernel.
   * @param signal - remote cancellation.
   * @returns the committed page.
   */
  @Remote
  async createPage(request: WikiCreatePage, signal: AbortSignal): Promise<WikiPage> {
    return await this.owner(() => this.wiki.create(request, signal))
  }

  /**
   * Read the live and recycled pages for page maintenance.
   * @param signal - remote cancellation.
   * @returns page records from one tree generation.
   */
  @Remote
  async maintenancePages(signal: AbortSignal): Promise<readonly WikiPage[]> {
    return await this.owner(() => this.wiki.maintenancePages(signal))
  }

  /**
   * Rename, move, recycle or restore a page and commit the native operation.
   * @param request - revision-checked change for the current store.
   * @param signal - remote cancellation.
   * @returns completion of the kernel transaction.
   */
  @Remote
  async changePage(request: WikiPageChange, signal: AbortSignal): Promise<void> {
    await this.owner(() => this.exclusive(signal, operationSignal => this.wiki.change(request, operationSignal)), 'maintenance')
  }

  /**
   * List documents in a selected WikiMan page.
   * @param pageId - selected page identity.
   * @param cursor - previous list continuation, or an empty string.
   * @param signal - remote cancellation.
   * @returns one batch of documents.
   */
  @Remote
  async documents(pageId: string, cursor: string, signal: AbortSignal): Promise<WikiListing<WikiDocument>> {
    return await this.owner(() => this.portable.documents(pageId, cursor, signal))
  }

  /**
   * Initialize an empty single-document body and issue a scoped editor capability.
   * @param document - selected document or historical reference.
   * @param signal - remote cancellation.
   * @returns the editor capability and current document version.
   */
  @Remote
  async openEditor(document: WikiDocument, signal: AbortSignal): Promise<WikiEditor> {
    return await this.owner(async () => this.bindPortableView(await this.portable.open(document, signal)))
  }

  /**
   * Open a Wiki workspace or initialize and open its single-document entry.
   * @param pageId - selected page.
   * @param signal - remote cancellation.
   * @returns the scoped editor view.
   */
  @Remote
  async openPage(pageId: string, signal: AbortSignal): Promise<WikiPageEditor> {
    return await this.owner(async () => this.bindPortableView(await this.portable.openPage(pageId, signal)))
  }

  /**
   * Resolve a document-tree selection or block link through the native kernel.
   * @param pageId - page whose editor requested navigation.
   * @param blockId - selected document or block identity.
   * @param signal - remote cancellation.
   * @returns authorized document location, or null when unavailable.
   */
  @Remote
  async resolveLink(pageId: string, blockId: string, signal: AbortSignal): Promise<WikiLink | null> {
    return await this.owner(() => this.portable.resolveLink(pageId, blockId,
      AbortSignal.any([signal, this.lifetime.signal, AbortSignal.timeout(this.config.sourceOperationTimeoutMs)])))
  }

  /**
   * Revoke a scoped editor view after the browser has drained pending input.
   * @param viewId - identity returned by openEditor.
   * @returns completion of lease revocation.
   */
  @Remote
  async closeEditor(viewId: string): Promise<void> {
    await this.owner(async () => {
      await this.wiki.close(viewId)
      this.viewPeers.get(viewId)?.()
      this.viewPeers.delete(viewId)
    })
  }

  /**
   * Inspect or publish a validated portable copy of a closed WikiMan SQLite library.
   * @param request - source and destination directories and portable name.
   * @param confirmation - digest and identity from the preview, or null to preview.
   * @param signal - remote cancellation.
   * @returns the inventory and optional published source path.
   */
  @Remote
  async importLibrary(
    request: WikiLegacyImport, confirmation: WikiImportConfirmation | null, signal: AbortSignal,
  ): Promise<WikiImportReport> {
    return await this.owner(() => this.exclusive(signal, (operationSignal) => {
      if (this.siyuan) throw new WikiError('portable_mode_required')
      return importLegacy(request, confirmation, this.config, this.validation, operationSignal)
    }), 'maintenance')
  }

  /**
   * Synchronize the portable source and reopen the current committed generation.
   * @param signal - remote cancellation before publication.
   * @returns the direction, current generation or preserved conflict.
   */
  @Remote
  async syncLibrary(signal: AbortSignal): Promise<WikiSyncResult> {
    return await this.owner(() => this.exclusive(signal, async (operationSignal) => {
      if (!this.config.syncDirectory) throw new WikiError('sync_not_configured')
      const replica = this.replica
      const source = this.source
      if (!replica || !source) throw new WikiError('kernel_unavailable')
      const staged: { source?: LiveSource } = {}
      try {
        return await replica.sync(source.root, () => source.kernel.identity().storeId, async (candidate) => {
          staged.source = await this.openSource(candidate)
        }, operationSignal)
      } finally {
        if (staged.source) {
          try {
            if (await replica.activeRoot() === staged.source.root) {
              this.source = staged.source
              await source.kernel.dispose()
            }
          } finally {
            if (this.source !== staged.source) await staged.source.kernel.dispose()
          }
        }
      }
    }), 'maintenance')
  }

  private async bindPortableView<T extends WikiPageEditor>(editor: T): Promise<T> {
    const peer = this.ctx.invocation?.peer
    const wiki = this.portable
    if (!peer || peer.signal.aborted) {
      await wiki.close(editor.view.viewId)
      throw new WikiError('editor_peer_closed')
    }
    const id = editor.view.viewId
    const detach = () => { peer.signal.removeEventListener('abort', close) }
    const close = () => {
      detach()
      this.viewPeers.delete(id)
      const pending = wiki.close(id).catch((error: unknown) => {
        this.ctx.logger.warn('WikiMan could not revoke a disconnected editor; restart the Wiki plugin before source maintenance: %s', String(error))
      }).finally(() => { this.closingViews.delete(pending) })
      this.closingViews.add(pending)
    }
    this.viewPeers.set(id, detach)
    peer.signal.addEventListener('abort', close, { once: true })
    return editor
  }

  private async owner<T>(operation: () => T | Promise<T>, kind: 'live' | 'maintenance' = 'live'): Promise<T> {
    if (this.ctx.invocation?.peer.principal.kind !== 'owner') {
      throw new RemoteError('wikiman/forbidden', 'WikiMan requires the local owner', {})
    }
    try {
      return await (kind === 'live' ? this.live(operation) : operation())
    } catch (error) {
      const code = error instanceof WikiError ? error.code : 'request_failed'
      throw new RemoteError('wikiman/failed', 'WikiMan request failed', { reason: code }, { cause: error })
    }
  }

  private async live<T>(operation: () => T | Promise<T>): Promise<T> {
    this.lifetime.signal.throwIfAborted()
    if (this.sourceOperation) throw new WikiError('source_busy')
    this.liveOperations++
    try {
      return await operation()
    } finally {
      this.liveOperations--
    }
  }

  private async exclusive<T>(signal: AbortSignal, operation: (signal: AbortSignal) => T | Promise<T>): Promise<T> {
    this.lifetime.signal.throwIfAborted()
    if (this.wiki.hasEditors()) throw new WikiError('editor_busy')
    if (this.sourceOperation || this.liveOperations !== 0) throw new WikiError('source_busy')
    const deadline = AbortSignal.timeout(this.config.sourceOperationTimeoutMs)
    const operationSignal = AbortSignal.any([signal, this.lifetime.signal, deadline])
    const run = Promise.resolve().then(() => {
      operationSignal.throwIfAborted()
      return operation(operationSignal)
    })
    this.sourceOperation = run
    try {
      return await run
    } catch (error) {
      if (deadline.aborted) throw new WikiError('source_operation_timeout', { cause: error })
      throw error
    } finally {
      this.sourceOperation = undefined
    }
  }

  private allows(agent: Agent | undefined): boolean {
    return this.workspaceRoot !== undefined && agent?.session.header.cwd === this.workspaceRoot
  }

  private requireWorkspace(agent: Agent | undefined): void {
    if (!this.allows(agent)) throw new WikiError('workspace_forbidden')
  }

  private registerTools(): void {
    const when = ({ agent }: { agent: Agent }) => this.allows(agent)
    this.ctx.effect(() => this.ctx.tools.register(defineTool({
      name: 'wiki_search',
      description: 'Find documents in the current workspace Wiki by literal text. Search a specific phrase or identifier, then use wiki_read to verify the relevant passage.',
      parameters: {
        query: { type: 'string', required: true, description: 'A short literal phrase, keyword, or identifier.' },
        cursor: { type: 'string', description: 'Continuation returned by a previous search with the same query.' },
      },
      output: {
        schema: {
          type: 'object',
          properties: {
            documents: { type: 'array', items: documentSchema, required: true },
            nextCursor: { type: 'string', required: true },
            truncated: { type: 'boolean', required: true },
          },
          additionalProperties: false,
        },
        render: (_args, value) => [{ type: 'text', text: JSON.stringify(value) }],
      },
      async execute({ query, cursor }, exec) {
        return await search(query, cursor ?? '', exec.agent, exec.signal)
      },
    }), { when }), 'wikiman: search tool')
    const search = (query: string, cursor: string, agent: Agent | undefined, signal: AbortSignal) => {
      this.requireWorkspace(agent)
      const term = query.trim()
      if (term.length === 0 || term.length > 200 || cursor.length > 8192) throw new WikiError('invalid_search')
      return this.live(() => this.wiki.search(term, cursor, this.config.maxSearchHits, signal))
    }

    const read = async (
      pageId: string, boxId: string, docId: string, offset: number, expectedHash: string | undefined,
      agent: Agent | undefined, signal: AbortSignal,
    ) => {
      this.requireWorkspace(agent)
      if (!Number.isSafeInteger(offset) || offset < 0) throw new WikiError('invalid_offset')
      const { text, hash, storeId } = await this.live(() => this.wiki.text({ pageId, boxId, docId }, signal))
      if ((offset > 0 || expectedHash !== undefined) && expectedHash !== hash) throw new WikiError('document_version_changed')
      if (offset > text.length) throw new WikiError('invalid_offset')
      let end = Math.min(text.length, offset + this.config.maxReadChars)
      if (end < text.length && /[\uDC00-\uDFFF]/u.test(text.charAt(end))) end--
      return {
        source: { pageId, boxId, docId, title: docId, hash, storeId },
        text: text.slice(offset, end),
        nextOffset: end < text.length ? end : null,
      }
    }
    this.ctx.effect(() => this.ctx.tools.register(defineTool({
      name: 'wiki_read',
      description: 'Read committed Wiki text and cite the original document using the source from the result. Cite only claims supported by the text you read.',
      parameters: {
        pageId: { type: 'string', required: true, description: 'Page identifier from wiki_search.' },
        boxId: { type: 'string', required: true, description: 'Notebook identifier from wiki_search.' },
        docId: { type: 'string', required: true, description: 'Document identifier from wiki_search.' },
        offset: { type: 'integer', description: 'Next offset from a previous read of this document.' },
        expectedHash: { type: 'string', description: 'Hash returned by the first read, required when reading at a nonzero offset.' },
      },
      output: {
        schema: {
          type: 'object',
          properties: {
            source: { ...documentSchema, required: true },
            text: { type: 'string', required: true },
            nextOffset: { oneOf: [{ type: 'integer' }, { type: 'null' }], required: true },
          },
          additionalProperties: false,
        },
        render: (_args, value) => [{ type: 'text', text: JSON.stringify(value) }],
        presentationMeta: (_args, value) => ({ source: value.source }),
      },
      async execute({ pageId, boxId, docId, offset, expectedHash }, exec) {
        return await read(pageId, boxId, docId, offset ?? 0, expectedHash, exec.agent, exec.signal)
      },
    }), { when }), 'wikiman: read tool')
  }
}
