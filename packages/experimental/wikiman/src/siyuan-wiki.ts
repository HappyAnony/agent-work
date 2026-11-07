/** The original IronMan page database and SiYuan workspaces. */
import { createHash } from 'node:crypto'
import { mkdir } from 'node:fs/promises'
import { join } from 'node:path'
import { DatabaseSync } from 'node:sqlite'
import type { SubprocessRuntime } from '@deepseek-ai/dsh-subprocess'
import { z } from 'zod'
import { WikiError, type KernelConfig } from './kernel.ts'
import { SiyuanKernel } from './siyuan-kernel.ts'
import { directory, pageIdSchema, readRegular, type SourceLimits } from './source-files.ts'
import type { WikiCreatePage, WikiDocument, WikiDocumentKey, WikiListing, WikiPage, WikiPageChange, WikiSiyuanView } from './types.ts'

const digest = (bytes: string | Buffer) => createHash('sha256').update(bytes).digest('hex')
const nativeId = z.string().regex(/^\d{14}-[a-z0-9]{7}$/u)
const nativeFile = /^(\d{14}-[a-z0-9]{7})\.(?:docj|sy)$/u
const pageTypes = ['docx', 'wiki', 'table'] as const
const nameSchema = z.string().trim().min(1).max(256)
const recordSchema = z.object({
  pageId: pageIdSchema, parentId: pageIdSchema.nullable(), name: z.string(), rank: z.string(),
  type: z.union([z.literal(0), z.literal(1), z.literal(2)]), updated: z.number(), deletedAt: z.string().nullable(),
})
const createSchema = z.object({
  operationId: z.uuid(), createdAt: z.iso.datetime(), name: nameSchema,
  type: z.enum(pageTypes), parentId: pageIdSchema.nullable(),
})
const changeIdentity = { pageId: pageIdSchema, hash: z.string().regex(/^[a-f0-9]{64}$/u) }
const changeSchema = z.discriminatedUnion('kind', [
  z.object({ ...changeIdentity, kind: z.literal('rename'), name: nameSchema }),
  z.object({ ...changeIdentity, kind: z.literal('move'), parentId: pageIdSchema.nullable(), rank: z.string().trim().min(1).max(128) }),
  z.object({ ...changeIdentity, kind: z.literal('trash') }),
  z.object({ ...changeIdentity, kind: z.literal('restore') }),
])
const documentKey = z.object({ pageId: pageIdSchema, boxId: nativeId, docId: nativeId })
const astRoot = z.object({
  ID: nativeId, Type: z.literal('NodeDocument'),
  Properties: z.object({ title: z.string().optional() }), Children: z.array(z.unknown()).optional(),
})
const astNode = z.object({
  Data: z.string().optional(), TextMarkTextContent: z.string().optional(), Children: z.array(z.unknown()).optional(),
})
type NativeDocument = WikiDocumentKey & { readonly path: string }

function textOf(bytes: Buffer, key: WikiDocumentKey) {
  const doc = astRoot.parse(JSON.parse(bytes.toString('utf8')))
  if (doc.ID !== key.docId) throw new WikiError('document_identity_mismatch')
  const spans = [doc.Properties.title ?? doc.ID]
  const pending = doc.Children?.toReversed() ?? []
  while (pending.length) {
    const node = astNode.parse(pending.pop())
    const value = node.TextMarkTextContent ?? node.Data
    if (value) spans.push(value)
    if (node.Children) pending.push(...node.Children.toReversed())
  }
  return { title: spans[0] ?? key.docId, text: spans.join('\n'), hash: digest(bytes) }
}

function descendants(page: WikiPage, pages: readonly WikiPage[]): WikiPage[] {
  const children = new Map<string, WikiPage[]>()
  for (const item of pages) {
    if (item.parentId === null) continue
    const siblings = children.get(item.parentId) ?? []
    siblings.push(item)
    children.set(item.parentId, siblings)
  }
  const result = [page]
  const seen = new Set([page.pageId])
  for (const item of result) {
    for (const child of children.get(item.pageId) ?? []) {
      if (seen.has(child.pageId)) throw new WikiError('invalid_page_tree')
      seen.add(child.pageId)
      result.push(child)
    }
  }
  return result
}

/** Reads the original page hierarchy and launches its original editor for the selected workspace. */
export class SiyuanWiki {
  private active: { readonly kernel: SiyuanKernel; readonly view: WikiSiyuanView } | undefined
  private opening: Promise<WikiSiyuanView> | undefined
  private closing: Promise<void> | undefined
  private readonly storeId: string

  constructor(
    private readonly db: DatabaseSync, private readonly subprocess: SubprocessRuntime,
    private readonly config: KernelConfig & SourceLimits & { readonly pageSize: number },
  ) {
    this.storeId = digest('siyuan:' + config.sourceRoot)
  }

  /**
   * Open the configured copy of IronMan's original page database.
   * @param subprocess - managed local process provider.
   * @param config - native resources, original library and operation limits.
   * @returns the original WikiMan source adapter.
   */
  static async load(
    subprocess: SubprocessRuntime, config: KernelConfig & SourceLimits & { readonly pageSize: number },
  ): Promise<SiyuanWiki> {
    const path = join(config.sourceRoot, 'page.db')
    if (!await readRegular(path, Math.min(config.maxSourceBytes, 64 * 1024 * 1024))) throw new WikiError('legacy_database_missing')
    const db = new DatabaseSync(path, { allowExtension: false })
    try {
      db.exec('PRAGMA trusted_schema=OFF; PRAGMA busy_timeout=3000')
      db.prepare('SELECT page_id, parent_id, name, rank, type, update_at FROM pages LIMIT 0').all()
      db.exec('CREATE TABLE IF NOT EXISTS dsh_wiki_recycle (page_id TEXT PRIMARY KEY, deleted_at TEXT NOT NULL)')
      return new SiyuanWiki(db, subprocess, config)
    } catch (error) {
      db.close()
      throw error
    }
  }

  private tree(): WikiPage[] {
    const raw = this.db.prepare(`SELECT p.page_id AS pageId, p.parent_id AS parentId, p.name, p.rank, p.type,
      p.update_at AS updated, r.deleted_at AS deletedAt FROM pages p LEFT JOIN dsh_wiki_recycle r ON r.page_id = p.page_id
      ORDER BY p.rank, p.page_id LIMIT ?`).all(this.config.maxSourceFiles + 1)
    if (raw.length > this.config.maxSourceFiles) throw new WikiError('source_limit_exceeded')
    return raw.map((value) => {
      const record = recordSchema.parse(value)
      return {
        pageId: record.pageId, parentId: record.parentId, name: record.name, rank: record.rank, deletedAt: record.deletedAt,
        type: pageTypes[record.type], hash: digest(JSON.stringify(record)),
      }
    })
  }

  private page(id: string): WikiPage {
    const page = this.tree().find(item => item.pageId === pageIdSchema.parse(id))
    if (!page || page.deletedAt !== null) throw new WikiError('page_unavailable')
    return page
  }

  /**
   * List or search live original WikiMan pages.
   * @param cursor - numeric continuation or empty string.
   * @param signal - caller cancellation.
   * @param query - title substring.
   * @returns the requested page batch.
   */
  pages(cursor: string, signal: AbortSignal, query = ''): WikiListing<WikiPage> {
    signal.throwIfAborted()
    const offset = Number(cursor || 0)
    if (!Number.isSafeInteger(offset) || offset < 0) throw new WikiError('invalid_cursor')
    const needle = query.toLocaleLowerCase()
    const pages = this.tree().filter(page => page.deletedAt === null && page.name.toLocaleLowerCase().includes(needle))
    const end = offset + this.config.pageSize
    return { items: pages.slice(offset, end), nextCursor: end < pages.length ? String(end) : '' }
  }

  /**
   * Read the complete original page tree including DSH's recycle status.
   * @param signal - caller cancellation.
   * @returns live and recycled original page records.
   */
  maintenancePages(signal: AbortSignal): readonly WikiPage[] {
    signal.throwIfAborted()
    return this.tree()
  }

  private transaction<T>(operation: () => T): T {
    this.db.exec('BEGIN IMMEDIATE')
    try {
      const result = operation()
      this.db.exec('COMMIT')
      return result
    } catch (error) {
      this.db.exec('ROLLBACK')
      throw error
    }
  }

  /**
   * Create the page metadata. The native kernel initializes its workspace when opened.
   * @param input - owner-supplied title, type and idempotent identity.
   * @param signal - caller cancellation.
   * @returns the committed original page.
   */
  create(input: WikiCreatePage, signal: AbortSignal): WikiPage {
    signal.throwIfAborted()
    const request = createSchema.parse(input)
    const stamp = Date.parse(request.createdAt)
    const id = pageIdSchema.parse(String(stamp) + String(parseInt(digest(request.operationId).slice(0, 8), 16) % 1_000_000).padStart(6, '0'))
    return this.transaction(() => {
      if (request.parentId) this.page(request.parentId)
      const existing = this.tree().find(item => item.pageId === id)
      if (existing) {
        if (existing.deletedAt !== null || existing.name !== request.name || existing.type !== request.type
          || existing.parentId !== request.parentId) throw new WikiError('source_conflict')
        return existing
      }
      const seconds = Math.trunc(stamp / 1000)
      this.db.prepare('INSERT INTO pages (page_id, name, parent_id, rank, create_at, update_at, type, ext) VALUES (?, ?, ?, ?, ?, ?, ?, ?)')
        .run(id, request.name, request.parentId, 'new-' + id, seconds, seconds, pageTypes.indexOf(request.type), '{}')
      return this.page(id)
    })
  }

  /**
   * Atomically update original SQLite page metadata and the local recycle state.
   * @param input - revision-checked edit.
   * @param signal - caller cancellation.
   * @returns completion of the metadata transaction.
   */
  change(input: WikiPageChange, signal: AbortSignal): void {
    signal.throwIfAborted()
    if (this.hasEditors()) throw new WikiError('editor_busy')
    const request = changeSchema.parse(input)
    this.transaction(() => {
      const pages = this.tree()
      const page = pages.find(item => item.pageId === request.pageId)
      if (!page || page.hash !== request.hash) throw new WikiError('source_conflict')
      if ((request.kind === 'restore') !== (page.deletedAt !== null)) throw new WikiError('page_unavailable')
      const updated = Math.trunc(Date.now() / 1000)
      if (request.kind === 'rename') {
        this.db.prepare('UPDATE pages SET name = ?, update_at = ? WHERE page_id = ?').run(request.name, updated, page.pageId)
      } else if (request.kind === 'move') {
        if (request.parentId) {
          this.page(request.parentId)
          if (descendants(page, pages).some(item => item.pageId === request.parentId)) throw new WikiError('invalid_page_tree')
        }
        this.db.prepare('UPDATE pages SET parent_id = ?, rank = ?, update_at = ? WHERE page_id = ?')
          .run(request.parentId, request.rank, updated, page.pageId)
      } else {
        const affected = descendants(page, pages).filter(item => item.deletedAt === page.deletedAt)
        if (request.kind === 'restore') {
          if (page.parentId) this.page(page.parentId)
          const restore = this.db.prepare('DELETE FROM dsh_wiki_recycle WHERE page_id = ?')
          for (const item of affected) restore.run(item.pageId)
        } else {
          const trash = this.db.prepare('INSERT INTO dsh_wiki_recycle (page_id, deleted_at) VALUES (?, ?)')
          const now = new Date()
          const cohorts = new Set(pages.map(item => item.deletedAt))
          while (cohorts.has(now.toISOString())) now.setTime(now.getTime() + 1)
          for (const item of affected) trash.run(item.pageId, now.toISOString())
        }
      }
    })
  }

  /**
   * Open the original desktop or single-document application for a native page.
   * @param id - WikiMan page identity.
   * @param parent - DSH browser origin.
   * @param reference - optional previously read document.
   * @param locale - DSH language to apply to this native workspace.
   * @param signal - caller cancellation before handoff.
   * @returns the native browser view.
   */
  async open(id: string, parent: string, reference: WikiDocument | null, locale: string, signal: AbortSignal): Promise<WikiSiyuanView> {
    if (this.closing) await this.closing
    signal.throwIfAborted()
    if (this.hasEditors()) throw new WikiError('editor_busy')
    const page = this.page(id)
    if (page.type === 'table') throw new WikiError('page_unavailable')
    const opening = this.startView(page, parent, reference, locale, signal)
    this.opening = opening
    try {
      return await opening
    } finally {
      this.opening = undefined
    }
  }

  private async startView(
    page: WikiPage, parent: string, reference: WikiDocument | null, locale: string, signal: AbortSignal,
  ): Promise<WikiSiyuanView> {
    let kernel: SiyuanKernel | undefined
    try {
      const root = join(this.config.sourceRoot, 'pages')
      if (await directory(root) === null) await mkdir(root)
      const workspace = join(root, page.pageId)
      if (await directory(workspace) === null) await mkdir(workspace)
      const single = page.type === 'docx'
      if (reference && (reference.storeId !== this.storeId || reference.pageId !== page.pageId)) throw new WikiError('source_changed')
      const revision = reference ? await this.text(reference, signal) : undefined
      const first = reference?.docId ?? (await this.files(page.pageId, signal))[0]?.docId
      kernel = new SiyuanKernel(this.subprocess, this.config, workspace)
      await kernel.start(single, page.name, locale, signal)
      const changed = revision !== undefined && revision.hash !== reference?.hash
      const view = await kernel.view(parent, single, first, changed, viewId => this.close(viewId), signal)
      signal.throwIfAborted()
      this.active = { kernel, view }
      return view
    } catch (error) {
      await kernel?.dispose()
      throw error
    }
  }

  /**
   * Check whether the original editor is leased.
   * @returns whether a browser is opening or holds the native runtime.
   */
  hasEditors(): boolean {
    return this.opening !== undefined || this.active !== undefined
  }

  /**
   * Release the native browser view after its committed transaction queue has drained.
   * @param viewId - identity of the current browser view.
   * @returns completion of the managed native process teardown.
   */
  async close(viewId: string): Promise<void> {
    const active = this.active
    if (!active || active.view.viewId !== viewId) return
    const closing = this.closing ??= active.kernel.dispose()
    try {
      await closing
      this.active = undefined
    } finally {
      this.closing = undefined
    }
  }

  /**
   * Query committed native document files across one bounded page batch.
   * @param query - literal search term.
   * @param cursor - continuation from the previous page batch.
   * @param limit - maximum document hits.
   * @param signal - caller cancellation.
   * @returns matching committed documents and page continuation.
   */
  async search(
    query: string, cursor: string, limit: number, signal: AbortSignal,
  ): Promise<{ documents: WikiDocument[]; nextCursor: string; truncated: boolean }> {
    const batch = this.pages(cursor, signal)
    const documents: WikiDocument[] = []
    let bytes = 0
    let files = 0
    const term = query.toLocaleLowerCase()
    for (const page of batch.items) {
      if (page.type === 'table') continue
      for (const document of await this.files(page.pageId, signal)) {
        signal.throwIfAborted()
        const content = await readRegular(document.path, this.config.maxResponseBytes)
        if (!content) continue
        bytes += content.length
        if (++files > this.config.maxSourceFiles || bytes > this.config.maxSourceBytes) throw new WikiError('source_limit_exceeded')
        const result = textOf(content, document)
        if (!result.text.toLocaleLowerCase().includes(term)) continue
        if (documents.length >= limit) return { documents, nextCursor: batch.nextCursor, truncated: true }
        const { pageId, boxId, docId } = document
        documents.push({ pageId, boxId, docId, storeId: this.storeId, title: result.title, hash: result.hash })
      }
    }
    return { documents, nextCursor: batch.nextCursor, truncated: false }
  }

  /**
   * Read the durable content and revision that the original kernel wrote.
   * @param input - original document coordinates.
   * @param signal - caller cancellation.
   * @returns text, hash and library identity for the source citation.
   */
  async text(input: WikiDocumentKey, signal: AbortSignal): Promise<{ text: string; hash: string; storeId: string }> {
    const key = documentKey.parse(input)
    this.page(key.pageId)
    const matches = (await this.files(key.pageId, signal)).filter(item => item.boxId === key.boxId && item.docId === key.docId)
    if (matches.length !== 1) throw new WikiError('document_not_found')
    const match = matches[0]
    const content = match && await readRegular(match.path, this.config.maxResponseBytes)
    if (!content) throw new WikiError('document_not_found')
    return { ...textOf(content, key), storeId: this.storeId }
  }

  private async files(pageId: string, signal: AbortSignal): Promise<NativeDocument[]> {
    const pages = join(this.config.sourceRoot, 'pages')
    if (await directory(pages) === null) return []
    const workspace = join(pages, pageId)
    if (await directory(workspace) === null) return []
    const data = join(workspace, 'data')
    const docs: NativeDocument[] = []
    let visited = 0
    const visit = async (root: string, boxId: string, depth: number) => {
      if (depth > 64) throw new WikiError('invalid_source_path')
      for (const name of await directory(root) ?? []) {
        signal.throwIfAborted()
        if (++visited > this.config.maxSourceFiles) throw new WikiError('source_limit_exceeded')
        const path = join(root, name)
        const docId = nativeFile.exec(name)?.[1]
        if (docId) docs.push({ pageId, boxId, docId, path })
        else if (nativeId.safeParse(name).success) await visit(path, boxId, depth + 1)
      }
    }
    for (const name of await directory(data) ?? []) {
      if (nativeId.safeParse(name).success) await visit(join(data, name), name, 0)
    }
    return docs
  }

  /**
   * Release the active original editor and close the metadata database.
   * @returns completion of plugin-owned cleanup.
   */
  async dispose(): Promise<void> {
    try {
      await this.opening?.catch((_error: unknown) => { /* The open request reports startup failures. */ })
      if (this.active) await this.close(this.active.view.viewId)
    } finally {
      this.db.close()
    }
  }
}
