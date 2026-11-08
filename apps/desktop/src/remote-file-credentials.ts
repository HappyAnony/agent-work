/** Mac connection credentials stored in owner-only files without operating-system keychain access. */
import { constants } from 'node:fs'
import { lstat, mkdir, open } from 'node:fs/promises'
import { join } from 'node:path'
import { z } from 'zod'
import { withFileLock, writeFileAtomic } from '@deepseek-ai/dsh-atomic-write'
import { remoteServerOrigin } from './remote-frontend.ts'

const settingsSchema = z.object({ schemaVersion: z.literal(1), serverUrl: z.string() }).strict()
const credentialSchema = z.object({ schemaVersion: z.literal(1), serverUrl: z.string(),
  cookie: z.string().min(1).max(8192).regex(/^[^=;\s]+=[^;,\s]+$/u).refine(value => !/[\x00-\x1f\x7f]/u.test(value)),
  expiresAt: z.number().int().positive().max(Number.MAX_SAFE_INTEGER),
}).strict()
const legacySchema = z.object({ serverUrl: z.string().optional(), selectedUrl: z.string().optional() })
const maximumFileBytes = 32 * 1024

/** Host-issued session cookie and its absolute lifetime; never the bootstrap access token. */
export type RemoteFileCredential = z.infer<typeof credentialSchema>

/** File failures contain no stored values or parser excerpts. */
export class RemoteCredentialFileError extends Error {
  /** @param code - Failed local operation. @param cause - Filesystem diagnostic, never credential parser input. */
  constructor(readonly code: 'permissions' | 'format' | 'io' | 'selection', cause?: unknown) {
    super('remote-credential-file-' + code, { cause })
  }
}

/**
 * Read one bounded JSON file without following links or accepting shared credentials.
 * @param path - Owner-selected configuration file.
 * @param privateFile - Whether POSIX ownership and permissions are required.
 * @returns Parsed JSON, or undefined only when the file does not exist.
 */
async function readJson(path: string, privateFile: boolean): Promise<unknown> {
  let file
  try { file = await open(path, constants.O_RDONLY | constants.O_NOFOLLOW) }
  catch (error) {
    const code = (error as NodeJS.ErrnoException).code
    if (code === 'ENOENT') return undefined
    throw new RemoteCredentialFileError(code === 'ELOOP' ? 'permissions' : 'io', error)
  }
  try {
    const info = await file.stat()
    if (!info.isFile() || info.nlink !== 1 || privateFile &&
      (info.uid !== process.getuid?.() || (info.mode & 0o777) !== 0o600)) throw new RemoteCredentialFileError('permissions')
    const bytes = Buffer.alloc(maximumFileBytes + 1)
    let bytesRead = 0
    for (;;) {
      const chunk = await file.read(bytes, bytesRead, bytes.length - bytesRead, null)
      bytesRead += chunk.bytesRead
      if (bytesRead > maximumFileBytes) throw new RemoteCredentialFileError('format')
      if (chunk.bytesRead === 0) break
    }
    try { return JSON.parse(bytes.subarray(0, bytesRead).toString('utf8')) }
    catch (_error) { throw new RemoteCredentialFileError('format') }
  } finally { await file.close() }
}

/**
 * Parse the owned Host's cookie response, retaining the server's expiry instead of a local default.
 * @param serverUrl - Validated server origin.
 * @param header - One authentication Set-Cookie response.
 * @param now - Time at receipt, for the Max-Age attribute.
 * @returns A validated file credential, rejecting session-only or expired cookies.
 */
export function remoteFileCredential(serverUrl: string, header: string, now: number): RemoteFileCredential {
  const age = /;\s*Max-Age=([^;]+)/iu.exec(header)?.[1]
  const expires = /;\s*Expires=([^;]+)/iu.exec(header)?.[1]
  const seconds = age === undefined ? undefined : /^\d+$/u.test(age) ? Number(age) : Number.NaN
  const expiresAt = seconds === undefined ? Date.parse(expires ?? '') : now + seconds * 1000
  const parsed = credentialSchema.safeParse({ schemaVersion: 1, serverUrl: remoteServerOrigin(serverUrl),
    cookie: header.split(';')[0], expiresAt })
  if (!parsed.success || expiresAt <= now) throw new RemoteCredentialFileError('format')
  return parsed.data
}

/** Owner-only, origin-bound Mac settings; serialized selection prevents a late migration replacing a new address. */
export class RemoteFileCredentials {
  private readonly settings: string
  private readonly credentials: string

  /** @param directory - Private client directory. @param legacySettings - Old Electron settings, used only for address migration. */
  constructor(private readonly directory: string, private readonly legacySettings: string) {
    this.settings = join(directory, 'connection.json')
    this.credentials = join(directory, 'credentials.json')
  }

  private async secureDirectory(): Promise<void> {
    await mkdir(this.directory, { recursive: true, mode: 0o700 })
    const info = await lstat(this.directory)
    if (!info.isDirectory() || info.isSymbolicLink() || info.uid !== process.getuid?.() ||
      (info.mode & 0o777) !== 0o700) throw new RemoteCredentialFileError('permissions')
  }

  private async readSelection(): Promise<string | undefined> {
    const value = await readJson(this.settings, true)
    if (value === undefined) return undefined
    const parsed = settingsSchema.safeParse(value)
    if (!parsed.success) throw new RemoteCredentialFileError('format')
    try { return remoteServerOrigin(parsed.data.serverUrl) }
    catch (_error) { throw new RemoteCredentialFileError('format') }
  }

  private async readCredential(): Promise<RemoteFileCredential | undefined> {
    const value = await readJson(this.credentials, true)
    if (value === undefined) return undefined
    const parsed = credentialSchema.safeParse(value)
    if (!parsed.success) throw new RemoteCredentialFileError('format')
    try { remoteServerOrigin(parsed.data.serverUrl) }
    catch (_error) { throw new RemoteCredentialFileError('format') }
    return parsed.data
  }

  /** @returns Saved address, or an address-only migration; encrypted legacy credentials are never decrypted. */
  async readAddress(): Promise<string> {
    return this.contain(async () => {
      await this.secureDirectory()
      return withFileLock(this.settings, async () => {
        const selected = await this.readSelection()
        if (selected !== undefined) return selected
        const legacy = await readJson(this.legacySettings, false)
        if (legacy === undefined) return ''
        const parsed = legacySchema.safeParse(legacy)
        if (!parsed.success) throw new RemoteCredentialFileError('format')
        const address = parsed.data.selectedUrl ?? parsed.data.serverUrl
        if (!address) return ''
        let origin: string
        try { origin = remoteServerOrigin(address) }
        catch (_error) { throw new RemoteCredentialFileError('format') }
        await writeFileAtomic(this.settings, JSON.stringify({ schemaVersion: 1, serverUrl: origin }) + '\n', { mode: 0o600 })
        return origin
      })
    })
  }

  /** @param serverUrl - Explicit selected origin. @returns Completion after the selection is atomically saved. */
  async select(serverUrl: string): Promise<void> {
    const origin = remoteServerOrigin(serverUrl)
    await this.contain(async () => {
      await this.secureDirectory()
      await withFileLock(this.settings, async () => {
        await this.readSelection()
        await writeFileAtomic(this.settings, JSON.stringify({ schemaVersion: 1, serverUrl: origin }) + '\n', { mode: 0o600 })
      })
    })
  }

  /** @param serverUrl - Requested origin. @param now - Current time. @returns Its unexpired credential only, without switching servers. */
  async credential(serverUrl: string, now: number): Promise<RemoteFileCredential | undefined> {
    const origin = remoteServerOrigin(serverUrl)
    return this.contain(async () => {
      await this.secureDirectory()
      const value = await this.readCredential()
      return value?.serverUrl === origin && value.expiresAt > now ? value : undefined
    })
  }

  /** @param credential - Validated, authenticated cookie. @returns Completion before the connection may become active. */
  async remember(credential: RemoteFileCredential): Promise<void> {
    await this.contain(async () => {
      await this.secureDirectory()
      await withFileLock(this.settings, async () => {
        if (await this.readSelection() !== credential.serverUrl) throw new RemoteCredentialFileError('selection')
        await this.readCredential()
        await writeFileAtomic(this.credentials, JSON.stringify(credential) + '\n', { mode: 0o600 })
      })
    })
  }

  private async contain<T>(operation: () => Promise<T>): Promise<T> {
    try { return await operation() }
    catch (error) {
      if (error instanceof RemoteCredentialFileError) throw error
      throw new RemoteCredentialFileError('io', error)
    }
  }
}
