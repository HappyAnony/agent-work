/** Filesystem operations for sealed portable generations and read-only legacy copies. */
import { createHash, randomUUID } from 'node:crypto'
import { constants } from 'node:fs'
import { lstat, mkdir, open, readdir, rename, rm } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { z } from 'zod'
import { WikiError } from './kernel.ts'

const maxFileBytes = 64 * 1024 * 1024

/** Native portable page identity. */
export const pageIdSchema = z.string().regex(/^\d{19}$/u)
/** SHA-256 for byte-identical files and portable generation inventories. */
export const digestSchema = z.string().regex(/^[a-f0-9]{64}$/u)
/** Portable store identity from the native store manifest. */
export const storeIdSchema = z.uuid()

/** Byte and entry budgets for an owner's source operation. */
export interface SourceLimits {
  /** Aggregate bytes accepted from a portable or legacy source. */
  readonly maxSourceBytes: number
  /** Aggregate files and directories accepted per inventory. */
  readonly maxSourceFiles: number
}

/**
 * Identify a missing path without hiding other filesystem failures.
 * @param error - Node filesystem failure.
 * @param code - expected error code.
 * @returns whether the failure has that code.
 */
export function fsError(error: unknown, code: string): boolean {
  return typeof error === 'object' && error !== null && 'code' in error && error.code === code
}

/**
 * Read a regular file with a size limit and refuse symbolic links.
 * @param path - file within a directory checked by the caller.
 * @param limit - maximum file bytes.
 * @returns the file bytes or null when the file is absent.
 */
export async function readRegular(path: string, limit: number): Promise<Buffer | null> {
  let file
  try {
    const stat = await lstat(path)
    if (!stat.isFile() || stat.size > limit) throw new WikiError('invalid_source_file')
    file = await open(path, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK)
  } catch (error) {
    if (fsError(error, 'ENOENT')) return null
    throw error
  }
  try {
    const stat = await file.stat()
    if (!stat.isFile() || stat.size > limit) throw new WikiError('invalid_source_file')
    const data = Buffer.allocUnsafe(stat.size)
    let bytesRead = 0
    while (bytesRead < data.length) {
      const chunk = await file.read(data, { offset: bytesRead, position: bytesRead })
      if (chunk.bytesRead === 0) break
      bytesRead += chunk.bytesRead
    }
    const after = await file.stat()
    if (bytesRead !== data.length || stat.size !== after.size || stat.mtimeMs !== after.mtimeMs || stat.ctimeMs !== after.ctimeMs) {
      throw new WikiError('source_changed')
    }
    return data
  } finally {
    await file.close()
  }
}

/**
 * Return a directory's entries in stable byte order and reject symbolic links to directories.
 * @param path - directory within a checked root.
 * @returns sorted names, or null when the directory is absent.
 */
export async function directory(path: string): Promise<string[] | null> {
  try {
    if (!(await lstat(path)).isDirectory()) throw new WikiError('invalid_source_directory')
    return (await readdir(path)).sort()
  } catch (error) {
    if (fsError(error, 'ENOENT')) return null
    throw error
  }
}

/**
 * Create or check a direct child directory of an already checked directory.
 * @param path - destination directory.
 * @returns completion of directory creation and validation.
 */
export async function ensureDirectory(path: string): Promise<void> {
  try {
    await mkdir(path, { mode: 0o700 })
  } catch (error) {
    if (!fsError(error, 'EEXIST')) throw error
  }
  if (await directory(path) === null) throw new WikiError('source_changed')
}

async function syncDirectory(path: string): Promise<void> {
  if (process.platform === 'win32') return
  const file = await open(path, 'r')
  try {
    await file.sync()
  } finally {
    await file.close()
  }
}

async function writeNew(path: string, data: Uint8Array): Promise<void> {
  const file = await open(path, 'wx', 0o600)
  try {
    await file.writeFile(data)
    await file.sync()
  } finally {
    await file.close()
  }
}

/**
 * Atomically publish a small JSON pointer in an existing directory.
 * @param path - pointer path.
 * @param value - serializable version or inventory.
 * @returns completion of publication.
 */
export async function publishJSON(path: string, value: object): Promise<void> {
  const temporary = join(dirname(path), '.writing-' + randomUUID())
  try {
    await writeNew(temporary, Buffer.from(JSON.stringify(value) + '\n'))
    await rename(temporary, path)
    await syncDirectory(dirname(path))
  } finally {
    await rm(temporary, { force: true })
  }
}

/**
 * Read the manifest identity without following a link out of the source.
 * @param root - checked source root.
 * @returns the portable store UUID.
 */
export async function portableStoreId(root: string): Promise<string> {
  const file = await readRegular(join(root, 'store.json'), 1024 * 1024)
  if (!file) throw new WikiError('store_manifest_missing')
  return z.object({ storeId: storeIdSchema }).parse(JSON.parse(file.toString('utf8'))).storeId
}

type FileEntry = readonly [path: string, digest: string]
type Inventory = readonly FileEntry[]
const compareEntries = (a: FileEntry, b: FileEntry) => a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0
const hash = (data: Buffer | string) => createHash('sha256').update(data).digest('hex')

/** SHA-256 and number of files in the source inventory. */
export interface SourceSnapshot {
  readonly digest: string
  readonly files: number
}

function pathPart(name: string): boolean {
  return name !== '' && name !== '.' && name !== '..' && !/[\\:\u0000-\u001f]/u.test(name)
}

function portableSelection(name: string): 'include' | 'skip' {
  const [first, second, third] = name.split('/')
  if (first === '.wiki') return second === undefined || name === '.wiki/preferences.json' ? 'include' : 'skip'
  if (first === 'pages' && second !== undefined) {
    if (!pageIdSchema.safeParse(second).success) return 'skip'
    if (third !== undefined && third !== 'page.json' && third !== 'data') return 'skip'
  }
  return ['store.json', 'pages', 'tombstones', 'deleted', 'versions'].some(root => root === first) ? 'include' : 'skip'
}

/** Bounded, byte-preserving inventory of one source and optional private copy. */
export class SourceFiles {
  private bytes = 0
  private visited = 0
  private readonly entries: FileEntry[] = []
  private readonly paths = new Set<string>()
  private readonly directories: FileEntry[] = []

  constructor(private readonly limits: SourceLimits, private readonly signal: AbortSignal) {}

  /**
   * Inventory portable source files and optionally copy them to a fresh directory.
   * @param source - checked portable root.
   * @param destination - fresh directory, or undefined for inventory alone.
   * @returns the content address of the source.
   */
  async portable(source: string, destination?: string): Promise<SourceSnapshot> {
    await this.walk(source, destination, '', portableSelection, name => name)
    if (!this.paths.has('store.json')) throw new WikiError('store_manifest_missing')
    return this.snapshot()
  }

  /**
   * Copy one legacy data directory, translating the native document extension.
   * @param source - legacy data directory.
   * @param destination - fresh portable data directory.
   * @param pageId - owning page for the aggregate inventory.
   * @returns relative paths copied under the portable data directory.
   */
  async legacyData(source: string, destination: string | undefined, pageId: string): Promise<readonly string[]> {
    const prefix = 'pages/' + pageId + '/data'
    const before = this.entries.length
    await this.walk(source, destination, prefix, () => 'include', (name, path) => {
      const box = path.split('/')[3] ?? ''
      return /^\d{14}-[a-z0-9]{7}$/u.test(box) && /^\d{14}-[a-z0-9]{7}\.sy$/u.test(name)
        ? name.replace(/\.sy$/u, '.docj') : name
    })
    return this.entries.slice(before).map(([name]) => name.slice(prefix.length + 1))
  }

  /**
   * Include a generated metadata record in the operation's aggregate inventory.
   * @param name - generated portable file path relative to the new source.
   * @param path - fresh file destination.
   * @param data - JSON bytes for the file.
   * @returns completion of the verified write.
   */
  async metadata(name: string, path: string, data: Buffer): Promise<void> {
    this.record(name, data)
    await writeNew(path, data)
  }

  /**
   * Address the source by sorted path and exact file bytes.
   * @returns inventory digest and file count.
   */
  snapshot(): SourceSnapshot {
    const inventory: Inventory = [...this.entries, ...this.directories].sort(compareEntries)
    return { digest: hash(JSON.stringify(inventory)), files: inventory.length }
  }

  private record(name: string, data: Buffer): void {
    this.bytes += data.length
    if (this.entries.length >= this.limits.maxSourceFiles || this.bytes > this.limits.maxSourceBytes) {
      throw new WikiError('source_limit_exceeded')
    }
    if (this.paths.has(name)) throw new WikiError('source_path_collision')
    this.paths.add(name)
    this.entries.push([name, hash(data)])
  }

  private async walk(
    source: string, destination: string | undefined, prefix: string,
    select: (path: string) => 'include' | 'skip', convert: (name: string, prefix: string) => string,
  ): Promise<void> {
    const names = await directory(source)
    if (names === null) return
    for (const name of names) {
      this.signal.throwIfAborted()
      const targetName = convert(name, prefix)
      const path = prefix ? prefix + '/' + targetName : targetName
      if (name.startsWith('.wiki-') || select(path) === 'skip') continue
      if (++this.visited > this.limits.maxSourceFiles || !pathPart(name) || path.length > 4096 || path.split('/').length > 64) {
        throw new WikiError('invalid_source_path')
      }
      const child = join(source, name)
      const target = destination && join(destination, targetName)
      const stat = await lstat(child)
      if (stat.isDirectory()) {
        this.directories.push([path + '/', ''])
        if (target) await mkdir(target, { mode: 0o700 })
        await this.walk(child, target, path, select, convert)
      } else {
        if (!stat.isFile()) throw new WikiError('invalid_source_file')
        const data = await readRegular(child, maxFileBytes)
        if (!data) throw new WikiError('source_changed')
        this.record(path, data)
        if (target) await writeNew(target, data)
      }
    }
    if (destination) await syncDirectory(destination)
  }
}
