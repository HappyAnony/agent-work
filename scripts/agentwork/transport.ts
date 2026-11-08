/** Workflow artifact transport preserves native executable modes and framework links. */
import { copyFile, cp, mkdir, open, readdir, rm } from 'node:fs/promises'
import { basename, dirname, join } from 'node:path'
import { ReleaseError, hashFile, verifyArtifact, within } from './model.ts'
import { assemblePackedRelease, verifyRelease, withPackedArtifact } from './release.ts'
import type { Target } from './model.ts'
import { run, type Execution } from './process.ts'

/**
 * Archive a verified artifact without losing executable permissions during Actions upload.
 * @param input - Complete artifact directory.
 * @param out - New archive path, outside the artifact.
 * @param execution - Archive subprocess settings.
 * @returns Archive digest and source target.
 */
export async function packArtifact(input: string, out: string, execution: Execution) {
  const manifest = await verifyArtifact(input)
  if (within(input, out)) throw new ReleaseError('invalid-output', 'Keep transport archives outside their input artifact')
  await mkdir(dirname(out), { recursive: true })
  const file = await open(out, 'wx')
  await file.close()
  let complete = false
  try {
    await run('tar', ['-czf', basename(out), '-C', input, 'artifact.json', 'payload'], { ...execution, cwd: dirname(out), env: { ...execution.env, COPYFILE_DISABLE: '1' } })
    await withPackedArtifact(out, async (root) => {
      if (await hashFile(join(root, 'artifact.json')) !== await hashFile(join(input, 'artifact.json'))) throw new ReleaseError('source-changed', 'Artifact changed while archiving it')
    })
    complete = true
    return { target: manifest.target, file: out, sha256: await hashFile(out) }
  } finally { if (!complete) await rm(out, { force: true }) }
}

/**
 * Gather same-run target archives and assemble the complete supported product cohort.
 * @param input - Directory containing only target tar.gz archives.
 * @param out - New release output directory.
 * @param execution - Archive subprocess ownership.
 * @returns Verified complete release inventory.
 */
export async function collectArtifacts(input: string, out: string, execution: Execution) {
  const names = await readdir(input)
  if (!names.length || names.some(name => !/^[a-z0-9-]+\.tar\.gz$/u.test(name))) throw new ReleaseError('invalid-input', 'Collect a directory of named target tar.gz archives only')
  return assemblePackedRelease(names.sort().map(name => join(input, name)), out, execution)
}

/**
 * Select one server or client from a complete release after checking the build run's source commit.
 * @param input - Verified release directory.
 * @param target - Target to import.
 * @param out - New artifact destination.
 * @param expectedCommit - Source selected from the trusted successful build attempt.
 * @returns Imported target manifest.
 */
export async function selectReleaseArtifact(input: string, target: Target, out: string, expectedCommit: string) {
  const release = await verifyRelease(input)
  if (release.commit !== expectedCommit) throw new ReleaseError('incompatible', 'Release source does not match the selected successful build')
  const selected = release.artifacts.find(row => row.target === target)
  if (!selected) throw new ReleaseError('missing-target', 'Selected target is absent from the release')
  return unpackArtifact(join(input, selected.file), out)
}

/**
 * Import a transport archive into a new verified artifact directory.
 * @param input - Archive containing one artifact.
 * @param out - New destination; existing output is preserved.
 * @returns Imported artifact manifest.
 */
export async function unpackArtifact(input: string, out: string) {
  return withPackedArtifact(input, async (root) => {
    await mkdir(dirname(out), { recursive: true })
    await mkdir(out)
    let complete = false
    try {
      await cp(join(root, 'payload'), join(out, 'payload'), { recursive: true, verbatimSymlinks: true })
      await copyFile(join(root, 'artifact.json'), join(out, 'artifact.json'))
      const manifest = await verifyArtifact(out)
      complete = true
      return manifest
    } finally { if (!complete) await rm(out, { recursive: true, force: true }) }
  })
}
