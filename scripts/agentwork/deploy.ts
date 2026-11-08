/** Stopped-service program installation and rollback preserve user data and retain private backups. */
import { copyFile, cp, lstat, mkdir, mkdtemp, open, readFile, readlink, rename, rm, statfs, symlink, unlink, writeFile } from 'node:fs/promises'
import { basename, isAbsolute, join, resolve } from 'node:path'
import { z } from 'zod'
import { ReleaseError, canonicalPath, hashFile, verifyArtifact, within } from './model.ts'
import { run, type Execution } from './process.ts'

/** Deployment selects programs only; the service owner starts the returned named-profile command. */
export interface DeployRequest extends Execution {
  /** Verified server artifact directory. */
  artifact: string
  /** Private server configuration JSON, outside program directories. */
  config: string
  /** Deployment root with immutable versions and a current symlink. */
  root: string
  /** Read-only inspection; no locks, backups or program paths are created. */
  dryRun: boolean
  /** Require an existing current version and compatible data formats. */
  rollback: boolean
}

const pathsSchema = z.object({ home: z.string().min(1), library: z.string().min(1), workspace: z.string().min(1) }).loose()

/**
 * Validate a deployment runner before a workflow queues work in the trusted network.
 * @param labels - JSON list configured as AGENTWORK_DEPLOY_RUNNER.
 * @param root - Absolute program root on that runner.
 * @param config - Absolute private server configuration path on that runner.
 * @returns Runner labels for the orchestration job; missing configuration fails immediately.
 */
export function deploymentRunner(labels: string, root: string, config: string): string[] {
  const parsed = z.array(z.string().regex(/^[a-zA-Z0-9._-]{1,80}$/u)).min(3).max(10).parse(JSON.parse(labels))
  if (!parsed.includes('self-hosted') || !parsed.includes('agentwork-deploy') || !isAbsolute(root) || !isAbsolute(config)) throw new ReleaseError('deployment-configuration', 'Configure a controlled self-hosted agentwork-deploy runner and absolute program/config paths')
  return parsed
}

async function existing(path: string): Promise<boolean> {
  try { await lstat(path); return true }
  catch (error) { if ((error as NodeJS.ErrnoException).code === 'ENOENT') return false; throw error }
}

/**
 * Install/select a server program while its owner-managed service is stopped.
 * @param request - Artifact, data configuration, program root and dry-run mode.
 * @param execute - Process transport; fixtures own it without changing global process APIs.
 * @returns Selected program and explicit start command; this operation never kills or starts a live service.
 */
export async function deployArtifact(request: DeployRequest, execute: typeof run = run) {
  const manifest = await verifyArtifact(request.artifact)
  if (!manifest.target.startsWith('server-') || manifest.platform !== process.platform || manifest.arch !== process.arch) throw new ReleaseError('wrong-platform', 'Deploy a server artifact matching this host OS and architecture')
  if (manifest.dirty) throw new ReleaseError('dirty-source', 'Deployment requires an artifact built from committed source')
  const root = await canonicalPath(request.root)
  const input = await canonicalPath(request.artifact)
  if (within(input, root)) throw new ReleaseError('data-overlap', 'Do not create the deployment program root inside its source artifact')
  const config = pathsSchema.parse(JSON.parse(await readFile(request.config, 'utf8')))
  const data = await Promise.all([config.home, config.library, config.workspace].map(canonicalPath))
  for (const path of data) {
    if (within(root, path) || within(path, root)) throw new ReleaseError('data-overlap', 'Keep the program root separate from home, library and workspace')
    if (data.some(other => other !== path && (within(path, other) || within(other, path)))) throw new ReleaseError('data-overlap', 'Home, library and workspace must not contain each other')
  }
  if (new Set(data).size !== 3 || within(root, await canonicalPath(request.config))) throw new ReleaseError('data-overlap', 'Keep three distinct data directories and a private configuration outside program versions')
  const current = join(root, 'current')
  let previous: string | undefined
  if (await existing(current)) {
    if (!(await lstat(current)).isSymbolicLink()) throw new ReleaseError('unmanaged-current', 'Current program path is not a managed symlink; preserve it and choose another deployment root')
    previous = await readlink(current)
    if (!within(join(root, 'releases'), resolve(root, previous))) throw new ReleaseError('unmanaged-current', 'Current symlink is outside managed program versions')
    const old = await verifyArtifact(resolve(root, previous))
    if (JSON.stringify(old.persistenceVersions) !== JSON.stringify(manifest.persistenceVersions)) throw new ReleaseError('data-incompatible', 'Program versions require different data formats; use the documented migration/backup recovery procedure')
  } else if (request.rollback) throw new ReleaseError('no-current-version', 'Rollback requires an existing managed current version')
  const runtime = join(request.artifact, 'payload/server')
  const identity = z.object({ storeId: z.string().min(1) }).loose().parse(JSON.parse(await readFile(join(config.library, 'store.json'), 'utf8')))
  async function inspectLibrary(): Promise<void> {
    const status = z.object({ ok: z.literal(true), result: z.object({ sessionLock: z.enum(['available', 'uninitialized']),
      sourceLock: z.enum(['readable', 'uninitialized']), recovery: z.literal('clear') }).loose() }).loose()
    const text = await execute(join(runtime, 'wiki/bin/wiki-cli'), ['status', '--root', config.library, '--request', '-', '--json'], request, true,
      JSON.stringify({ protocolVersion: 1, storeId: identity.storeId }))
    if (!status.safeParse(JSON.parse(text)).success) throw new ReleaseError('library-busy', 'Library is active, requires recovery or has unknown lock state; close its editors and stop every kernel owner before deployment')
  }
  await inspectLibrary()
  await execute(join(runtime, 'bin/node'), [join(runtime, 'start.mjs'), '--config', request.config, '--check'], request, true)
  const pid = join(config.home, '.agentwork-server.pid')
  if (await existing(pid)) throw new ReleaseError('service-running', 'Stop the managed server gracefully and verify saved editors before deployment; inspect a stale PID file manually instead of deleting it automatically')
  const destination = join(root, 'releases', manifest.releaseId + '-' + manifest.target)
  if (await existing(destination)) {
    await verifyArtifact(destination)
    if (await hashFile(join(destination, 'artifact.json')) !== await hashFile(join(request.artifact, 'artifact.json'))) throw new ReleaseError('version-exists', 'Selected version directory contains different content')
  }
  const start = [join(current, 'payload/server/bin/node'), join(current, 'payload/server/start.mjs'), '--config', request.config]
  const result = { releaseId: manifest.releaseId, target: manifest.target, previous, destination, current,
    data: { home: config.home, library: config.library, workspace: config.workspace }, start,
    service: 'stopped' as const, dryRun: request.dryRun, backup: undefined }
  if (request.dryRun) return result
  await mkdir(root, { recursive: true, mode: 0o700 })
  await mkdir(config.home, { recursive: true, mode: 0o700 })
  const guardPath = join(config.home, '.agentwork-deploy.guard')
  const rootGuardPath = join(root, '.deployment.guard')
  const rootGuard = await open(rootGuardPath, 'wx', 0o600)
  let guard
  try { guard = await open(guardPath, 'wx', 0o600) }
  catch (error) { await rootGuard.close(); await unlink(rootGuardPath); throw error }
  let temporaryLink: string | undefined
  try {
    if (await existing(pid)) throw new ReleaseError('service-running', 'A server started during deployment preflight; stop it before retrying')
    await inspectLibrary()
    const now = await existing(current) ? await readlink(current) : undefined
    if (now !== previous) throw new ReleaseError('deployment-moved', 'Current program changed during preflight; inspect it before retrying')
    // A backup of all three data directories is private and separate from release assets.
    const free = await statfs(root)
    const needed = manifest.files.reduce((sum, row) => sum + (row.kind === 'file' ? row.bytes : 0), 0)
    if (free.bavail * free.bsize < needed * 2) throw new ReleaseError('disk-full', 'Not enough space to stage the program and retain recovery files')
    await mkdir(join(root, 'backups'), { recursive: true, mode: 0o700 })
    const backup = await mkdtemp(join(root, 'backups/deploy-'))
    const transaction = basename(backup)
    for (const [name, path] of Object.entries({ home: data[0], library: data[1], workspace: data[2] })) {
      if (path && await existing(path)) await cp(path, join(backup, name), {
        recursive: true, verbatimSymlinks: true, filter: path => path !== guardPath && path !== pid,
      })
    }
    await writeFile(join(backup, 'operation.json'), JSON.stringify({ ...result, backup, status: 'backed-up', action: request.rollback ? 'rollback' : 'deploy' }, null, 2) + '\n', { flag: 'wx', mode: 0o600 })
    if (!(await existing(destination))) {
      await mkdir(join(root, 'releases'), { recursive: true })
      const staging = await import('node:fs/promises').then(fs => fs.mkdtemp(join(root, 'releases/.stage-')))
      try {
        await cp(join(request.artifact, 'payload'), join(staging, 'payload'), { recursive: true, verbatimSymlinks: true })
        await copyFile(join(request.artifact, 'artifact.json'), join(staging, 'artifact.json'))
        await verifyArtifact(staging)
        await rename(staging, destination)
      } finally { await rm(staging, { recursive: true, force: true }) }
    }
    temporaryLink = join(root, '.current-' + transaction)
    await symlink('releases/' + manifest.releaseId + '-' + manifest.target, temporaryLink)
    await rename(temporaryLink, current)
    temporaryLink = undefined
    await writeFile(join(backup, 'selected.json'), JSON.stringify({ ...result, backup, status: 'program-selected-service-stopped' }, null, 2) + '\n', { flag: 'wx', mode: 0o600 })
    return { ...result, backup }
  } catch (error) {
    // Current may already be selected if receipt persistence failed; never claim an automatic rollback.
    if (error instanceof ReleaseError) throw error
    throw new ReleaseError('deployment-incomplete', 'Deployment did not complete; inspect current and private backup receipts before retrying; the service remains stopped')
  } finally {
    try { if (temporaryLink) await rm(temporaryLink, { force: true }) }
    finally {
      const release = async (handle: typeof guard, path: string) => {
        try { await handle.close() } finally { await unlink(path) }
      }
      const cleanups = await Promise.allSettled([release(guard, guardPath), release(rootGuard, rootGuardPath)])
      if (cleanups.some(row => row.status === 'rejected')) throw new ReleaseError('deployment-incomplete', 'Deployment guard cleanup failed; inspect current, receipts and guard files before restarting')
    }
  }
}
