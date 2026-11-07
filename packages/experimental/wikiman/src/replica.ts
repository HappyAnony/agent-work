/** Bidirectional portable replication over a directory with atomic mkdir and rename. */
import { randomUUID } from 'node:crypto'
import { mkdir, mkdtemp, realpath, rename, rm, rmdir } from 'node:fs/promises'
import { isAbsolute, join, relative, sep } from 'node:path'
import { z } from 'zod'
import { WikiError } from './kernel.ts'
import type { NativeValidation } from './native-validation.ts'
import { digestSchema, directory, ensureDirectory, fsError, portableStoreId, publishJSON, readRegular, SourceFiles, type SourceLimits } from './source-files.ts'
import type { WikiSyncResult } from './types.ts'

const pointerSchema = z.object({
  version: z.literal(1),
  base: digestSchema,
  active: z.string().regex(/^work-[a-f0-9-]{36}$/u).nullable(),
}).strict()
type Pointer = z.infer<typeof pointerSchema>
const headSchema = z.object({ version: z.literal(1), digest: digestSchema }).strict()

/** Configured shared directory and resource limits for portable replication. */
export interface ReplicaConfig extends SourceLimits {
  /** Absolute shared directory for one store's verified generations. */
  readonly syncDirectory?: string
}

async function jsonFile<T>(path: string, schema: z.ZodType<T>): Promise<T | null> {
  const raw = await readRegular(path, 4096)
  if (!raw) return null
  const result = schema.safeParse(JSON.parse(raw.toString('utf8')))
  if (!result.success) throw new WikiError('replica_invalid')
  return result.data
}

function contains(parent: string, child: string): boolean {
  const path = relative(parent, child)
  return path === '' || path !== '..' && !path.startsWith('..' + sep) && !isAbsolute(path)
}

/** Keeps verified immutable copies and a pointer to the active local working directory. */
export class PortableReplica {
  private readonly stateRoot: string

  constructor(
    private readonly sourceRoot: string,
    private readonly config: ReplicaConfig,
    private readonly native: NativeValidation,
  ) {
    this.stateRoot = join(sourceRoot, '.local', 'dsh-wikiman-sync')
  }

  /**
   * Locate the active working source on DSH startup.
   * @returns the original or active physical store directory.
   */
  async activeRoot(): Promise<string> {
    const state = await this.pointer()
    const root = state?.active ? join(this.stateRoot, state.active) : this.sourceRoot
    if (await directory(root) === null) throw new WikiError('replica_invalid')
    if (root !== this.sourceRoot && await portableStoreId(root) !== await portableStoreId(this.sourceRoot)) {
      throw new WikiError('store_identity_mismatch')
    }
    return root
  }

  /**
   * Synchronize a source whose owner has paused writes, preserving independent changes as immutable copies.
   * @param root - active local source directory.
   * @param currentStoreId - reads the identity while the Host still owns the native session lock.
   * @param activate - start the kernel for a verified received working copy before committing its pointer.
   * @param signal - cancellation before publication.
   * @returns synchronization outcome and native validation warnings.
   */
  async sync(
    root: string, currentStoreId: () => string, activate: (root: string) => Promise<void>, signal: AbortSignal,
  ): Promise<WikiSyncResult> {
    const configured = this.config.syncDirectory
    if (!configured) throw new WikiError('sync_not_configured')
    if (!isAbsolute(configured)) throw new WikiError('absolute_paths_required')
    const shared = await realpath(configured)
    if (contains(this.sourceRoot, shared) || contains(shared, this.sourceRoot)) throw new WikiError('sync_directory_overlap')
    const storeId = currentStoreId()
    if (storeId !== await portableStoreId(this.sourceRoot)) throw new WikiError('store_identity_mismatch')
    const remote = join(shared, 'wikiman-' + storeId)
    await ensureDirectory(remote)
    const lock = join(remote, '.sync-lock')
    try {
      await mkdir(lock, { mode: 0o700 })
    } catch (error) {
      if (fsError(error, 'EEXIST')) throw new WikiError('sync_peer_busy')
      throw error
    }
    try {
      return await this.synchronize(root, remote, storeId, currentStoreId, activate, signal)
    } finally {
      await rmdir(lock)
    }
  }

  private async pointer(): Promise<Pointer | null> {
    if (await directory(join(this.sourceRoot, '.local')) === null || await directory(this.stateRoot) === null) return null
    return await jsonFile(join(this.stateRoot, 'state.json'), pointerSchema)
  }

  private async synchronize(
    root: string, remote: string, storeId: string, currentStoreId: () => string,
    activate: (root: string) => Promise<void>, signal: AbortSignal,
  ): Promise<WikiSyncResult> {
    await ensureDirectory(join(this.sourceRoot, '.local'))
    await ensureDirectory(this.stateRoot)
    const state = await this.pointer()
    if ((state?.active ? join(this.stateRoot, state.active) : this.sourceRoot) !== root) throw new WikiError('source_changed')
    const head = await jsonFile(join(remote, 'HEAD.json'), headSchema)
    if (!head && state) throw new WikiError('sync_remote_missing')
    const generations = join(remote, 'generations')
    await ensureDirectory(generations)
    const staged = await mkdtemp(join(this.stateRoot, 'stage-'))
    try {
      const local = await new SourceFiles(this.config, signal).portable(root, staged)
      const warnings = await this.native.validate(staged, storeId, signal)
      const current = await new SourceFiles(this.config, signal).portable(root)
      if (current.digest !== local.digest || currentStoreId() !== storeId) throw new WikiError('source_changed')

      if (head) await this.verify(join(generations, head.digest), head.digest, storeId, signal)
      if (head && head.digest !== local.digest && state?.base !== head.digest && state?.base !== local.digest) {
        const conflicts = join(this.stateRoot, 'conflicts')
        await ensureDirectory(conflicts)
        await this.seal(staged, conflicts, local.digest, storeId, signal)
        await this.seal(join(generations, head.digest), conflicts, head.digest, storeId, signal)
        await this.native.validate(join(conflicts, head.digest), storeId, signal)
        return { kind: 'conflict', localDigest: local.digest, remoteDigest: head.digest }
      }

      if (head && head.digest !== local.digest && state?.base === local.digest) {
        const active = 'work-' + randomUUID()
        const candidate = join(this.stateRoot, active)
        await mkdir(candidate, { mode: 0o700 })
        const incoming = await new SourceFiles(this.config, signal).portable(join(generations, head.digest), candidate)
        if (incoming.digest !== head.digest) throw new WikiError('replica_invalid')
        const incomingWarnings = await this.native.validate(candidate, storeId, signal)
        signal.throwIfAborted()
        await activate(candidate)
        await publishJSON(join(this.stateRoot, 'state.json'), { version: 1, active, base: head.digest })
        return { kind: 'received', digest: head.digest, warnings: incomingWarnings }
      }

      const kind = head?.digest === local.digest ? 'unchanged' : 'published'
      if (kind === 'published') {
        await this.seal(staged, generations, local.digest, storeId, signal)
        signal.throwIfAborted()
        await publishJSON(join(remote, 'HEAD.json'), { version: 1, digest: local.digest })
      }
      await publishJSON(join(this.stateRoot, 'state.json'), { version: 1, active: state?.active ?? null, base: local.digest })
      return { kind, digest: local.digest, warnings }
    } finally {
      await rm(staged, { recursive: true, force: true })
    }
  }

  private async verify(root: string, digest: string, storeId: string, signal: AbortSignal): Promise<void> {
    if (await directory(root) === null || (await new SourceFiles(this.config, signal).portable(root)).digest !== digest
      || await portableStoreId(root) !== storeId) throw new WikiError('replica_invalid')
  }

  private async seal(source: string, parent: string, digest: string, storeId: string, signal: AbortSignal): Promise<void> {
    const root = join(parent, digest)
    if (await directory(root) !== null) {
      await this.verify(root, digest, storeId, signal)
      return
    }
    const staged = await mkdtemp(join(parent, '.stage-'))
    try {
      const copied = await new SourceFiles(this.config, signal).portable(source, staged)
      if (copied.digest !== digest) throw new WikiError('source_changed')
      signal.throwIfAborted()
      await rename(staged, root)
    } finally {
      await rm(staged, { recursive: true, force: true })
    }
  }
}
