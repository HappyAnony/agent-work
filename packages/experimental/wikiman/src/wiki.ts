/** Projects portable WikiMan pages and committed documents into bounded knowledge queries. */
import { createHash, randomUUID } from 'node:crypto'
import { z } from 'zod'
import { WikiError, type WikiKernel } from './kernel.ts'
import type { WikiCreatePage, WikiDocument, WikiDocumentKey, WikiEditor, WikiEditorView, WikiLink, WikiListing, WikiPage, WikiPageChange, WikiPageEditor } from './types.ts'

const pageId = z.string().regex(/^\d{19}$/)
const nativeId = z.string().regex(/^\d{14}-[a-z0-9]{7}$/)
const documentKey = z.object({ pageId, boxId: nativeId, docId: nativeId })
const pageSchema = z.strictObject({
  schemaVersion: z.literal(1), rank: z.string(), revision: z.int().positive(),
  createdAt: z.string(), updatedAt: z.string(), ext: z.record(z.string(), z.unknown()),
  pageId, parentId: pageId.nullable(), name: z.string(), type: z.enum(['wiki', 'docx', 'table']),
  entryDocument: z.object({ boxId: nativeId, docId: nativeId }).nullable(),
  deletedAt: z.string().nullable(),
})
const sourceHash = z.string().regex(/^[a-f0-9]{64}$/)
const pageViewSchema = z.object({ page: pageSchema, hash: sourceHash, entryHash: sourceHash.optional() })
const pagesSchema = z.object({
  pages: z.array(pageViewSchema).default([]),
  treeSourceDigest: sourceHash,
  nextCursor: z.string().optional(),
})
const itemSchema = z.object({ boxId: nativeId, docId: nativeId, title: z.string().optional(), hash: z.string() })
const itemsSchema = z.object({ items: z.array(itemSchema).default([]), nextCursor: z.string().optional() })
const textSchema = z.object({ text: z.string().default(''), hash: z.string() })
const documentStructureSchema = z.object({ hash: z.string(), ast: z.object({
  Type: z.literal('NodeDocument'), Children: z.array(z.unknown()).optional(),
}) })
const viewSchema = z.object({
  storeId: z.string(), pageId, sessionId: z.string(), viewId: z.string(), ticket: z.string(),
  mode: z.enum(['wikiDesktop', 'docxSingle']),
})
const createPageSchema = z.object({
  operationId: z.uuid(),
  createdAt: z.iso.datetime(),
  name: z.string().trim().min(1).max(256),
  type: z.enum(['wiki', 'docx', 'table']),
  parentId: pageId.nullable(),
})
const changeIdentity = { pageId, hash: sourceHash }
const pageChangeSchema = z.discriminatedUnion('kind', [
  z.object({ ...changeIdentity, kind: z.literal('rename'), name: z.string().trim().min(1).max(256) }),
  z.object({ ...changeIdentity, kind: z.literal('move'), parentId: pageId.nullable(), rank: z.string().trim().min(1).max(128) }),
  z.object({ ...changeIdentity, kind: z.literal('trash') }),
  z.object({ ...changeIdentity, kind: z.literal('restore') }),
])

function pageRow({ page: { pageId, parentId, name, type, rank, deletedAt }, hash }: z.infer<typeof pageViewSchema>): WikiPage {
  return { pageId, parentId, name, type, rank, deletedAt, hash }
}

const committedSchema = z.object({ receipt: z.object({ sourceCommitted: z.literal(true) }) })
const closedSchema = z.object({ closed: z.literal(true) })

/** Browser and tool operations over the same committed WikiMan source. */
export class Wiki {
  private readonly editors = new Map<string, string>()
  private openingViews = 0

  constructor(
    private readonly kernel: WikiKernel,
    private readonly pageSize: number,
    private readonly maxReferenceDocuments: number,
    private readonly legacyStoreIds: readonly string[] = [],
  ) {}

  /**
   * List active pages using the portable tree cursor.
   * @param cursor - previous page-list continuation, or an empty string.
   * @param signal - caller cancellation.
   * @param query - optional page title filter.
   * @returns page metadata and continuation.
   */
  async pages(cursor: string, signal: AbortSignal, query = ''): Promise<WikiListing<WikiPage>> {
    const result = await this.kernel.call('pages', { action: 'list', limit: this.pageSize, cursor, query }, pagesSchema, signal)
    return { items: result.pages.map(pageRow), nextCursor: result.nextCursor ?? '' }
  }

  /**
   * List native documents in a page.
   * @param id - page identifier.
   * @param cursor - document-list continuation, or an empty string.
   * @param signal - caller cancellation.
   * @returns document locations and continuation.
   */
  async documents(id: string, cursor: string, signal: AbortSignal): Promise<WikiListing<WikiDocument>> {
    return await this.findDocuments(id, 'document.list', cursor, '', this.pageSize, signal)
  }

  /**
   * Search a bounded page of the tree with the kernel's committed-content search.
   * @param query - literal text to find.
   * @param cursor - previous page-tree continuation, or an empty string.
   * @param maxHits - maximum number of returned document references.
   * @param signal - caller cancellation.
   * @returns matching documents and an indication that the scan was bounded.
   */
  async search(
    query: string, cursor: string, maxHits: number, signal: AbortSignal,
  ): Promise<{ documents: WikiDocument[]; nextCursor: string; truncated: boolean }> {
    const pages = await this.pages(cursor, signal)
    const documents: WikiDocument[] = []
    let truncated = false
    for (const page of pages.items) {
      if (page.type === 'table') continue
      const left = maxHits - documents.length
      if (left === 0) { truncated = true; break }
      const found = await this.findDocuments(page.pageId, 'search', '', query, left, signal)
      documents.push(...found.items)
      truncated ||= found.nextCursor !== ''
    }
    return { documents, nextCursor: pages.nextCursor, truncated }
  }

  /**
   * Read a document's text and hash from one committed source read.
   * @param key - native document location.
   * @param signal - caller cancellation.
   * @returns text and its committed version hash.
   */
  async text(key: WikiDocumentKey, signal: AbortSignal): Promise<{ text: string; hash: string; storeId: string }> {
    const { storeId } = this.kernel.identity()
    return { ...await this.kernel.call('content', { ...documentKey.parse(key), action: 'document.get', format: 'text' }, textSchema, signal), storeId }
  }

  /**
   * Open a scoped editor lease, initializing one paragraph if a single document has no body.
   * @param document - document row or historical reference.
   * @param signal - caller cancellation.
   * @returns capability and current committed document coordinates.
   */
  async open(document: WikiDocument, signal: AbortSignal): Promise<WikiEditor> {
    const { pageId: id } = documentKey.parse(document)
    const { storeId } = this.kernel.identity()
    if (document.storeId !== storeId && !this.legacyStoreIds.includes(document.storeId)) throw new WikiError('source_changed')
    const { page } = await this.page(id, signal)
    if (page.type === 'docx' && (page.entryDocument?.boxId !== document.boxId || page.entryDocument.docId !== document.docId)) {
      throw new WikiError('document_not_in_page')
    }
    const hash = page.type === 'docx' ? await this.prepareSingleDocument(document, signal) : (await this.text(document, signal)).hash
    return { ...await this.lease(id, page.type, signal), document: { ...document, hash, storeId } }
  }

  /**
   * Open a Wiki file tree or a single document, initializing its body when empty.
   * @param id - selected WikiMan page.
   * @param signal - caller cancellation.
   * @returns the scoped native workspace view.
   */
  async openPage(id: string, signal: AbortSignal): Promise<WikiPageEditor> {
    const { page, entryHash } = await this.page(id, signal)
    const { storeId } = this.kernel.identity()
    let document = page.type === 'docx' && page.entryDocument && entryHash
      ? { ...page.entryDocument, pageId: id, title: page.name, hash: entryHash, storeId }
      : null
    if (page.type === 'docx' && !document) throw new WikiError('document_not_in_page')
    if (document) document = { ...document, hash: await this.prepareSingleDocument(document, signal) }
    return { ...await this.lease(id, page.type, signal), document }
  }

  /**
   * Resolve native document-tree navigation and block links across active Wiki pages.
   * @param id - WikiMan page which issued the navigation.
   * @param block - native document or block identifier.
   * @param signal - caller cancellation.
   * @returns the original document and block, or null when outside the supported scan.
   */
  async resolveLink(id: string, block: string, signal: AbortSignal): Promise<WikiLink | null> {
    nativeId.parse(block)
    const pages = (await this.maintenancePages(signal)).filter(page => page.type !== 'table' && page.deletedAt === null)
    if (!pages.some(page => page.pageId === id)) throw new WikiError('page_unavailable')
    let visited = 0
    let found: WikiLink | null = null
    for (const page of pages) {
      let cursor = ''
      const cursors = new Set<string>()
      do {
        const batch = await this.documents(page.pageId, cursor, signal)
        for (const candidate of batch.items) {
          if (++visited > this.maxReferenceDocuments) throw new WikiError('reference_lookup_limit')
          try {
            const { hash } = await this.kernel.call('content', {
              ...documentKey.parse(candidate), action: 'block.get', blockId: block, format: 'text',
            }, textSchema, signal)
            if (found) throw new WikiError('ambiguous_block_reference')
            found = { document: { ...candidate, hash }, blockId: block, mode: page.type === 'docx' ? 'docxSingle' : 'wikiDesktop' }
          } catch (error) {
            if (!(error instanceof WikiError) || error.code !== 'not_found') throw error
          }
        }
        cursor = batch.nextCursor
        if (cursor && cursors.has(cursor)) throw new WikiError('invalid_kernel_response')
        cursors.add(cursor)
      } while (cursor)
    }
    return found
  }

  /**
   * Create a Wiki container or a Docx page with an atomic native entry document.
   * @param request - stable operation identity, time, page type and location.
   * @param signal - caller cancellation.
   * @returns the committed page.
   */
  async create(request: WikiCreatePage, signal: AbortSignal): Promise<WikiPage> {
    const { name, type, parentId, operationId, createdAt } = createPageSchema.parse(request)
    const timestamp = Date.parse(createdAt)
    if (!Number.isSafeInteger(timestamp) || timestamp < 1_000_000_000_000 || timestamp >= 10_000_000_000_000) {
      throw new WikiError('invalid_page_timestamp')
    }
    const digest = createHash('sha256').update(operationId).digest()
    const suffix = digest.readUInt32BE(0) % 1_000_000
    const id = `${timestamp}${String(suffix).padStart(6, '0')}`
    const nativePrefix = new Date(timestamp).toISOString().replace(/[-:T]/g, '').slice(0, 14)
    const hex = digest.toString('hex')
    const entryDocument = type === 'docx' ? { boxId: `${nativePrefix}-${hex.slice(0, 7)}`, docId: `${nativePrefix}-${hex.slice(7, 14)}` } : null
    const record = {
      schemaVersion: 1, pageId: id, parentId, name, rank: `new-${id}`, type, revision: 1,
      createdAt, updatedAt: createdAt, deletedAt: null, entryDocument, ext: {},
    }
    const update = { page: record, expectedHash: '' }
    if (entryDocument) {
      const { boxId, docId } = entryDocument
      await this.kernel.call('content', {
        action: 'document.put', operationId, pageId: id, boxId, docId, notebookName: name, pageUpdate: update,
        ast: {
          ID: docId, Type: 'NodeDocument', Spec: '1', Properties: { id: docId, title: name, type: 'doc' },
          Children: [{ ID: `${nativePrefix}-${hex.slice(14, 21)}`, Type: 'NodeParagraph',
            Properties: { id: `${nativePrefix}-${hex.slice(14, 21)}`, updated: nativePrefix } }],
        },
      }, committedSchema, signal)
    } else {
      await this.kernel.call('pages', { action: 'apply', operationId, updates: [update] }, committedSchema, signal)
    }
    return pageRow(await this.page(id, signal))
  }

  /**
   * Read live and recycled pages for the maintenance dialog.
   * @param signal - caller cancellation.
   * @returns all page records in one tree generation.
   */
  async maintenancePages(signal: AbortSignal): Promise<readonly WikiPage[]> {
    const { views } = await this.completeTree(signal)
    return views.map(pageRow)
  }

  /**
   * Apply one revision-checked page operation using the portable kernel's atomic writer.
   * @param request - page revision and requested change.
   * @param signal - caller cancellation.
   * @returns completion of the committed transaction.
   */
  async change(request: WikiPageChange, signal: AbortSignal): Promise<void> {
    if (this.hasEditors()) throw new WikiError('editor_busy')
    const change = pageChangeSchema.parse(request)
    const { views, digest } = await this.completeTree(signal)
    const original = views.find(view => view.page.pageId === change.pageId)
    if (!original || original.hash !== change.hash) throw new WikiError('source_conflict')
    const restoring = change.kind === 'restore'
    if (restoring !== (original.page.deletedAt !== null)) throw new WikiError('page_unavailable')

    const timestamp = new Date()
    if (change.kind === 'trash') {
      const cohorts = new Set(views.map(view => view.page.deletedAt))
      while (cohorts.has(timestamp.toISOString())) timestamp.setTime(timestamp.getTime() + 1)
    }
    const now = timestamp.toISOString()
    const updates = []
    if (change.kind === 'rename') {
      if (change.name === original.page.name) return
      updates.push(this.update(original, now, { name: change.name }))
    } else if (change.kind === 'move') {
      if (change.parentId === original.page.parentId && change.rank === original.page.rank) return
      if (change.parentId !== null && !views.some(view => view.page.pageId === change.parentId && view.page.deletedAt === null)) {
        throw new WikiError('page_unavailable')
      }
      updates.push(this.update(original, now, { parentId: change.parentId, rank: change.rank }))
    } else {
      const children = new Map<string, typeof views>()
      for (const view of views) {
        if (view.page.parentId === null) continue
        const group = children.get(view.page.parentId) ?? []
        group.push(view)
        children.set(view.page.parentId, group)
      }
      const pending = [original]
      const cohort = original.page.deletedAt
      for (let view = pending.pop(); view; view = pending.pop()) {
        if (view.page.deletedAt === cohort) updates.push(this.update(view, now, { deletedAt: restoring ? null : now }))
        pending.push(...children.get(view.page.pageId) ?? [])
      }
    }
    // The kernel applies at most one thousand records atomically.
    if (updates.length > 1000) throw new WikiError('page_operation_limit')
    await this.kernel.call('pages', { action: 'apply', operationId: randomUUID(), expectedTreeDigest: digest, updates }, committedSchema, signal)
  }

  private update(view: z.infer<typeof pageViewSchema>, now: string, fields: Partial<z.infer<typeof pageSchema>>) {
    const { page, hash, entryHash } = view
    return {
      page: { ...page, ...fields, revision: page.revision + 1, updatedAt: now },
      expectedHash: hash,
      ...(entryHash && { entryExpectedHash: entryHash }),
    }
  }

  private async completeTree(signal: AbortSignal) {
    const views: z.infer<typeof pageViewSchema>[] = []
    const visited = new Set<string>()
    let cursor = ''
    let digest = ''
    do {
      const batch = await this.kernel.call('pages', { action: 'list', includeDeleted: true, limit: 1000, cursor }, pagesSchema, signal)
      if (digest && batch.treeSourceDigest !== digest) throw new WikiError('source_conflict')
      views.push(...batch.pages)
      digest = batch.treeSourceDigest
      cursor = batch.nextCursor ?? ''
      if (visited.has(cursor)) throw new WikiError('invalid_kernel_response')
      visited.add(cursor)
    } while (cursor)
    return { views, digest }
  }

  // Opening an editor may initialize an empty document. Knowledge reads remain read-only.
  private async prepareSingleDocument(key: WikiDocumentKey, signal: AbortSignal): Promise<string> {
    const result = await this.kernel.call('content', {
      ...documentKey.parse(key), action: 'document.get', format: 'ast',
    }, documentStructureSchema, signal)
    if (result.ast.Children && result.ast.Children.length > 0) return result.hash
    const operationId = randomUUID()
    const prefix = new Date().toISOString().replace(/[-:T]/g, '').slice(0, 14)
    const paragraphId = `${prefix}-${createHash('sha256').update(operationId).digest('hex').slice(0, 7)}`
    await this.kernel.call('content', {
      ...documentKey.parse(key), action: 'block.create', operationId, expectedHash: result.hash, parentId: key.docId, blockId: paragraphId,
      ast: { ID: paragraphId, Type: 'NodeParagraph', Properties: { id: paragraphId, updated: prefix } },
    }, committedSchema, signal)
    return (await this.text(key, signal)).hash
  }

  private async page(id: string, signal: AbortSignal) {
    const info = await this.kernel.call('pages', { action: 'get', pageId: pageId.parse(id) }, pagesSchema, signal)
    const view = info.pages[0]
    if (!view || view.page.deletedAt !== null) throw new WikiError('page_unavailable')
    return view
  }

  private async lease(id: string, type: WikiPage['type'], signal: AbortSignal): Promise<WikiEditorView> {
    const { baseUrl, storeId, sessionId } = this.kernel.identity()
    if (type === 'table') throw new WikiError('page_unavailable')
    const mode = type === 'docx' ? 'docxSingle' : 'wikiDesktop'
    signal.throwIfAborted()
    this.openingViews++
    let view: z.infer<typeof viewSchema>
    try {
      view = await this.kernel.call('editor', { action: 'open', pageId: id, mode }, viewSchema, new AbortController().signal)
      this.editors.set(view.viewId, view.ticket)
    } finally {
      this.openingViews--
    }
    if (signal.aborted || view.storeId !== storeId || view.sessionId !== sessionId || view.pageId !== id || view.mode !== mode) {
      await this.close(view.viewId)
      signal.throwIfAborted()
      throw new WikiError('invalid_editor_identity')
    }
    return { baseUrl, view }
  }

  /**
   * Check whether the source is still leased by a browser.
   * @returns whether an editor view is open.
   */
  hasEditors(): boolean {
    return this.editors.size !== 0 || this.openingViews !== 0
  }

  /**
   * Close a previously issued editor lease after the browser has drained it.
   * @param viewId - kernel-issued editor identity.
   * @returns completion of capability withdrawal.
   */
  async close(viewId: string): Promise<void> {
    const ticket = this.editors.get(viewId)
    if (!ticket) return
    try {
      await this.kernel.call('editor', { action: 'close', ticket }, closedSchema, new AbortController().signal)
    } catch (error) {
      if (!(error instanceof WikiError) || error.code !== 'stale_session') throw error
    }
    this.editors.delete(viewId)
  }

  private async findDocuments(id: string, action: 'document.list' | 'search', cursor: string, query: string, limit: number, signal: AbortSignal) {
    const { storeId } = this.kernel.identity()
    const result = await this.kernel.call('content', {
      action, pageId: pageId.parse(id), cursor, limit, ...(query ? { query } : {}),
    }, itemsSchema, signal)
    return {
      items: result.items.map(item => ({ ...item, title: (item.title || item.docId).slice(0, 256), pageId: id, storeId })),
      nextCursor: result.nextCursor ?? '',
    }
  }
}
