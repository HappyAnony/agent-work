/** Download immutable Electron archives with bounded transport retries and pinned package checksums. */
import { link, mkdir, mkdtemp, readFile, rm } from 'node:fs/promises'
import { isAbsolute, join } from 'node:path'
import { z } from 'zod'
import { hashFile, ReleaseError } from './model.ts'
import { run, type Execution } from './process.ts'

/**
 * Prepare one verified official Electron archive without mutating an existing cache entry.
 * @param repository - Checkout containing the locked Electron npm package.
 * @param arch - Native Mac CPU selected by the build request.
 * @param cache - Private build cache outside the artifact payload.
 * @param execution - Cancellation and duration budget for the download subprocess.
 * @param execute - Owned process transport; tests supply a per-case implementation.
 * @returns Complete cached archive verified against the installed package's SHA-256 list.
 */
export async function prepareElectronArchive(repository: string, arch: 'arm64' | 'x64', cache: string,
  execution: Execution, execute: typeof run = run): Promise<string> {
  if (!isAbsolute(cache)) throw new ReleaseError('arguments', 'AGENTWORK_ELECTRON_CACHE must be an absolute directory')
  const pkg = join(repository, 'apps/desktop/node_modules/electron')
  const { version } = z.object({ version: z.string().regex(/^\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?$/u) })
    .parse(JSON.parse(await readFile(join(pkg, 'package.json'), 'utf8')))
  const checksums = z.record(z.string(), z.string().regex(/^[a-f0-9]{64}$/u)).parse(JSON.parse(await readFile(join(pkg, 'checksums.json'), 'utf8')))
  const name = `electron-v${version}-darwin-${arch}.zip`
  const expected = checksums[name]
  if (!expected) throw new ReleaseError('electron-integrity', 'The locked Electron package has no checksum for the selected Mac architecture')
  const destination = join(cache, name)
  const verify = async (file: string) => {
    if (await hashFile(file) !== expected) throw new ReleaseError('electron-integrity', 'Electron archive checksum differs from the locked package; inspect the download source and selected cache entry before retrying')
  }
  try { await verify(destination); return destination }
  catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error }
  await mkdir(cache, { recursive: true, mode: 0o700 })
  const temporary = await mkdtemp(join(cache, '.download-'))
  try {
    const archive = join(temporary, name)
    await execute('curl', ['--fail', '--location', '--silent', '--show-error', '--proto', '=https', '--proto-redir', '=https',
      '--retry', '3', '--retry-delay', '2', '--retry-max-time', String(Math.ceil(execution.timeoutMs / 1000)),
      '--connect-timeout', '20', '--max-time', String(Math.ceil(execution.timeoutMs / 1000)),
      '--output', archive, `https://github.com/electron/electron/releases/download/v${version}/${name}`], execution)
    await verify(archive)
    try { await link(archive, destination) }
    catch (error) { if ((error as NodeJS.ErrnoException).code !== 'EEXIST') throw error; await verify(destination) }
    return destination
  } finally { await rm(temporary, { recursive: true, force: true }) }
}
