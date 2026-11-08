/** Immutable local release assembly; publication consumes only the verified release directory. */
import { mkdir, mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { t, x } from 'tar'
import { z } from 'zod'
import { ReleaseError, assertCompatible, assertEmbeddedFrontend, hashFile, targetSchema, verifyArtifact, within, type Manifest, type Target } from './model.ts'
import { run, type Execution } from './process.ts'

/** Default product cohort; additional native targets are opt-in and remain individually identified. */
export const requiredTargets: readonly Target[] = [
  'frontend', 'server-linux-x64', 'server-darwin-arm64', 'mac-arm64', 'ios-simulator',
]

/** Release inventory contains archive digests and the source identity shared by every target. */
export const releaseSchema = z.object({
  schemaVersion: z.literal(1), product: z.literal('AgentWork'), releaseId: z.string().regex(/^[a-zA-Z0-9][a-zA-Z0-9._-]{0,119}$/u),
  version: z.string().regex(/^\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?$/u), commit: z.string().regex(/^[a-f0-9]{40}$/u),
  requiredTargets: z.array(targetSchema).min(1),
  artifacts: z.array(z.object({ target: targetSchema,
    file: z.string().regex(/^[a-zA-Z0-9][a-zA-Z0-9._-]+\.tar\.gz$/u), sha256: z.string().regex(/^[a-f0-9]{64}$/u),
    manifestHash: z.string().regex(/^[a-f0-9]{64}$/u),
    signing: z.enum(['not-applicable', 'unsigned-development', 'developer-id', 'development-device']) }).strict()).min(1),
}).strict()
/** Parsed release inventory. */
export type Release = z.infer<typeof releaseSchema>

/**
 * Extract only a validated artifact archive into a private temporary directory.
 * @param archive - Tar archive from the local release or a downloaded workflow artifact.
 * @param consume - Receives the verified temporary artifact root, which must not escape this callback.
 * @returns Callback result; temporary files are removed on every exit path.
 */
export async function withPackedArtifact<T>(archive: string, consume: (directory: string, manifest: Manifest) => Promise<T>): Promise<T> {
  const root = await mkdtemp(join(tmpdir(), 'agentwork-verify-'))
  try {
    let size = 0
    let count = 0
    let invalid: ReleaseError | undefined
    const seen = new Set<string>()
    await t({ file: archive, strict: true, onReadEntry(entry) {
      const path = entry.path.replace(/\/$/u, '')
      if (seen.has(path) || path.split('/').some(part => part === '..' || part === '.' || part === '') || path.includes('\\')
        || (path !== 'artifact.json' && path !== 'payload' && !path.startsWith('payload/'))
        || !['File', 'Directory', 'SymbolicLink', 'Link'].includes(entry.type)) invalid ??= new ReleaseError('unsafe-archive', 'Archive has duplicate, unsupported or escaping entries')
      seen.add(path)
      size += entry.size; count++
      if (size > 8 * 1024 ** 3 || count > 1_000_000) invalid ??= new ReleaseError('unsafe-archive', 'Archive exceeds the supported unpacked size or file count')
      if (entry.type === 'SymbolicLink' && (!entry.linkpath || !within(join(root, 'payload'), resolve(root, path, '..', entry.linkpath)))) invalid ??= new ReleaseError('unsafe-archive', 'Archive link leaves its payload')
      if (entry.type === 'Link' && (!entry.linkpath || !within(join(root, 'payload'), resolve(root, entry.linkpath)))) invalid ??= new ReleaseError('unsafe-archive', 'Archive hard link leaves its payload')
    } })
    if (invalid) throw invalid
    await x({ file: archive, cwd: root, strict: true })
    return await consume(root, await verifyArtifact(root))
  } finally { await rm(root, { recursive: true, force: true }) }
}

/**
 * Validate a complete cohort before packaging or publication.
 * @param manifests - Verified target manifests.
 * @param sharedHash - Digest of the frontend artifact manifest.
 * @param required - Targets promised by this release.
 * @returns Shared frontend manifest.
 */
export function validateCohort(manifests: readonly Manifest[], sharedHash: string, required: readonly Target[]): Manifest {
  const frontend = manifests.find(row => row.target === 'frontend')
  if (!frontend) throw new ReleaseError('missing-frontend', 'Release requires its shared frontend artifact')
  if (new Set(manifests.map(row => row.target)).size !== manifests.length) throw new ReleaseError('duplicate-target', 'Release repeats a target')
  if (new Set(required).size !== required.length || required.length === 0) throw new ReleaseError('invalid-targets', 'Declare a nonempty unique target list')
  for (const target of required) if (!manifests.some(row => row.target === target)) throw new ReleaseError('missing-target', 'Release is missing required target: ' + target)
  if (manifests.some(row => row.dirty)) throw new ReleaseError('dirty-source', 'Release refuses development artifacts built from uncommitted source')
  assertCompatible(frontend, manifests.filter(row => row.target !== 'frontend'), sharedHash)
  for (const target of manifests) if (target.target !== 'frontend') assertEmbeddedFrontend(frontend, target)
  return frontend
}

/**
 * Assemble a new release from complete artifacts without publishing or deploying it.
 * @param inputs - Artifact directories, including the shared frontend.
 * @param out - New release directory; existing versions are never overwritten.
 * @param execution - Archive subprocess settings.
 * @param required - Explicit promised targets, defaulting to the supported product cohort.
 * @returns Complete release inventory after checksum verification.
 */
export async function assembleRelease(inputs: readonly string[], out: string, execution: Execution,
  required: readonly Target[] = requiredTargets): Promise<Release> {
  if (inputs.some(path => within(path, out) || within(out, path))) throw new ReleaseError('invalid-output', 'Release output must be separate from artifact inputs')
  const verified = await Promise.all(inputs.map(async directory => ({
    directory, manifest: await verifyArtifact(directory), hash: await hashFile(join(directory, 'artifact.json')),
  })))
  const manifests = verified.map(row => row.manifest)
  const shared = verified.find(row => row.manifest.target === 'frontend')
  if (!shared) throw new ReleaseError('missing-frontend', 'Release requires the shared frontend')
  const sharedHash = shared.hash
  const frontend = validateCohort(manifests, sharedHash, required)
  await mkdir(resolve(out, '..'), { recursive: true })
  try { await mkdir(out) }
  catch (error) { if ((error as NodeJS.ErrnoException).code === 'EEXIST') throw new ReleaseError('output-exists', 'Release directory already exists; verify it or select a new release identifier'); throw error }
  let complete = false
  try {
    const artifacts: Release['artifacts'] = []
    for (const { directory, manifest, hash } of verified) {
      const file = `${frontend.releaseId}-${manifest.target}.tar.gz`
      await run('tar', ['-czf', file, '-C', directory, 'artifact.json', 'payload'], { ...execution, cwd: out, env: { ...execution.env, COPYFILE_DISABLE: '1' } })
      // Inputs cannot change while their archives are being assembled.
      await verifyArtifact(directory)
      const manifestHash = await hashFile(join(directory, 'artifact.json'))
      if (manifestHash !== hash) throw new ReleaseError('source-changed', 'An artifact manifest changed during release assembly')
      artifacts.push({ target: manifest.target, file, sha256: await hashFile(join(out, file)), manifestHash, signing: manifest.signing })
    }
    const release = releaseSchema.parse({ schemaVersion: 1, product: 'AgentWork', releaseId: frontend.releaseId, version: frontend.version, commit: frontend.commit,
      requiredTargets: [...required], artifacts: artifacts.sort((a, b) => a.target.localeCompare(b.target, 'en')) })
    await writeFile(join(out, 'release.json'), JSON.stringify(release, null, 2) + '\n', { flag: 'wx' })
    const sums = [...artifacts.map(row => ({ file: row.file, sha256: row.sha256 })), { file: 'release.json', sha256: await hashFile(join(out, 'release.json')) }]
      .sort((a, b) => a.file.localeCompare(b.file, 'en')).map(row => `${row.sha256}  ${row.file}\n`).join('')
    await writeFile(join(out, 'SHA256SUMS'), sums, { flag: 'wx' })
    await verifyRelease(out)
    complete = true
    return release
  } finally { if (!complete) await rm(out, { recursive: true, force: true }) }
}

/**
 * Verify a release's exact file set, target completeness and archive checksums.
 * @param directory - Release directory; archives are validated before private temporary extraction.
 * @returns Validated release inventory.
 */
export async function verifyRelease(directory: string): Promise<Release> {
  const release = releaseSchema.parse(JSON.parse(await readFile(join(directory, 'release.json'), 'utf8')))
  if (new Set(release.artifacts.map(row => row.target)).size !== release.artifacts.length || new Set(release.artifacts.map(row => row.file)).size !== release.artifacts.length) throw new ReleaseError('duplicate-target', 'Release target and archive names must be unique')
  if (!release.artifacts.some(row => row.target === 'frontend')) throw new ReleaseError('missing-frontend', 'Release must include shared resources')
  for (const target of release.requiredTargets) if (!release.artifacts.some(row => row.target === target)) throw new ReleaseError('missing-target', 'Required release target is missing: ' + target)
  const expected = [...release.artifacts.map(row => row.file), 'release.json', 'SHA256SUMS'].sort()
  if (JSON.stringify((await readdir(directory)).sort()) !== JSON.stringify(expected)) throw new ReleaseError('unexpected-file', 'Release contains missing or unlisted files')
  const sums = []
  const manifests: Manifest[] = []
  let sharedHash = ''
  for (const row of release.artifacts) {
    const actual = await hashFile(join(directory, row.file))
    if (actual !== row.sha256) throw new ReleaseError('integrity-mismatch', 'Release archive checksum failed: ' + row.file)
    sums.push({ file: row.file, sha256: actual })
    await withPackedArtifact(join(directory, row.file), async (root, manifest) => {
      if (manifest.target !== row.target || manifest.signing !== row.signing || manifest.releaseId !== release.releaseId
        || manifest.version !== release.version || manifest.commit !== release.commit
        || await hashFile(join(root, 'artifact.json')) !== row.manifestHash) throw new ReleaseError('incompatible', 'Packed artifact metadata differs from the release: ' + row.file)
      if (manifest.target === 'frontend') sharedHash = row.manifestHash
      manifests.push(manifest)
    })
  }
  sums.push({ file: 'release.json', sha256: await hashFile(join(directory, 'release.json')) })
  const expectedSums = sums.sort((a, b) => a.file.localeCompare(b.file, 'en')).map(row => `${row.sha256}  ${row.file}\n`).join('')
  if (await readFile(join(directory, 'SHA256SUMS'), 'utf8') !== expectedSums) throw new ReleaseError('integrity-mismatch', 'Release checksum list differs from its inventory')
  validateCohort(manifests, sharedHash, release.requiredTargets)
  return release
}
