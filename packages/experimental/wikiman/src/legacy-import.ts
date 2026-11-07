/** Read-only import from a WikiMan SQLite library into a validated portable store. */
import { randomUUID } from 'node:crypto'
import { lstat, mkdir, mkdtemp, realpath, rename, rm, rmdir, writeFile } from 'node:fs/promises'
import { isAbsolute, join, relative, sep } from 'node:path'
import { z } from 'zod'
import { parseWikiJSON, WikiError } from './kernel.ts'
import type { NativeValidation } from './native-validation.ts'
import { digestSchema, directory, fsError, pageIdSchema, readRegular, SourceFiles, storeIdSchema, type SourceLimits } from './source-files.ts'
import type { WikiImportConfirmation, WikiImportReport, WikiLegacyImport } from './types.ts'

const requestSchema = z.object({
  source: z.string().min(1).max(4096),
  destination: z.string().min(1).max(4096),
  name: z.string().trim().min(1).max(256),
})
const confirmationSchema = z.object({ storeId: storeIdSchema, digest: digestSchema })
const legacyPage = z.object({
  page_id: pageIdSchema,
  parent_id: pageIdSchema.nullable(),
  name: z.string().min(1),
  rank: z.string().min(1),
  create_at: z.number().int().positive(),
  update_at: z.number().int().positive(),
  type: z.union([z.literal(0), z.literal(1), z.literal(2)]),
  ext: z.string(),
  deleted_at: z.iso.datetime().nullable(),
})
const nativeDocument = /^(\d{14}-[a-z0-9]{7})\/(?:\d{14}-[a-z0-9]{7}\/)*(\d{14}-[a-z0-9]{7})\.docj$/u
const timestamp = (value: number) => new Date(value < 100_000_000_000 ? value * 1000 : value).toISOString()
const json = (value: object) => Buffer.from(JSON.stringify(value) + '\n')

async function assertClosed(root: string): Promise<void> {
  for (const name of ['page.db-wal', 'page.db-journal']) {
    try {
      await readRegular(join(root, name), 0)
    } catch (error) {
      throw new WikiError('legacy_database_in_use', { cause: error })
    }
  }
}

async function readPages(database: string, limit: number) {
  const { DatabaseSync } = await import('node:sqlite')
  const db = new DatabaseSync(database, { readOnly: true, allowExtension: false })
  try {
    db.exec('PRAGMA trusted_schema=OFF')
    const hasRecycle = db.prepare("SELECT 1 FROM sqlite_schema WHERE name = 'dsh_wiki_recycle' AND type = 'table'").get()
    const recycledAt = hasRecycle ? '(SELECT deleted_at FROM dsh_wiki_recycle r WHERE r.page_id = pages.page_id)' : 'NULL'
    const pages = db.prepare(`SELECT page_id, parent_id, name, rank, create_at, update_at, type, ext,
      ${recycledAt} AS deleted_at FROM pages ORDER BY page_id LIMIT ?`)
      .all(limit + 1).map(row => legacyPage.parse(row))
    if (pages.length > limit) throw new WikiError('source_limit_exceeded')
    const hasTombstones = db.prepare("SELECT 1 FROM sqlite_schema WHERE name = 'page_tree_tombstones' AND type = 'table'").get()
    const deleted = hasTombstones
      ? z.number().int().nonnegative().parse(db.prepare('SELECT count(*) AS count FROM page_tree_tombstones').get()?.count)
      : 0
    return { pages, deleted }
  } finally {
    db.close()
  }
}

/**
 * Build and validate a new portable copy of a closed legacy WikiMan library.
 * @param input - legacy directory, destination parent and new store title.
 * @param confirmation - preview to confirm, or null to discard the staged copy after inspection.
 * @param limits - aggregate source budgets.
 * @param native - native portable store validator.
 * @param signal - cancellation before publication.
 * @returns the validated inventory and, on confirmation, the new store directory.
 */
export async function importLegacy(
  input: WikiLegacyImport, confirmation: WikiImportConfirmation | null, limits: SourceLimits, native: NativeValidation, signal: AbortSignal,
): Promise<WikiImportReport> {
  const request = requestSchema.parse(input)
  const expected = confirmation === null ? null : confirmationSchema.parse(confirmation)
  if (![request.source, request.destination].every(isAbsolute)) throw new WikiError('absolute_paths_required')
  const [source, destination] = await Promise.all([realpath(request.source), realpath(request.destination)])
  const child = relative(source, destination)
  if (child === '' || child !== '..' && !child.startsWith('..' + sep) && !isAbsolute(child)) throw new WikiError('import_destination_overlap')
  const storeId = expected?.storeId ?? randomUUID()
  const database = join(source, 'page.db')
  await assertClosed(source)
  const bytes = await readRegular(database, Math.min(limits.maxSourceBytes, 64 * 1024 * 1024))
  if (!bytes) throw new WikiError('legacy_database_missing')
  const stage = await mkdtemp(join(destination, '.wikiman-import-'))
  const portable = join(stage, 'portable')
  try {
    const copy = join(stage, 'page.db')
    await writeFile(copy, bytes, { mode: 0o600, flag: 'wx' })
    const { pages, deleted } = await readPages(copy, limits.maxSourceFiles)
    const pageRoot = join(source, 'pages')
    const pageIds = new Set(pages.map(row => row.page_id))
    if ((await directory(pageRoot) ?? []).some(name => !name.startsWith('.') && !pageIds.has(name))) {
      throw new WikiError('legacy_orphaned_pages')
    }
    await mkdir(portable, { mode: 0o700 })
    await mkdir(join(portable, 'pages'), { mode: 0o700 })
    const writer = new SourceFiles(limits, signal)
    const verifier = new SourceFiles(limits, signal)
    const manifest = json({ schemaVersion: 1, storeId, name: request.name, documentFormat: 'docj', layoutVersion: 1 })
    await writer.metadata('store.json', join(portable, 'store.json'), manifest)
    let documents = 0
    for (const page of pages) {
      signal.throwIfAborted()
      const target = join(portable, 'pages', page.page_id)
      await mkdir(target, { mode: 0o700 })
      const existing = join(pageRoot, page.page_id)
      const data = join(existing, 'data')
      if (await directory(existing) !== null && await directory(data) !== null) {
        await mkdir(join(target, 'data'), { mode: 0o700 })
      }
      const paths = await writer.legacyData(data, join(target, 'data'), page.page_id)
      const found = paths.flatMap((path) => {
        const match = nativeDocument.exec(path)
        return match ? [{ boxId: match[1], docId: match[2] }] : []
      })
      documents += found.length
      if (page.type === 0 && found.length !== 1) throw new WikiError('legacy_docx_entry_ambiguous')
      const ext = z.record(z.string(), z.unknown()).parse(parseWikiJSON(page.ext))
      const record = {
        schemaVersion: 1, pageId: page.page_id, name: page.name, parentId: page.parent_id, rank: page.rank,
        type: (['docx', 'wiki', 'table'] as const)[page.type], revision: 1,
        createdAt: timestamp(page.create_at), updatedAt: timestamp(page.update_at), deletedAt: page.deleted_at,
        entryDocument: page.type === 0 ? found[0] : null, ext,
      }
      const name = 'pages/' + page.page_id + '/page.json'
      await writer.metadata(name, join(target, 'page.json'), json(record))
    }

    const warnings = await native.validate(portable, storeId, signal)
    const { digest } = await new SourceFiles(limits, signal).portable(portable)
    const sourceFiles = new SourceFiles(limits, signal)
    for (const page of pages) {
      const data = join(pageRoot, page.page_id, 'data')
      await verifier.legacyData(join(portable, 'pages', page.page_id, 'data'), undefined, page.page_id)
      await sourceFiles.legacyData(data, undefined, page.page_id)
    }
    await assertClosed(source)
    const current = await readRegular(database, bytes.length)
    if (!current || !bytes.equals(current) || sourceFiles.snapshot().digest !== verifier.snapshot().digest) {
      throw new WikiError('source_changed')
    }
    signal.throwIfAborted()
    if (expected && expected.digest !== digest) throw new WikiError('import_preview_changed')
    const output = expected ? join(destination, 'wikiman-' + storeId) : null
    if (output) {
      await publishImport(portable, output, signal)
    }
    return { digest, storeId, destination: output, pages: pages.length, documents, deletedLegacyPages: deleted, warnings }
  } finally {
    await rm(stage, { recursive: true, force: true })
  }
}

async function publishImport(staged: string, output: string, signal: AbortSignal): Promise<void> {
  const lock = output + '.import-lock'
  try {
    await mkdir(lock, { mode: 0o700 })
  } catch (error) {
    if (fsError(error, 'EEXIST')) throw new WikiError('import_destination_exists')
    throw error
  }
  try {
    try {
      await lstat(output)
      throw new WikiError('import_destination_exists')
    } catch (error) {
      if (!fsError(error, 'ENOENT')) throw error
    }
    signal.throwIfAborted()
    await rename(staged, output)
  } finally {
    await rmdir(lock)
  }
}
