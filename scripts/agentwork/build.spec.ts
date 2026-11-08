/** Source metadata distinguishes clean releases from reproducible development snapshots. */
import { mkdir, writeFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { verifyRuntimeClosure } from '../verify-runtime-closure.ts'
import { expect, it } from 'vitest'
import { buildArtifact, sourceMetadata, type BuildRequest } from './build.ts'
import { temporary } from './fixture.ts'
import { run } from './process.ts'

async function checkout() {
  const root = await temporary()
  const repository = join(root, 'checkout')
  await mkdir(repository)
  const write = async (path: string, content: string) => {
    await mkdir(dirname(join(repository, path)), { recursive: true })
    await writeFile(join(repository, path), content)
  }
  await write('package.json', JSON.stringify({ name: 'fixture', version: '1.0.0-rc.1' }))
  await write('pnpm-lock.yaml', 'lockfileVersion: 9\n')
  for (const [path, symbol, version] of [
    ['packages/core/session/src/types.ts', 'SESSION_FORMAT_VERSION', 4],
    ['packages/storage/storage-sqlite/src/schema.ts', 'STORAGE_SQLITE_SCHEMA_VERSION', 1],
    ['packages/session-query/session-query-sqlite/src/schema.ts', 'SESSION_QUERY_SQLITE_SCHEMA_VERSION', 8],
  ] as const) await write(path, `export const ${symbol} = ${version}\n`)
  const execution = { cwd: repository, timeoutMs: 30_000 }
  await run('git', ['init', '-b', 'develop'], execution, true)
  await run('git', ['add', '.'], execution, true)
  await run('git', ['-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', 'commit', '-m', 'source fixture'], execution, true)
  const request: BuildRequest = { ...execution, repository, out: join(root, 'output'), target: 'frontend', allowDirty: false, signing: 'unsigned-development' }
  return { root, repository, request }
}

it('records clean source, pinned dependencies and persistence versions', async () => {
  const f = await checkout()
  const metadata = await sourceMetadata(f.request)
  expect(metadata).toMatchObject({ dirty: false, version: '1.0.0-rc.1', persistenceVersions: { session: 4, storageSqlite: 1, sessionQuerySqlite: 8 } })
  expect(metadata.lockHash).toHaveLength(64)
})

it('refuses dirty release builds and fingerprints different untracked content', async () => {
  const f = await checkout()
  const file = join(f.repository, 'untracked.txt')
  await writeFile(file, 'first')
  await expect(buildArtifact(f.request)).rejects.toMatchObject({ code: 'dirty-source' })
  const first = await sourceMetadata({ ...f.request, allowDirty: true })
  await writeFile(file, 'second')
  const second = await sourceMetadata({ ...f.request, allowDirty: true })
  expect(second.sourceHash).not.toBe(first.sourceHash)
  expect(second.dirty).toBe(true)
})

it('rejects an output that contains its input before running any build tool', async () => {
  const f = await checkout()
  await expect(buildArtifact({ ...f.request, out: f.root })).rejects.toMatchObject({ code: 'invalid-output' })
})

it('closes the standalone product dependency graph over required workspace peers', async () => {
  const root = fileURLToPath(new URL('../..', import.meta.url))
  expect((await verifyRuntimeClosure(root, 'packaging/agentwork-server/package.json', false)).failures).toEqual([])
})
