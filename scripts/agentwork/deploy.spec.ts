/** Deployment fixtures pin read-only preview, stopped-service ownership and recoverable version selection. */
import { mkdir, readFile, readdir, readlink, unlink, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { expect, it } from 'vitest'
import { deployArtifact, deploymentRunner } from './deploy.ts'
import { artifact, temporary } from './fixture.ts'
import { seal, type Target } from './model.ts'
import type { run } from './process.ts'

async function fixture() {
  const root = await temporary()
  const home = join(root, 'home'), library = join(root, 'library'), workspace = join(root, 'workspace')
  for (const path of [home, library, workspace]) await mkdir(path)
  await writeFile(join(library, 'store.json'), JSON.stringify({ storeId: 'fixture' }))
  await writeFile(join(library, 'note.docj'), 'saved knowledge')
  const config = join(root, 'server.json')
  await writeFile(config, JSON.stringify({ home, library, workspace }))
  const target: Target = process.platform === 'linux' ? (process.arch === 'arm64' ? 'server-linux-arm64' : 'server-linux-x64')
    : process.arch === 'arm64' ? 'server-darwin-arm64' : 'server-darwin-x64'
  const server = await artifact(root, target)
  const execute: typeof run = async command => command.endsWith('wiki-cli')
    ? JSON.stringify({ ok: true, result: { sessionLock: 'available', sourceLock: 'readable', recovery: 'clear' } }) : '{}'
  return { root, home, library, workspace, config, server, execute,
    request: { cwd: root, timeoutMs: 30_000, artifact: server.directory, config, root: join(root, 'programs'), dryRun: false, rollback: false } }
}

// Program activation uses POSIX symlinks and native Linux/macOS binaries; Windows is not a deployment target.
it.skipIf(process.platform === 'win32')('previews without creating a program root or changing data', async () => {
  const f = await fixture()
  const before = (await readdir(f.root)).sort()
  expect(await deployArtifact({ ...f.request, dryRun: true }, f.execute)).toMatchObject({ dryRun: true, service: 'stopped' })
  expect((await readdir(f.root)).sort()).toEqual(before)
  expect(await readFile(join(f.library, 'note.docj'), 'utf8')).toBe('saved knowledge')
})

it.skipIf(process.platform === 'win32')('preserves private backups and selects only a verified stopped-service version', async () => {
  const f = await fixture()
  const result = await deployArtifact(f.request, f.execute)
  expect(await readlink(result.current)).toContain(f.server.manifest.releaseId)
  expect(result.backup).toBeDefined()
  expect(await readFile(join(result.backup!, 'library/note.docj'), 'utf8')).toBe('saved knowledge')
  expect(await readFile(join(f.library, 'note.docj'), 'utf8')).toBe('saved knowledge')
  expect(await readdir(f.home)).not.toContain('.agentwork-deploy.guard')
  expect(await deployArtifact({ ...f.request, rollback: true }, f.execute)).toMatchObject({ service: 'stopped' })
})

it.skipIf(process.platform === 'win32')('rejects running services, active Wiki owners and incompatible data formats', async () => {
  const f = await fixture()
  await writeFile(join(f.home, '.agentwork-server.pid'), String(process.pid))
  await expect(deployArtifact(f.request, f.execute)).rejects.toMatchObject({ code: 'service-running' })
  await unlink(join(f.home, '.agentwork-server.pid'))
  const busy: typeof run = async () => JSON.stringify({ ok: true, result: { sessionLock: 'held', sourceLock: 'readable', recovery: 'clear' } })
  await expect(deployArtifact(f.request, busy)).rejects.toMatchObject({ code: 'library-busy' })
  await deployArtifact(f.request, f.execute)
  const nextRoot = join(f.root, 'next')
  await mkdir(nextRoot)
  const next = await artifact(nextRoot, f.server.manifest.target, { releaseId: 'next', persistenceVersions: { session: 5, storageSqlite: 1, sessionQuerySqlite: 8 } })
  await expect(deployArtifact({ ...f.request, artifact: next.directory, rollback: true }, f.execute)).rejects.toMatchObject({ code: 'data-incompatible' })
})

it.skipIf(process.platform === 'win32')('refuses dirty artifacts', async () => {
  const f = await fixture()
  await unlink(join(f.server.directory, 'artifact.json'))
  await seal(f.server.directory, { ...f.server.manifest, dirty: true })
  await expect(deployArtifact(f.request, f.execute)).rejects.toMatchObject({ code: 'dirty-source' })
})

it.skipIf(process.platform === 'win32')('rejects a program root inside the source artifact', async () => {
  const f = await fixture()
  for (const root of [f.server.directory, join(f.server.directory, 'programs')]) {
    await expect(deployArtifact({ ...f.request, root }, f.execute)).rejects.toMatchObject({ code: 'data-overlap' })
  }
})

it('requires an explicitly controlled self-hosted deployment runner and absolute paths', () => {
  expect(deploymentRunner('["self-hosted","agentwork-deploy","linux"]', '/srv/programs', '/private/server.json')).toContain('agentwork-deploy')
  expect(() => deploymentRunner('["ubuntu-24.04"]', '/srv/programs', '/private/server.json')).toThrow()
  expect(() => deploymentRunner('["self-hosted","agentwork-deploy","linux"]', 'programs', '/private/server.json')).toThrow()
})
