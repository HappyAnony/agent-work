/** Private Mac credentials retain origin/expiry and fail closed on unsafe or unreadable files. */
import { chmod, link, mkdir, mkdtemp, readFile, rm, stat, symlink, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, expect, it, onTestFinished } from 'vitest'
import { RemoteFileCredentials, remoteFileCredential } from '../src/remote-file-credentials.ts'

const now = 1_800_000_000_000
const origin = 'https://agentwork.test'
const issued = () => remoteFileCredential(origin, 'dsh-auth-fixture=v1.fixture.signature; Max-Age=60; HttpOnly', now)

async function fixture() {
  const root = await mkdtemp(join(tmpdir(), 'agentwork-file-credentials-'))
  onTestFinished(() => rm(root, { recursive: true, force: true }))
  const directory = join(root, 'client'), legacy = join(root, 'remote-server.json')
  return { root, directory, legacy, store: new RemoteFileCredentials(directory, legacy) }
}

// The carrier uses this implementation only on macOS; ownership and no-follow flags require POSIX.
describe.skipIf(process.platform === 'win32')('owner-only connection files', () => {
  it('restores a saved connection in a new store without a key or ciphertext file', async () => {
    const f = await fixture()
    expect(await f.store.readAddress()).toBe('')
    await f.store.select(origin)
    await f.store.remember(issued())
    const reopened = new RemoteFileCredentials(f.directory, f.legacy)
    expect(await reopened.readAddress()).toBe(origin)
    expect(await reopened.credential(origin, now + 1000)).toEqual(issued())
    expect((await stat(f.directory)).mode & 0o777).toBe(0o700)
    for (const name of ['connection.json', 'credentials.json']) {
      expect((await stat(join(f.directory, name))).mode & 0o777).toBe(0o600)
    }
  })

  it('migrates only the selected legacy address and preserves the encrypted original', async () => {
    const f = await fixture()
    const legacy = JSON.stringify({ serverUrl: 'https://old.test', selectedUrl: origin, encryptedCookie: 'legacy-ciphertext' })
    await writeFile(f.legacy, legacy)
    expect(await f.store.readAddress()).toBe(origin)
    expect(await f.store.credential(origin, now)).toBeUndefined()
    expect(await readFile(f.legacy, 'utf8')).toBe(legacy)
    expect(await readFile(join(f.directory, 'connection.json'), 'utf8')).not.toContain('legacy-ciphertext')
    await expect(stat(join(f.directory, 'credentials.json'))).rejects.toMatchObject({ code: 'ENOENT' })
    await f.store.select('https://new.test')
    expect(await new RemoteFileCredentials(f.directory, f.legacy).readAddress()).toBe('https://new.test')
  })

  it('never reuses another origin or an expired cookie and rejects a stale connection save', async () => {
    const f = await fixture()
    await f.store.select(origin)
    await f.store.remember(issued())
    expect(await f.store.credential('https://other.test', now)).toBeUndefined()
    expect(await f.store.credential(origin, now + 60_000)).toBeUndefined()
    await f.store.select('https://other.test')
    await expect(f.store.remember(issued())).rejects.toMatchObject({ code: 'selection' })
    expect(await f.store.readAddress()).toBe('https://other.test')
    expect(JSON.parse(await readFile(join(f.directory, 'credentials.json'), 'utf8'))).toMatchObject({ serverUrl: origin })
  })

  it('preserves damaged or newer credentials instead of overwriting them during a new login', async () => {
    const f = await fixture()
    await f.store.select(origin)
    const path = join(f.directory, 'credentials.json')
    for (const contents of ['{"cookie":"private-fixture-value"', JSON.stringify({ ...issued(), schemaVersion: 2 })]) {
      await writeFile(path, contents, { mode: 0o600 })
      await expect(f.store.credential(origin, now)).rejects.toMatchObject({ code: 'format' })
      await expect(f.store.remember(issued())).rejects.toMatchObject({ code: 'format' })
      expect(await readFile(path, 'utf8')).toBe(contents)
    }
  })

  it('rejects shared permissions without exposing credential contents', async () => {
    const f = await fixture()
    await f.store.select(origin)
    await f.store.remember(issued())
    await chmod(join(f.directory, 'credentials.json'), 0o644)
    await expect(f.store.credential(origin, now)).rejects.toMatchObject({ code: 'permissions' })
    await chmod(f.directory, 0o755)
    await expect(f.store.readAddress()).rejects.toMatchObject({ code: 'permissions' })
  })

  it('refuses linked paths and leaves their targets untouched', async () => {
    const f = await fixture()
    const outside = join(f.root, 'outside')
    await mkdir(outside, { mode: 0o700 })
    await symlink(outside, f.directory, 'dir')
    await expect(f.store.select(origin)).rejects.toMatchObject({ code: 'permissions' })
    await rm(f.directory)
    await f.store.select(origin)
    const sentinel = join(outside, 'sentinel.json')
    await writeFile(sentinel, JSON.stringify(issued()), { mode: 0o600 })
    await symlink(sentinel, join(f.directory, 'credentials.json'))
    await expect(f.store.credential(origin, now)).rejects.toMatchObject({ code: 'permissions' })
    await rm(join(f.directory, 'credentials.json'))
    await link(sentinel, join(f.directory, 'credentials.json'))
    await expect(f.store.remember(issued())).rejects.toMatchObject({ code: 'permissions' })
    expect(JSON.parse(await readFile(sentinel, 'utf8'))).toEqual(issued())
  })

  it('does not replace a malformed selection or accept oversized input', async () => {
    const f = await fixture()
    await f.store.select(origin)
    const path = join(f.directory, 'connection.json')
    for (const contents of ['invalid-private-fixture', 'x'.repeat(33 * 1024)]) {
      await writeFile(path, contents, { mode: 0o600 })
      await expect(f.store.select('https://other.test')).rejects.toMatchObject({ code: 'format' })
      expect(await readFile(path, 'utf8')).toBe(contents)
    }
  })
})

it('uses the server lifetime, preferring Max-Age to Expires', () => {
  expect(remoteFileCredential(origin, 'cookie=value; Max-Age=60; Expires=Thu, 01 Jan 1970 00:00:00 GMT', now))
    .toMatchObject({ expiresAt: now + 60_000 })
  expect(remoteFileCredential(origin, 'cookie=value; Expires=' + new Date(now + 60_000).toUTCString(), now))
    .toMatchObject({ expiresAt: now + 60_000 })
})

it.each(['cookie=value', 'cookie=value; Max-Age=0', 'cookie=value; Max-Age=-1', 'cookie=value; Max-Age=invalid',
  'cookie=value; Max-Age=9999999999999999999', 'cookie=value\r\ninjected; Max-Age=60',
  'cookie=; Max-Age=60', 'cookie=value; Expires=invalid'])('rejects an unsafe or nonpersistent cookie: %s', (header) => {
  expect(() => remoteFileCredential(origin, header, now)).toThrow('remote-credential-file-format')
})
