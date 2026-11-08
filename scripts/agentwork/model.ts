/** Versioned AgentWork artifacts shared by local builds, release assembly and Actions. */
import { createHash } from 'node:crypto'
import { createReadStream } from 'node:fs'
import { pipeline } from 'node:stream/promises'
import { lstat, open, readFile, readdir, readlink, realpath, writeFile } from 'node:fs/promises'
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from 'node:path'
import { z } from 'zod'
import { RemoteFrontend } from '../../apps/desktop/src/remote-frontend.ts'

/** A handling-relevant build or publication failure; the CLI owns reporting and exit status. */
export class ReleaseError extends Error {
  /**
   * Create an actionable failure without including child output or credentials.
   * @param code - Stable machine-readable failure category.
   * @param message - Affected operation and safe next action.
   */
  constructor(readonly code: string, message: string) { super(message); this.name = 'ReleaseError' }
}

const digest = z.string().regex(/^[a-f0-9]{64}$/u)
const safePath = z.string().min(1).refine(p => !isAbsolute(p) && !p.includes('\\') && p.split('/').every(s => s !== '..' && s !== '.' && s !== ''), 'Use a relative artifact path')
/** Targets identify native platform requirements, not the runner that assembles shared resources. */
export const targetSchema = z.enum(['frontend', 'server-linux-x64', 'server-linux-arm64', 'server-darwin-arm64', 'server-darwin-x64', 'mac-arm64', 'mac-x64', 'ios-simulator', 'ios-device'])
/** Validated target name. */
export type Target = z.infer<typeof targetSchema>
/** Complete inventory entry, including contained application framework links. */
export const fileSchema = z.discriminatedUnion('kind', [
  z.object({ kind: z.literal('file'), path: safePath, sha256: digest, bytes: z.number().int().nonnegative(), executable: z.boolean() }).strict(),
  z.object({ kind: z.literal('link'), path: safePath, target: z.string().min(1) }).strict(),
])
/** Release metadata bound to exact source, dependencies and every payload file. */
export const manifestSchema = z.object({
  schemaVersion: z.literal(1), product: z.literal('AgentWork'),
  releaseId: z.string().regex(/^[a-zA-Z0-9][a-zA-Z0-9._-]{0,119}$/u),
  version: z.string().regex(/^\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?$/u),
  commit: z.string().regex(/^[a-f0-9]{40}$/u), dirty: z.boolean(), sourceHash: digest, lockHash: digest,
  target: targetSchema, platform: z.enum(['any', 'linux', 'darwin', 'ios']), arch: z.enum(['any', 'x64', 'arm64']),
  protocolVersion: z.literal(1), storeFormatVersion: z.literal(1),
  persistenceVersions: z.object({
    session: z.number().int().nonnegative(), storageSqlite: z.number().int().nonnegative(),
    sessionQuerySqlite: z.number().int().nonnegative(),
  }).strict(),
  sharedHash: digest.optional(), signing: z.enum(['not-applicable', 'unsigned-development', 'developer-id', 'development-device']),
  tools: z.record(z.string(), z.string()), files: z.array(fileSchema).min(1),
}).strict().superRefine((m, ctx) => {
  const pieces = m.target.split('-')
  const server = m.target.startsWith('server-')
  const mac = m.target.startsWith('mac-')
  const platform = server ? pieces[1] : mac ? 'darwin' : m.target.startsWith('ios-') ? 'ios' : 'any'
  const arch = server ? pieces[2] : mac ? pieces[1] : m.target === 'frontend' ? 'any' : m.target === 'ios-device' ? 'arm64' : m.arch
  if (m.platform !== platform || m.arch !== arch || (m.target === 'ios-simulator' && m.arch === 'any')) ctx.addIssue({ code: 'custom', message: 'Target OS and architecture disagree' })
  if (((server || m.target === 'frontend') && m.signing !== 'not-applicable')
    || (mac && !['unsigned-development', 'developer-id'].includes(m.signing))
    || (m.target === 'ios-simulator' && m.signing !== 'unsigned-development')
    || (m.target === 'ios-device' && m.signing !== 'development-device')) ctx.addIssue({ code: 'custom', message: 'Target signing state is invalid' })
})
/** Parsed artifact manifest. */
export type Manifest = z.infer<typeof manifestSchema>
/** Complete manifest payload entry. */
export type InventoryEntry = z.infer<typeof fileSchema>

/**
 * Hash a file without buffering native application archives in memory.
 * @param path - File to read.
 * @returns SHA-256 hex digest.
 */
export async function hashFile(path: string): Promise<string> {
  const hash = createHash('sha256')
  await pipeline(createReadStream(path), hash)
  return hash.digest('hex')
}

/**
 * Reject an output inside the source tree's tracked directories or overlapping an input.
 * @param root - Directory that must not contain the candidate.
 * @param candidate - Candidate path.
 * @returns Whether the candidate is the root or one of its descendants.
 */
export function within(root: string, candidate: string): boolean {
  const rel = relative(resolve(root), resolve(candidate))
  return rel === '' || (!rel.startsWith('..' + sep) && rel !== '..' && !isAbsolute(rel))
}

/**
 * Resolve existing directory aliases before checking a new output against data paths.
 * @param path - Existing or prospective filesystem path.
 * @returns Canonical path with the nearest existing ancestor resolved, without creating anything.
 */
export async function canonicalPath(path: string): Promise<string> {
  const absolute = resolve(path)
  try { return await realpath(absolute) }
  catch (error) {
    if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error
    if (dirname(absolute) === absolute) throw error
    return join(await canonicalPath(dirname(absolute)), basename(absolute))
  }
}

function forbidden(path: string): boolean {
  return path.split('/').some(part => /^(?:\.git|\.env(?:\..+)?|id_(?:rsa|ed25519)|Signing\.local\.xcconfig)$/u.test(part))
    || /\.(?:p12|pfx|p8|keychain(?:-db)?)$/iu.test(path)
}

/**
 * Inventory the complete payload and reject external links and private key files.
 * @param directory - Artifact root; only its payload subtree is scanned.
 * @returns Sorted file and contained-link records.
 */
export async function inventory(directory: string): Promise<InventoryEntry[]> {
  const artifact = await realpath(directory)
  const root = join(artifact, 'payload')
  const result: InventoryEntry[] = []
  async function walk(at: string): Promise<void> {
    for (const row of await readdir(at, { withFileTypes: true })) {
      const path = join(at, row.name)
      const rel = relative(artifact, path).split(sep).join('/')
      if (forbidden(rel)) throw new ReleaseError('secret-file', 'Refusing private configuration or signing material in artifact: ' + rel)
      if (row.isSymbolicLink()) {
        const target = await readlink(path)
        if (isAbsolute(target) || !within(root, await realpath(path))) throw new ReleaseError('external-link', 'Artifact link leaves its payload: ' + rel)
        result.push({ kind: 'link', path: rel, target })
      } else if (row.isDirectory()) await walk(path)
      else if (row.isFile()) {
        const stat = await lstat(path)
        if (stat.size < 1024 * 1024) {
          const text = await readFile(path, 'utf8')
          if (/^-----BEGIN (?:RSA |EC |OPENSSH |ENCRYPTED )?PRIVATE KEY-----\r?\n[A-Za-z0-9+/=]{32,}/mu.test(text)) throw new ReleaseError('secret-file', 'Private key material is present in artifact: ' + rel)
        }
        result.push({ kind: 'file', path: rel, sha256: await hashFile(path), bytes: stat.size, executable: (stat.mode & 0o111) !== 0 })
      } else throw new ReleaseError('unsupported-file', 'Artifact contains a device or socket: ' + rel)
    }
  }
  await walk(root)
  return result.sort((a, b) => a.path.localeCompare(b.path, 'en'))
}

/**
 * Write a new immutable manifest after all payload files are present.
 * @param directory - New artifact directory.
 * @param metadata - Source and target metadata.
 * @returns Validated complete manifest.
 */
export async function seal(directory: string, metadata: Omit<Manifest, 'files'>): Promise<Manifest> {
  const manifest = manifestSchema.parse({ ...metadata, files: await inventory(directory) })
  await writeFile(join(directory, 'artifact.json'), JSON.stringify(manifest, null, 2) + '\n', { flag: 'wx' })
  return manifest
}

/**
 * Verify a manifest against all payload bytes, including unexpected additions and link targets.
 * @param directory - Artifact root.
 * @returns Verified manifest; invalid or altered artifacts reject without activation.
 */
export async function verifyArtifact(directory: string): Promise<Manifest> {
  if (!(await lstat(directory)).isDirectory() || !(await lstat(join(directory, 'payload'))).isDirectory()
    || !(await lstat(join(directory, 'artifact.json'))).isFile()) throw new ReleaseError('invalid-path', 'Artifact roots and manifest must be real directories and a regular file')
  const rootEntries = (await readdir(directory)).sort()
  if (rootEntries.join(',') !== 'artifact.json,payload') throw new ReleaseError('unexpected-file', 'Artifact root must contain only artifact.json and payload')
  const manifest = manifestSchema.parse(JSON.parse(await readFile(join(directory, 'artifact.json'), 'utf8')))
  if (manifest.files.some(f => !f.path.startsWith('payload/'))) throw new ReleaseError('invalid-path', 'All manifest files must be inside payload')
  if (new Set(manifest.files.map(f => f.path)).size !== manifest.files.length) throw new ReleaseError('duplicate-file', 'Manifest repeats a payload path')
  const actual = await inventory(directory)
  if (JSON.stringify(actual) !== JSON.stringify(manifest.files)) throw new ReleaseError('integrity-mismatch', 'Artifact payload differs from its manifest; rebuild or retrieve the original artifact')
  if (manifest.target === 'frontend') {
    for (const path of ['wiki/stage/build/portable/boot.js', 'wiki/appearance/langs/zh_CN.json', 'notices/LICENSE-AGPL-3.0.txt', 'notices/LICENSE-DSH', 'notices/THIRD_PARTY_NOTICES.md']) {
      if (!(await lstat(join(directory, 'payload', path))).isFile()) throw new ReleaseError('missing-resource', 'Shared frontend is missing required resources: ' + path)
    }
    const root = join(directory, 'payload')
    await (await RemoteFrontend.load(root)).localBootstrap()
    const legacy = z.object({ wikiHash: digest }).parse(JSON.parse(await readFile(join(root, 'manifest.json'), 'utf8')))
    if (legacy.wikiHash !== await hashFile(join(root, 'wiki/stage/build/portable/boot.js'))) throw new ReleaseError('integrity-mismatch', 'Wiki bootstrap digest differs from the packaged client inventory')
  }
  if (manifest.target.startsWith('server-')) {
    const paths = new Set(manifest.files.map(row => row.path))
    for (const file of manifest.files) {
      if (file.kind !== 'file' || !/^payload\/server\/node_modules\/@deepseek-ai\/[^/]+\/package\.json$/u.test(file.path)) continue
      const pkg = z.object({ main: z.string().optional() }).parse(JSON.parse(await readFile(join(directory, file.path), 'utf8')))
      if (pkg.main && /\.[cm]?js$/u.test(pkg.main)) {
        const entry = relative(directory, resolve(directory, dirname(file.path), pkg.main)).split(sep).join('/')
        if (!paths.has(entry)) throw new ReleaseError('missing-resource', 'Server package is missing its Node entry: ' + entry)
      }
    }
    if (!manifest.files.some(row => row.kind === 'file' && row.path === 'payload/server/node_modules/@deepseek-ai/dsh-client-modules/lib/index.js')) {
      throw new ReleaseError('missing-resource', 'Server is missing the Node-side GUI module adapter; build node-adapters before deployment')
    }
    for (const path of ['bin/node', 'wiki/bin/wiki-kernel', 'wiki/bin/wiki-cli']) await verifyExecutable(join(directory, 'payload/server', path), manifest.platform, manifest.arch)
    if (manifest.platform === 'linux') {
      await verifyExecutable(join(directory, 'payload/server/node_modules/@deepseek-ai/node-addon-system-linux-' + manifest.arch + '/bin/landlock-run'), 'linux', manifest.arch)
    }
  } else if (manifest.target.startsWith('mac-')) {
    const folder = manifest.arch === 'arm64' ? 'mac-arm64' : 'mac'
    const macos = join(directory, 'payload', folder, 'AgentWork.app/Contents/MacOS')
    const names = await readdir(macos)
    if (names.length !== 1) throw new ReleaseError('wrong-platform', 'Mac app must contain one main executable')
    const name = names[0]
    if (!name) throw new ReleaseError('wrong-platform', 'Mac app has no main executable')
    await verifyExecutable(join(macos, name), 'darwin', manifest.arch)
  } else if (manifest.platform === 'ios') {
    await verifyExecutable(join(directory, 'payload', manifest.target === 'ios-device' ? 'AgentWork-device.app/App' : 'AgentWork-simulator.app/App'), 'darwin', manifest.arch)
  }
  return manifest
}

/**
 * Check ELF or Mach-O architecture before activating a native artifact on another host.
 * @param path - Main native executable.
 * @param platform - Linux uses ELF; Apple targets use Mach-O.
 * @param arch - Declared CPU architecture.
 * @returns Resolves when the binary includes the declared architecture.
 */
export async function verifyExecutable(path: string, platform: Manifest['platform'], arch: Manifest['arch']): Promise<void> {
  const file = await open(path, 'r')
  const bytes = Buffer.alloc(512)
  try { await file.read(bytes) }
  finally { await file.close() }
  let matches = false
  if (platform === 'linux' && bytes.subarray(0, 4).equals(Buffer.from([0x7f, 0x45, 0x4c, 0x46])) && bytes[4] === 2) {
    const machine = bytes[5] === 1 ? bytes.readUInt16LE(18) : bytes.readUInt16BE(18)
    matches = machine === (arch === 'arm64' ? 183 : 62)
  } else if (platform === 'darwin') {
    const cpu = arch === 'arm64' ? 0x100000c : 0x1000007
    if (bytes.readUInt32LE(0) === 0xfeedfacf) matches = bytes.readUInt32LE(4) === cpu
    else if (bytes.readUInt32BE(0) === 0xfeedfacf) matches = bytes.readUInt32BE(4) === cpu
    else if ([0xcafebabe, 0xcafebabf].includes(bytes.readUInt32BE(0))) {
      const count = bytes.readUInt32BE(4)
      const stride = bytes.readUInt32BE(0) === 0xcafebabf ? 32 : 20
      if (count <= 16) for (let i = 0; i < count; i++) if (bytes.readUInt32BE(8 + i * stride) === cpu) matches = true
    }
  }
  if (!matches) throw new ReleaseError('wrong-platform', 'Native executable does not match declared OS/architecture: ' + path)
}

/**
 * Check that independently built targets consume one exact shared frontend release.
 * @param frontend - Verified shared frontend manifest.
 * @param targets - Verified target manifests.
 * @param sharedHash - Digest of the shared artifact manifest.
 * @returns Resolves when all source, version, protocol, format and shared-resource identities agree.
 */
export function assertCompatible(frontend: Manifest, targets: readonly Omit<Manifest, 'files'>[], sharedHash: string): void {
  if (frontend.target !== 'frontend') throw new ReleaseError('wrong-target', 'Select a frontend artifact as the shared resource source')
  for (const target of targets) {
    for (const key of ['releaseId', 'version', 'commit', 'dirty', 'sourceHash', 'lockHash', 'protocolVersion', 'storeFormatVersion'] as const) {
      if (target[key] !== frontend[key]) throw new ReleaseError('incompatible', 'Artifact differs from shared frontend in ' + key)
    }
    if (target.sharedHash !== sharedHash) throw new ReleaseError('incompatible', 'Target was built with another shared resource package')
    if (JSON.stringify(target.persistenceVersions) !== JSON.stringify(frontend.persistenceVersions)) throw new ReleaseError('incompatible', 'Target persistence versions differ from shared release metadata')
  }
}

/**
 * Compare embedded frontend resources with the same release's shared file inventory.
 * @param frontend - Verified shared frontend manifest.
 * @param target - Verified native artifact manifest.
 * @returns Resolves when every consumed shared file has identical bytes in the native target.
 */
export function assertEmbeddedFrontend(frontend: Manifest, target: Manifest): void {
  const files = new Map(target.files.map(row => [row.path, row]))
  for (const source of frontend.files) {
    const path = source.path.slice('payload/'.length)
    let mapped: string
    if (target.target.startsWith('mac-')) mapped = 'payload/' + (target.arch === 'arm64' ? 'mac-arm64' : 'mac') + '/AgentWork.app/Contents/Resources/frontend/' + path
    else if (target.platform === 'ios') {
      const prefix = 'payload/' + (target.target === 'ios-device' ? 'AgentWork-device.app' : 'AgentWork-simulator.app') + '/public/'
      mapped = prefix + (path === 'web/index.html' ? '_ios/original-index.html' : path.startsWith('web/') ? path.slice(4) : path.startsWith('wiki/') ? path.slice(5)
        : path.startsWith('plugins/') ? '_ios/' + path : path)
    } else if (target.target.startsWith('server-')) {
      const prefix = 'payload/server/'
      if (path === 'manifest.json' || path === 'bootstrap.json') continue
      if (path.startsWith('web/')) mapped = prefix + 'node_modules/@deepseek-ai/dsh-web-frontend/dist/' + path.slice(4)
      else if (path.startsWith('wiki/')) mapped = prefix + 'wiki/wiki-app/' + path.slice(5)
      else if (path.startsWith('plugins/')) {
        const parts = path.split('/')
        const pkg = parts.slice(1, 3).join('/')
        if (!files.has(prefix + 'node_modules/' + pkg + '/package.json')) continue
        mapped = prefix + 'node_modules/' + pkg + '/lib/' + parts.slice(3).join('/')
      } else mapped = prefix + path
    } else throw new ReleaseError('wrong-target', 'Embedded frontend verification requires a native target')
    const embedded = files.get(mapped)
    if (!embedded || embedded.kind !== source.kind || (source.kind === 'file' && (embedded.kind !== 'file' || embedded.sha256 !== source.sha256))
      || (source.kind === 'link' && (embedded.kind !== 'link' || embedded.target !== source.target))) throw new ReleaseError('incompatible', 'Embedded shared resource differs or is missing: ' + mapped)
  }
}
