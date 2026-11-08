/** Native archive transport and whole-cohort releases preserve bytes and reject incomplete publication inputs. */
import { access, chmod, mkdir, readFile, writeFile } from 'node:fs/promises'
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
