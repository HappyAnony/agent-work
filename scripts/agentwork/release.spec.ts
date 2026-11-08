/** Native archive transport and whole-cohort releases preserve bytes and reject incomplete publication inputs. */
import { access, chmod, mkdir, readFile, unlink, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { c } from 'tar'
import { expect, it } from 'vitest'
import { artifact, cohort, temporary } from './fixture.ts'
import { hashFile, seal, verifyArtifact } from './model.ts'
import { assembleRelease, validateCohort, verifyRelease, withPackedArtifact } from './release.ts'
import { collectArtifacts, packArtifact, unpackArtifact } from './transport.ts'

it('assembles and verifies all required native targets and rejects archive tampering', async () => {
  const root = await temporary()
  const inputs = await cohort(root)
  const out = join(root, 'release')
  const result = await assembleRelease(inputs, out, { cwd: root, timeoutMs: 30_000 })
  expect(result.artifacts).toHaveLength(5)
  expect(await verifyRelease(out)).toEqual(result)
  await expect(assembleRelease(inputs, out, { cwd: root, timeoutMs: 30_000 })).rejects.toMatchObject({ code: 'output-exists' })
  await writeFile(join(out, result.artifacts[0]!.file), 'changed')
  await expect(verifyRelease(out)).rejects.toMatchObject({ code: 'integrity-mismatch' })
})

it('refuses missing targets, duplicate targets, dirty builds and mixed source cohorts', async () => {
  const root = await temporary()
  const inputs = await cohort(root)
  const manifests = await Promise.all(inputs.map(verifyArtifact))
  const hash = await hashFile(join(inputs[0]!, 'artifact.json'))
  expect(() => validateCohort(manifests.slice(0, -1), hash, manifests.map(row => row.target))).toThrow(/missing/u)
  expect(() => validateCohort([...manifests, manifests[0]!], hash, ['frontend'])).toThrow(/repeats/u)
  expect(() => validateCohort(manifests.map(row => ({ ...row, dirty: true })), hash, ['frontend'])).toThrow(/uncommitted/u)
  expect(() => validateCohort(manifests.map((row, i) => i === 1 ? { ...row, sourceHash: 'e'.repeat(64) } : row), hash, ['frontend'])).toThrow(/sourceHash/u)
})

it('rejects a sealed native package whose embedded frontend differs from its declared shared package', async () => {
  const root = await temporary()
  const inputs = await cohort(root)
  const mac = inputs[3]!
  const manifest = await verifyArtifact(mac)
  await writeFile(join(mac, 'payload/mac-arm64/AgentWork.app/Contents/Resources/frontend/web/index.html'), 'another frontend')
  const { unlink } = await import('node:fs/promises')
  await unlink(join(mac, 'artifact.json'))
  await seal(mac, manifest)
  await expect(assembleRelease(inputs, join(root, 'release'), { cwd: root, timeoutMs: 30_000 })).rejects.toMatchObject({ code: 'incompatible' })
})

it('preserves artifact bytes through transport and refuses overwriting imports', async () => {
  const root = await temporary()
  const { directory, manifest } = await artifact(root)
  const archive = join(root, 'frontend.tar.gz')
  await packArtifact(directory, archive, { cwd: root, timeoutMs: 30_000 })
  const imported = join(root, 'imported')
  expect(await unpackArtifact(archive, imported)).toEqual(manifest)
  expect(await readFile(join(imported, 'payload/web/index.html'))).toEqual(await readFile(join(directory, 'payload/web/index.html')))
  await expect(unpackArtifact(archive, imported)).rejects.toThrow()
})

// Executable mode is a POSIX runtime property; Windows does not preserve this bit.
it.skipIf(process.platform === 'win32')('retains executable modes in archived payloads', async () => {
  const root = await temporary()
  const { directory, manifest } = await artifact(root)
  await chmod(join(directory, 'payload/web/index.html'), 0o755)
  const { unlink } = await import('node:fs/promises')
  await unlink(join(directory, 'artifact.json'))
  const resealed = await seal(directory, manifest)
  const archive = join(root, 'executable.tar.gz')
  await packArtifact(directory, archive, { cwd: root, timeoutMs: 30_000 })
  expect((await unpackArtifact(archive, join(root, 'imported'))).files).toEqual(resealed.files)
})

it('rejects an escaping archive member before extraction', async () => {
  const root = await temporary()
  await mkdir(join(root, 'input'))
  await writeFile(join(root, 'outside'), 'must not extract')
  const archive = join(root, 'escape.tar.gz')
  await c({ cwd: join(root, 'input'), file: archive, gzip: true, preservePaths: true }, ['../outside'])
  await expect(withPackedArtifact(archive, async () => undefined)).rejects.toMatchObject({ code: 'unsafe-archive' })
})

it('collects exact native archives without rewriting them and rejects incomplete cohorts', async () => {
  const root = await temporary()
  const inputs = await cohort(root)
  const transports = join(root, 'transports')
  await mkdir(transports)
  const hashes = new Map<string, string>()
  for (const input of inputs) {
    const manifest = await verifyArtifact(input)
    const archive = join(transports, manifest.target + '.tar.gz')
    await packArtifact(input, archive, { cwd: root, timeoutMs: 30_000 })
    hashes.set(manifest.target, await hashFile(archive))
  }
  const out = join(root, 'release')
  const release = await collectArtifacts(transports, out, { cwd: root, timeoutMs: 30_000 })
  for (const row of release.artifacts) expect(await hashFile(join(out, row.file))).toBe(hashes.get(row.target))
  await expect(collectArtifacts(transports, out, { cwd: root, timeoutMs: 30_000 })).rejects.toMatchObject({ code: 'output-exists' })
  const { unlink } = await import('node:fs/promises')
  await unlink(join(transports, 'ios-simulator.tar.gz'))
  const incomplete = join(root, 'incomplete')
  await expect(collectArtifacts(transports, incomplete, { cwd: root, timeoutMs: 30_000 })).rejects.toMatchObject({ code: 'missing-target' })
  await expect(access(incomplete)).rejects.toMatchObject({ code: 'ENOENT' })
})

it('finishes extraction and removes its private root before propagating a consumer failure', async () => {
  const root = await temporary()
  const { directory } = await artifact(root)
  const archive = join(root, 'frontend.tar.gz')
  await packArtifact(directory, archive, { cwd: root, timeoutMs: 30_000 })
  let extracted = ''
  const failure = new Error('consumer failed')
  await expect(withPackedArtifact(archive, async (path) => { extracted = path; throw failure })).rejects.toBe(failure)
  expect(extracted).not.toBe('')
  await expect(access(extracted)).rejects.toMatchObject({ code: 'ENOENT' })
})

it('includes both installation guides in the release inventory and rejects missing or modified instructions', async () => {
  const root = await temporary()
  const out = join(root, 'release')
  const release = await assembleRelease(await cohort(root), out, { cwd: root, timeoutMs: 30_000 })
  expect(release.schemaVersion).toBe(2)
  if (release.schemaVersion !== 2) throw new Error('Expected a documented release')
  expect(release.documents.map(row => row.file)).toEqual(['README.md', 'README.en.md'])
  const chinese = await readFile(join(out, 'README.md'), 'utf8')
  expect(chinese).toContain('不能直接安装到 iPhone')
  expect(chinese).toContain('payload/server')
  for (const row of release.artifacts) expect(chinese).toContain(row.file)
  const sums = await readFile(join(out, 'SHA256SUMS'), 'utf8')
  expect(sums).toContain('  README.md\n')
  expect(sums).toContain('  README.en.md\n')
  await writeFile(join(out, 'README.md'), 'unverified instructions')
  await expect(verifyRelease(out)).rejects.toMatchObject({ code: 'integrity-mismatch' })
  await unlink(join(out, 'README.md'))
  await expect(verifyRelease(out)).rejects.toMatchObject({ code: 'unexpected-file' })
})

it('can still verify original release inventories without installation guides', async () => {
  const root = await temporary()
  const out = join(root, 'release')
  const result = await assembleRelease(await cohort(root), out, { cwd: root, timeoutMs: 30_000 })
  const legacy = { schemaVersion: 1, product: result.product, releaseId: result.releaseId, version: result.version,
    commit: result.commit, requiredTargets: result.requiredTargets, artifacts: result.artifacts }
  await unlink(join(out, 'README.md')); await unlink(join(out, 'README.en.md'))
  await writeFile(join(out, 'release.json'), JSON.stringify(legacy) + '\n')
  const sums = [...result.artifacts.map(row => ({ file: row.file, sha256: row.sha256 })),
    { file: 'release.json', sha256: await hashFile(join(out, 'release.json')) }]
    .sort((a, b) => a.file.localeCompare(b.file, 'en')).map(row => `${row.sha256}  ${row.file}\n`).join('')
  await writeFile(join(out, 'SHA256SUMS'), sums)
  expect(await verifyRelease(out)).toEqual(legacy)
})
