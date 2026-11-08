/** Private test fixtures allocate independent roots and model real native file headers. */
import { createHash } from 'node:crypto'
import { copyFile, mkdtemp, mkdir, rm, unlink, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { onTestFinished } from 'vitest'
import { hashFile, seal, type Manifest, type Target } from './model.ts'

/**
 * Allocate a per-case filesystem root with registered cleanup.
 * @returns New private directory.
 */
export async function temporary(): Promise<string> {
  const root = await mkdtemp(join(tmpdir(), 'agentwork-release-test-'))
  onTestFinished(() => rm(root, { recursive: true, force: true }))
  return root
}

/**
 * Create a native header representing the expected OS and CPU.
 * @param platform - Native target operating system.
 * @param arch - Native CPU.
 * @returns Minimal header for static compatibility checks, not an executable program.
 */
export function nativeHeader(platform: string, arch: string): Buffer {
  const bytes = Buffer.alloc(512)
  if (platform === 'linux') {
    bytes.set([0x7f, 0x45, 0x4c, 0x46, 2, 1])
    bytes.writeUInt16LE(arch === 'arm64' ? 183 : 62, 18)
  } else { bytes.writeUInt32LE(0xfeedfacf, 0); bytes.writeUInt32LE(arch === 'arm64' ? 0x100000c : 0x1000007, 4) }
  return bytes
}

/**
 * Create a sealed target fixture including native headers where required.
 * @param root - Owned test root.
 * @param target - Artifact target.
 * @param overrides - Selected metadata variations.
 * @returns Artifact directory and parsed manifest.
 */
export async function artifact(root: string, target: Target = 'frontend', overrides: Partial<Omit<Manifest, 'files'>> = {}) {
  const directory = join(root, target)
  const pieces = target.split('-')
  const platform = target.startsWith('server-') ? (pieces[1] === 'linux' ? 'linux' : 'darwin') : target.startsWith('mac-') ? 'darwin' : target.startsWith('ios-') ? 'ios' : 'any'
  const arch = target.startsWith('server-') ? (pieces[2] === 'arm64' ? 'arm64' : 'x64') : target === 'frontend' ? 'any' : target === 'mac-x64' ? 'x64' : 'arm64'
  const signing = target === 'frontend' || target.startsWith('server-') ? 'not-applicable' : target === 'ios-device' ? 'development-device' : 'unsigned-development'
  const metadata: Omit<Manifest, 'files'> = { schemaVersion: 1, product: 'AgentWork', releaseId: '1.0.0-rc.1-test', version: '1.0.0-rc.1',
    commit: 'a'.repeat(40), dirty: false, sourceHash: 'b'.repeat(64), lockHash: 'c'.repeat(64), target, platform, arch, protocolVersion: 1, storeFormatVersion: 1,
    persistenceVersions: { session: 4, storageSqlite: 1, sessionQuerySqlite: 8 }, signing, tools: { node: 'v24.22.2' }, ...overrides }
  await mkdir(join(directory, 'payload'), { recursive: true })
  const write = async (path: string, bytes: string | Uint8Array) => {
    await mkdir(dirname(join(directory, path)), { recursive: true })
    await writeFile(join(directory, path), bytes)
  }
  if (target.startsWith('server-')) {
    await write('payload/server/node_modules/@deepseek-ai/dsh-client-modules/lib/index.js', 'export {}')
    for (const bin of ['bin/node', 'wiki/bin/wiki-cli', 'wiki/bin/wiki-kernel']) await write('payload/server/' + bin, nativeHeader(platform, arch))
    if (platform === 'linux') await write('payload/server/node_modules/@deepseek-ai/node-addon-system-linux-' + arch + '/bin/landlock-run', nativeHeader(platform, arch))
  } else if (target.startsWith('mac-')) await write('payload/' + (arch === 'arm64' ? 'mac-arm64' : 'mac') + '/AgentWork.app/Contents/MacOS/AgentWork', nativeHeader('darwin', arch))
  else if (target.startsWith('ios-')) await write('payload/' + (target === 'ios-device' ? 'AgentWork-device.app' : 'AgentWork-simulator.app') + '/App', nativeHeader('darwin', arch))
  else {
    const index = '<html>shared</html>', wiki = 'wiki', plugin = 'export const fixture = true'
    const hash = (s: string) => createHash('sha256').update(s).digest('hex')
    await write('payload/web/index.html', index)
    await write('payload/wiki/stage/build/portable/boot.js', wiki)
    await write('payload/wiki/appearance/langs/zh_CN.json', '{}')
    await write('payload/plugins/@fixture/plugin/client.js', plugin)
    for (const file of ['LICENSE-AGPL-3.0.txt', 'LICENSE-DSH', 'THIRD_PARTY_NOTICES.md']) await write('payload/notices/' + file, 'fixture license')
    const legacy = { protocolVersion: 1, frontendHash: hash(index), wikiHash: hash(wiki), bundles: { '@fixture/plugin': hash(plugin) } }
    await write('payload/manifest.json', JSON.stringify(legacy))
    await write('payload/bootstrap.json', JSON.stringify({ ...legacy, injections: [], defaultServerUrl: 'http://127.0.0.1:18686' }))
  }
  return { directory, manifest: await seal(directory, metadata) }
}

/**
 * Create a complete same-source cohort for release and publisher cases.
 * @param root - Owned case directory.
 * @returns Target directories in the default supported cohort.
 */
export async function cohort(root: string): Promise<string[]> {
  const frontend = await artifact(root)
  const sharedHash = await hashFile(join(frontend.directory, 'artifact.json'))
  const targets: Target[] = ['server-linux-x64', 'server-darwin-arm64', 'mac-arm64', 'ios-simulator']
  const directories = [frontend.directory]
  for (const target of targets) {
    const native = await artifact(root, target, { sharedHash })
    for (const entry of frontend.manifest.files) {
      const path = entry.path.slice('payload/'.length)
      let relative: string
      if (target.startsWith('server-')) {
        if (['manifest.json', 'bootstrap.json'].includes(path) || path.startsWith('plugins/')) continue
        relative = 'server/' + (path.startsWith('web/') ? 'node_modules/@deepseek-ai/dsh-web-frontend/dist/' + path.slice(4) : path.startsWith('wiki/') ? 'wiki/wiki-app/' + path.slice(5) : path)
      } else if (target === 'mac-arm64') relative = 'mac-arm64/AgentWork.app/Contents/Resources/frontend/' + path
      else relative = 'AgentWork-simulator.app/public/' + (path === 'web/index.html' ? '_ios/original-index.html' : path.startsWith('web/') ? path.slice(4) : path.startsWith('wiki/') ? path.slice(5) : path.startsWith('plugins/') ? '_ios/' + path : path)
      const to = join(native.directory, 'payload', relative)
      await mkdir(dirname(to), { recursive: true })
      await copyFile(join(frontend.directory, entry.path), to)
    }
    await unlink(join(native.directory, 'artifact.json'))
    await seal(native.directory, native.manifest)
    directories.push(native.directory)
  }
  return directories
}
