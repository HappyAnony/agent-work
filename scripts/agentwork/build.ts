/** Platform builders consume one verified frontend instead of independently rebuilding its bytes. */
import { createHash } from 'node:crypto'
import { access, chmod, copyFile, cp, mkdir, readFile, readdir, rm, writeFile } from 'node:fs/promises'
import { join, relative, resolve } from 'node:path'
import { z } from 'zod'
import { materializeStagedLinks, deduplicateStagedWorkspacePackages } from '../build-exe-for-python-sdk-staging.ts'
import { verifyStagedDependencies } from '../executable-packaging.ts'
import { verifyRuntimeClosure } from '../verify-runtime-closure.ts'
import { ReleaseError, assertCompatible, assertEmbeddedFrontend, canonicalPath, hashFile, seal, verifyArtifact, within, type Manifest, type Target } from './model.ts'
import { pnpm, run, type Execution } from './process.ts'

/** One explicit build request; official builds require a clean checkout. */
export interface BuildRequest extends Execution {
  /** Repository root. */
  repository: string
  /** New artifact directory; an existing path is never replaced. */
  out: string
  /** Artifact to produce on its native platform. */
  target: Target
  /** Shared artifact required by native targets. */
  frontend?: string
  /** Explicit development-only allowance; release assembly rejects dirty artifacts. */
  allowDirty: boolean
  /** Optional release identifier, otherwise version and source identity determine it. */
  releaseId?: string
  /** Select a development package or configured Developer ID signing. */
  signing: 'unsigned-development' | 'developer-id' | 'development-device'
}

const pkgSchema = z.object({
  name: z.string(), version: z.string(), dependencies: z.record(z.string(), z.string()).optional(),
}).loose()

async function verifyProductVersions(repository: string, version: string, pnpmVersion: string | undefined): Promise<void> {
  const expectedNode = (await readFile(join(repository, '.node-version'), 'utf8')).trim()
  if (process.versions.node !== expectedNode) throw new ReleaseError('toolchain-mismatch', 'Use Node ' + expectedNode + ' for unified product builds')
  const manager = z.object({ packageManager: z.string().regex(/^pnpm@\d+\.\d+\.\d+$/u) }).parse(JSON.parse(await readFile(join(repository, 'package.json'), 'utf8'))).packageManager
  if (pnpmVersion !== manager.slice(5)) throw new ReleaseError('toolchain-mismatch', 'Use the root packageManager version for unified builds')
  const paths = ['apps/cli', 'apps/desktop', 'apps/ios', 'packaging/agentwork-server']
  let wiki = 0
  for (const group of await readdir(join(repository, 'packages'), { withFileTypes: true })) {
    if (!group.isDirectory()) continue
    for (const entry of await readdir(join(repository, 'packages', group.name), { withFileTypes: true })) {
      if (!entry.isDirectory()) continue
      const path = join('packages', group.name, entry.name)
      let text: string
      try { text = await readFile(join(repository, path, 'package.json'), 'utf8') }
      catch (error) { if ((error as NodeJS.ErrnoException).code === 'ENOENT') continue; throw error }
      if (pkgSchema.parse(JSON.parse(text)).name === '@deepseek-ai/dsh-wikiman') { paths.push(path); wiki++ }
    }
  }
  if (wiki !== 1) throw new ReleaseError('version-source', 'Exactly one WikiMan workspace package must supply the product build')
  for (const path of paths) if (pkgSchema.parse(JSON.parse(await readFile(join(repository, path, 'package.json'), 'utf8'))).version !== version) {
    throw new ReleaseError('version-mismatch', path + ' must match the root product version before building')
  }
}

/**
 * Read a source identity that includes uncommitted and untracked changes for development builds.
 * @param request - Checkout and package-manager execution settings.
 * @returns Common metadata used by every artifact target.
 */
export async function sourceMetadata(request: BuildRequest): Promise<Omit<Manifest, 'files'>> {
  const execution = { ...request, cwd: request.repository }
  const commit = await run('git', ['rev-parse', 'HEAD'], execution, true)
  const state = await run('git', ['status', '--porcelain', '--untracked-files=normal'], execution, true)
  const dirty = state !== ''
  if (dirty && !request.allowDirty) throw new ReleaseError('dirty-source', 'Commit or preserve all source changes before an official build; use --allow-dirty only for development')
  const hash = createHash('sha256').update(commit)
  if (dirty) {
    hash.update(await run('git', ['diff', '--binary', 'HEAD'], execution, true))
    const files = await run('git', ['ls-files', '--others', '--exclude-standard', '-z'], execution, true)
    for (const file of files.split('\0').filter(Boolean).sort()) hash.update('\0' + file + '\0').update(await hashFile(join(request.repository, file)))
  }
  const sourceHash = hash.digest('hex')
  const pkg = pkgSchema.parse(JSON.parse(await readFile(join(request.repository, 'package.json'), 'utf8')))
  const tools: Record<string, string> = { node: process.version, pnpm: await pnpm(['--version'], execution, true) }
  const versions: [string, string][] = [
    ['packages/core/session/src/types.ts', 'SESSION_FORMAT_VERSION'],
    ['packages/storage/storage-sqlite/src/schema.ts', 'STORAGE_SQLITE_SCHEMA_VERSION'],
    ['packages/session-query/session-query-sqlite/src/schema.ts', 'SESSION_QUERY_SQLITE_SCHEMA_VERSION'],
  ]
  const dataVersions: number[] = []
  for (const [path, symbol] of versions) {
    const match = new RegExp('export const ' + symbol + ' = (\\d+)', 'u').exec(await readFile(join(request.repository, path), 'utf8'))
    if (!match) throw new ReleaseError('version-source', 'Review persistence version extraction for ' + path)
    dataVersions.push(Number(match[1]))
  }
  const [session, storageSqlite, sessionQuerySqlite] = z.tuple([z.number(), z.number(), z.number()]).parse(dataVersions)
  return {
    schemaVersion: 1, product: 'AgentWork', releaseId: request.releaseId ?? `${pkg.version}-${sourceHash.slice(0, 12)}`,
    version: pkg.version, commit, dirty, sourceHash, lockHash: await hashFile(join(request.repository, 'pnpm-lock.yaml')),
    target: request.target, platform: 'any', arch: 'any', protocolVersion: 1, storeFormatVersion: 1,
    persistenceVersions: { session, storageSqlite, sessionQuerySqlite },
    signing: 'not-applicable', tools,
  }
}

function requiredFrontend(request: BuildRequest): string {
  if (!request.frontend) throw new ReleaseError('missing-frontend', 'Native targets require a verified shared frontend')
  return request.frontend
}

async function sharedFrontend(request: BuildRequest, payload: string): Promise<void> {
  await pnpm(['run', 'build:artifacts'], request)
  await pnpm(['run', 'build:check'], request)
  const native = join(request.repository, 'native/wikiman')
  const app = join(native, 'wiki-app')
  await pnpm(['install', '--ignore-workspace', '--frozen-lockfile', '--ignore-scripts'], { ...request, cwd: app })
  await pnpm(['run', 'build'], { ...request, cwd: app })
  const runtime = join(request.out, '.wiki-resources')
  await mkdir(join(runtime, 'wiki-app'), { recursive: true })
  for (const part of ['stage', 'appearance']) await cp(join(app, part), join(runtime, 'wiki-app', part), { recursive: true })
  await mkdir(join(runtime, 'notices/pollwatch'), { recursive: true })
  await copyFile(join(native, 'LICENSE-AGPL-3.0.txt'), join(runtime, 'notices/LICENSE-AGPL-3.0.txt'))
  const backend = join(native, 'wiki-kernel/internal/pollwatch/backend')
  for (const name of await readdir(backend)) if (name === 'LICENSE' || name.endsWith('.md')) await copyFile(join(backend, name), join(runtime, 'notices/pollwatch', name))
  const { prepareRemoteFrontend } = await import('../../apps/desktop/scripts/prepare-remote-frontend.ts')
  await prepareRemoteFrontend(request.repository, payload, runtime)
  await copyFile(join(request.repository, 'LICENSE'), join(payload, 'notices/LICENSE-DSH'))
  const thirdParty = join(request.repository, 'THIRD_PARTY_NOTICES.md')
  await copyFile(thirdParty, join(payload, 'notices/THIRD_PARTY_NOTICES.md'))
  await rm(runtime, { recursive: true })
}

async function importFrontend(frontend: string, runtime: string): Promise<void> {
  const modules = join(runtime, 'node_modules')
  await rm(join(modules, '@deepseek-ai/dsh-web-frontend/dist'), { recursive: true, force: true })
  await cp(join(frontend, 'payload/web'), join(modules, '@deepseek-ai/dsh-web-frontend/dist'), { recursive: true })
  const plugins = join(frontend, 'payload/plugins')
  for (const scope of await readdir(plugins)) {
    for (const name of await readdir(join(plugins, scope))) {
      const destination = join(modules, scope, name)
      try { await access(join(destination, 'package.json')) }
      catch (error) { if ((error as NodeJS.ErrnoException).code === 'ENOENT') continue; throw error }
      for (const part of await readdir(join(plugins, scope, name))) {
        await cp(join(plugins, scope, name, part), join(destination, 'lib', part), { recursive: true, force: true })
      }
    }
  }
  await cp(join(frontend, 'payload/wiki'), join(runtime, 'wiki/wiki-app'), { recursive: true })
  await cp(join(frontend, 'payload/notices'), join(runtime, 'notices'), { recursive: true })
}

async function server(request: BuildRequest, metadata: Omit<Manifest, 'files'>, payload: string): Promise<void> {
  const [, platform, arch] = request.target.split('-')
  if (platform !== process.platform || arch !== process.arch) throw new ReleaseError('wrong-platform', 'Build this server on its declared OS and architecture')
  metadata.platform = platform === 'linux' ? 'linux' : 'darwin'; metadata.arch = arch === 'arm64' ? 'arm64' : 'x64'
  if (platform === 'linux') metadata.tools.glibc = (await run('getconf', ['GNU_LIBC_VERSION'], request, true)).replace(/^glibc /u, '')
  if (platform === 'darwin') metadata.tools.minimumMacOS = '15.0'
  const execution = platform === 'darwin' ? { ...request, env: { ...request.env, MACOSX_DEPLOYMENT_TARGET: '15.0' } } : request
  const closure = await verifyRuntimeClosure(request.repository, 'packaging/agentwork-server/package.json', false)
  if (closure.failures.length) throw new ReleaseError('runtime-closure', 'Declare every required workspace peer in packaging/agentwork-server/package.json before deployment: ' + closure.failures.join('; '))
  await pnpm(['run', 'build:native-system'], execution)
  await pnpm(['run', 'build:lib:host'], request)
  const runtime = join(payload, 'server')
  await pnpm(['--filter', 'agentwork-server-closure', 'deploy', '--prod', '--config.allow-unused-patches=true', '--config.node-linker=hoisted',
    '--config.pnpmfile=' + join(request.repository, 'scripts/python-runtime-deploy.pnpmfile.mjs'), '--config.link-workspace-packages=true', '--config.hoist-workspace-packages=false', runtime], request)
  await materializeStagedLinks(runtime)
  await deduplicateStagedWorkspacePackages(runtime, runtime)
  await verifyStagedDependencies(runtime)
  for (const name of ['.modules.yaml', '.pnpm-workspace-state-v1.json', '.pnpm/lock.yaml']) await rm(join(runtime, 'node_modules', name), { force: true })
  await writeFile(join(runtime, 'package.json'), JSON.stringify({ name: 'agentwork-server-runtime', private: true, type: 'module', version: metadata.version,
    dependencies: { '@deepseek-ai/dsh': metadata.version, '@deepseek-ai/dsh-wikiman': metadata.version } }, null, 2) + '\n')
  // The generated CLI resolver includes this product's optional plugin; source DSH defaults stay independent.
  const cliManifest = join(runtime, 'node_modules/@deepseek-ai/dsh/package.json')
  const pkg = pkgSchema.parse(JSON.parse(await readFile(cliManifest, 'utf8')))
  pkg.dependencies = { ...pkg.dependencies, '@deepseek-ai/dsh-wikiman': metadata.version }
  await writeFile(cliManifest, JSON.stringify(pkg, null, 2) + '\n')
  await importFrontend(requiredFrontend(request), runtime)
  const native = join(request.repository, 'native/wikiman/wiki-kernel')
  const go = /^go (\S+)$/mu.exec(await readFile(join(native, 'go.mod'), 'utf8'))?.[1]
  if (!go) throw new ReleaseError('toolchain-mismatch', 'Native module must declare a Go toolchain version')
  const nativeExecution = { ...execution, cwd: native, env: { ...execution.env, GOTOOLCHAIN: 'go' + go } }
  metadata.tools.go = await run('go', ['version'], nativeExecution, true)
  await mkdir(join(runtime, 'wiki/bin'), { recursive: true })
  for (const bin of ['wiki-kernel', 'wiki-cli']) await run('go', ['build', '-trimpath', '-ldflags=-s -w', '-tags', 'fts5', '-o', join(runtime, 'wiki/bin', bin), './cmd/' + bin], nativeExecution)
  if (process.platform === 'darwin') await run('xcrun', ['clang', '-Os', '-fobjc-arc', '-framework', 'AppKit', '-framework', 'WebKit', '-framework', 'PDFKit',
    '-mmacosx-version-min=15.0', join(request.repository, 'native/wikiman/wiki-pdf/main.m'), '-o', join(runtime, 'wiki/bin/wiki-pdf')], execution)
  await mkdir(join(runtime, 'bin'), { recursive: true })
  await copyFile(process.execPath, join(runtime, 'bin/node'))
  await chmod(join(runtime, 'bin/node'), 0o755)
  await cp(join(request.repository, 'scripts/agentwork/server-start.mjs'), join(runtime, 'start.mjs'))
  await writeFile(join(runtime, 'release.json'), JSON.stringify(metadata, null, 2) + '\n')
  const version = await run(join(runtime, 'bin/node'), [join(runtime, 'node_modules/@deepseek-ai/dsh/lib/bin.js'), '--version'], { ...request, cwd: runtime }, true)
  if (!version.includes(metadata.version)) throw new ReleaseError('runtime-smoke', 'Packaged dsh does not report the selected release version')
  await run(join(runtime, 'wiki/bin/wiki-kernel'), ['--version'], request, true)
}

async function mac(request: BuildRequest, metadata: Omit<Manifest, 'files'>, payload: string): Promise<void> {
  if (process.platform !== 'darwin') throw new ReleaseError('wrong-platform', 'Build Mac clients on macOS')
  const arch = request.target === 'mac-arm64' ? 'arm64' : 'x64'
  if (arch !== process.arch) throw new ReleaseError('wrong-platform', 'Build the Mac client on its native architecture')
  metadata.platform = 'darwin'; metadata.arch = arch
  metadata.signing = request.signing
  if (request.signing === 'development-device') throw new ReleaseError('signing', 'Use unsigned-development or developer-id for a Mac client')
  if (request.signing === 'developer-id' && !(request.env?.CSC_LINK ?? process.env.CSC_LINK)) throw new ReleaseError('signing', 'Configure Developer ID signing before requesting a signed Mac distribution')
  await pnpm(['run', 'build:lib:host'], request)
  const env = { ...process.env, ...request.env, DSH_REMOTE_FRONTEND_DIR: join(requiredFrontend(request), 'payload'), DSH_REMOTE_OUTPUT_DIR: payload,
    DSH_DESKTOP_UNSIGNED: request.signing === 'unsigned-development' ? '1' : '0',
    ...(request.signing === 'unsigned-development' ? { CSC_IDENTITY_AUTO_DISCOVERY: 'false' } : {}) }
  await pnpm(['--filter', '@deepseek-ai/dsh-desktop', 'exec', 'electron-builder', '--config', 'scripts/electron-builder-remote.config.mjs', '--mac', 'dir', '--' + arch, '--publish', 'never'], { ...request, env })
  // Builder diagnostics contain host paths and are not runtime resources.
  for (const name of ['builder-debug.yml', 'builder-effective-config.yaml']) await rm(join(payload, name), { force: true })
  if (request.signing === 'developer-id') {
    const folder = arch === 'arm64' ? 'mac-arm64' : 'mac'
    const app = join(payload, folder, 'AgentWork.app')
    await run('codesign', ['--verify', '--deep', '--strict', app], request)
    await run('spctl', ['--assess', '--type', 'execute', app], request)
  }
}

async function ios(request: BuildRequest, metadata: Omit<Manifest, 'files'>, payload: string): Promise<void> {
  if (process.platform !== 'darwin') throw new ReleaseError('wrong-platform', 'Build iOS clients on macOS with full Xcode')
  const device = request.target === 'ios-device'
  if (device && (request.signing !== 'development-device' || !(request.env?.AGENTWORK_APPLE_TEAM ?? process.env.AGENTWORK_APPLE_TEAM))) throw new ReleaseError('signing', 'A device build requires --signing development-device and AGENTWORK_APPLE_TEAM; use Xcode for account and device authorization')
  if (!device && request.signing !== 'unsigned-development') throw new ReleaseError('signing', 'Simulator builds use unsigned-development and cannot be installed on an iPhone')
  metadata.platform = 'ios'; metadata.arch = device ? 'arm64' : process.arch === 'arm64' ? 'arm64' : 'x64'; metadata.signing = request.signing
  metadata.tools.xcode = await run('xcodebuild', ['-version'], request, true)
  await pnpm(['run', 'build:lib:host'], request)
  await pnpm(['--filter', '@deepseek-ai/dsh-ios', 'run', 'prepare:frontend', '--frontend', requiredFrontend(request)], request)
  await pnpm(['--filter', '@deepseek-ai/dsh-ios', 'run', 'sync'], request)
  const derived = join(request.out, '.xcode')
  const version = z.string().min(1).parse(metadata.version.split('-')[0])
  await run('xcodebuild', ['-project', join(request.repository, 'apps/ios/ios/App/App.xcodeproj'), '-scheme', 'App', '-configuration', 'Release',
    '-destination', device ? 'generic/platform=iOS' : 'generic/platform=iOS Simulator', '-derivedDataPath', derived,
    'MARKETING_VERSION=' + version, 'CURRENT_PROJECT_VERSION=1', ...device ? ['DEVELOPMENT_TEAM=' + z.string().min(1).parse(request.env?.AGENTWORK_APPLE_TEAM ?? process.env.AGENTWORK_APPLE_TEAM)] : ['CODE_SIGNING_ALLOWED=NO'], 'build'], request)
  const app = join(derived, 'Build/Products', device ? 'Release-iphoneos' : 'Release-iphonesimulator', 'App.app')
  if (device) await run('codesign', ['--verify', '--deep', '--strict', app], request)
  await cp(app, join(payload, device ? 'AgentWork-device.app' : 'AgentWork-simulator.app'), { recursive: true })
  await rm(derived, { recursive: true })
}

/**
 * Build a new artifact, verify source consistency and seal all payload bytes.
 * @param request - Explicit target, frontend, signing and execution settings.
 * @returns Complete manifest; failure removes only the newly reserved output.
 */
export async function buildArtifact(request: BuildRequest): Promise<Manifest> {
  const out = await canonicalPath(request.out)
  const repository = await canonicalPath(request.repository)
  const frontend = request.frontend ? await canonicalPath(request.frontend) : undefined
  if (out === repository || within(out, repository) || (frontend && (within(out, frontend) || within(frontend, out)))) throw new ReleaseError('invalid-output', 'Select an output separate from the checkout and shared input')
  if (within(repository, out)) await run('git', ['check-ignore', '--quiet', relative(repository, out) + '/'], request, true)
  const metadata = await sourceMetadata(request)
  await verifyProductVersions(request.repository, metadata.version, metadata.tools.pnpm)
  if (request.target !== 'frontend') {
    if (!request.frontend) throw new ReleaseError('missing-frontend', 'Native targets require --frontend pointing to a verified shared artifact')
    const shared = await verifyArtifact(request.frontend)
    metadata.sharedHash = await hashFile(join(request.frontend, 'artifact.json'))
    assertCompatible(shared, [metadata], metadata.sharedHash)
  }
  await mkdir(resolve(out, '..'), { recursive: true })
  try { await mkdir(out) }
  catch (error) { if ((error as NodeJS.ErrnoException).code === 'EEXIST') throw new ReleaseError('output-exists', 'Output already exists; select a new artifact directory'); throw error }
  let complete = false
  try {
    const payload = join(out, 'payload')
    if (request.target !== 'frontend') await mkdir(payload)
    if (request.target === 'frontend') await sharedFrontend(request, payload)
    else if (request.target.startsWith('server-')) await server(request, metadata, payload)
    else if (request.target.startsWith('mac-')) await mac(request, metadata, payload)
    else await ios(request, metadata, payload)
    const after = await sourceMetadata(request)
    if (after.sourceHash !== metadata.sourceHash || after.lockHash !== metadata.lockHash) throw new ReleaseError('source-changed', 'Source or dependency lock changed while building; rebuild from one fixed checkout')
    await seal(out, metadata)
    const verified = await verifyArtifact(out)
    if (request.frontend && request.target !== 'frontend') {
      if (await hashFile(join(request.frontend, 'artifact.json')) !== verified.sharedHash) throw new ReleaseError('source-changed', 'Shared resources changed while building a native target')
      assertEmbeddedFrontend(await verifyArtifact(request.frontend), verified)
    }
    complete = true
    return verified
  } finally { if (!complete) await rm(out, { recursive: true, force: true }) }
}
