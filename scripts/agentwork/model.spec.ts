/** Payload mutation and compatibility checks use whole artifacts, not entry-only fingerprints. */
import { chmod, mkdir, readFile, rm, symlink, unlink, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { artifact, nativeHeader, temporary } from './fixture.ts'
import { assertCompatible, hashFile, manifestSchema, seal, verifyArtifact, verifyExecutable } from './model.ts'

describe('complete artifact verification', () => {
  it.each(['changed', 'missing', 'extra'])('rejects %s nested resources', async (mutation) => {
    const root = await temporary()
    const { directory } = await artifact(root)
    const file = join(directory, 'payload/web/index.html')
    if (mutation === 'missing') await rm(file)
    else await writeFile(mutation === 'extra' ? join(directory, 'payload/web/extra.css') : file, mutation)
    await expect(verifyArtifact(directory)).rejects.toMatchObject({ code: 'integrity-mismatch' })
  })

  it('rejects malformed metadata, duplicate paths and escaping paths', async () => {
    const { directory, manifest } = await artifact(await temporary())
    expect(manifestSchema.safeParse({ ...manifest, protocolVersion: 2 }).success).toBe(false)
    expect(manifestSchema.safeParse({ ...manifest, target: 'server-linux-x64', platform: 'darwin' }).success).toBe(false)
    await writeFile(join(directory, 'artifact.json'), JSON.stringify({ ...manifest, files: [...manifest.files, manifest.files[0]] }))
    await expect(verifyArtifact(directory)).rejects.toMatchObject({ code: 'duplicate-file' })
    await writeFile(join(directory, 'artifact.json'), JSON.stringify({ ...manifest, files: [{ ...manifest.files[0], path: '../escape' }] }))
    await expect(verifyArtifact(directory)).rejects.toThrow()
  })

  it('rejects private configuration and real PEM blocks without rejecting parser literals', async () => {
    const { directory } = await artifact(await temporary())
    const file = join(directory, 'payload/.env')
    await writeFile(file, 'MODEL_KEY=fixture')
    await expect(verifyArtifact(directory)).rejects.toMatchObject({ code: 'secret-file' })
    await rm(file)
    await writeFile(join(directory, 'payload/private.pem'), '-----BEGIN PRIVATE KEY-----\n' + 'A'.repeat(64) + '\n-----END PRIVATE KEY-----\n')
    await expect(verifyArtifact(directory)).rejects.toMatchObject({ code: 'secret-file' })
    await rm(join(directory, 'payload/private.pem'))
    await writeFile(join(directory, 'payload/parser.js'), "const header = '-----BEGIN PRIVATE KEY-----'\n")
    await expect(verifyArtifact(directory)).rejects.toMatchObject({ code: 'integrity-mismatch' })
  })

  // Windows filesystems require privileges for symlinks and do not expose POSIX executable modes.
  it.skipIf(process.platform === 'win32')('rejects an escaping link and changed executable mode', async () => {
    const root = await temporary()
    const { directory } = await artifact(root)
    await writeFile(join(root, 'outside'), 'private')
    await symlink(join(root, 'outside'), join(directory, 'payload/link'))
    await expect(verifyArtifact(directory)).rejects.toMatchObject({ code: 'external-link' })
    await rm(join(directory, 'payload/link'))
    await chmod(join(directory, 'payload/web/index.html'), 0o755)
    await expect(verifyArtifact(directory)).rejects.toMatchObject({ code: 'integrity-mismatch' })
  })

  it('checks native headers even when a misleading binary has a valid checksum', async () => {
    const root = await temporary()
    const { directory, manifest } = await artifact(root, 'server-linux-x64')
    await expect(verifyArtifact(directory)).resolves.toMatchObject({ arch: 'x64' })
    await writeFile(join(directory, 'payload/server/bin/node'), nativeHeader('linux', 'arm64'))
    await unlink(join(directory, 'artifact.json'))
    await seal(directory, manifest)
    await expect(verifyArtifact(directory)).rejects.toMatchObject({ code: 'wrong-platform' })
    const file = join(root, 'wrong-cpu')
    await writeFile(file, nativeHeader('linux', 'arm64'))
    await expect(verifyExecutable(file, 'linux', 'x64')).rejects.toMatchObject({ code: 'wrong-platform' })
    await writeFile(file, nativeHeader('darwin', 'arm64'))
    await expect(verifyExecutable(file, 'darwin', 'arm64')).resolves.toBeUndefined()
    await expect(verifyExecutable(file, 'linux', 'arm64')).rejects.toMatchObject({ code: 'wrong-platform' })
  })

  it('binds native targets to exact shared resources, source and persistence versions', async () => {
    const root = await temporary()
    const frontend = await artifact(root)
    const sharedHash = await hashFile(join(frontend.directory, 'artifact.json'))
    const target = await artifact(root, 'server-linux-x64', { sharedHash })
    expect(() => { assertCompatible(frontend.manifest, [target.manifest], sharedHash) }).not.toThrow()
    for (const changed of [{ sharedHash: 'd'.repeat(64) }, { commit: 'd'.repeat(40) }, { sourceHash: 'd'.repeat(64) },
      { lockHash: 'd'.repeat(64) }, { persistenceVersions: { session: 5, storageSqlite: 1, sessionQuerySqlite: 8 } }]) {
      expect(() => { assertCompatible(frontend.manifest, [{ ...target.manifest, ...changed }], sharedHash) }).toThrow()
    }
    expect(await readFile(join(frontend.directory, 'payload/web/index.html'), 'utf8')).toContain('shared')
    await mkdir(join(frontend.directory, 'unexpected'))
    await expect(verifyArtifact(frontend.directory)).rejects.toMatchObject({ code: 'unexpected-file' })
  })
})
