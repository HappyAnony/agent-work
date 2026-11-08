/** The native package must retain DSH modules independently of Cordova's plugin directory. */
import { createHash } from 'node:crypto'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { expect, it } from 'vitest'
import { verifyPackagedModules } from '../scripts/verify-frontend.ts'

it('retains DSH modules outside the reserved plugin directory and rejects missing or modified resources', async () => {
  const root = await mkdtemp(join(tmpdir(), 'dsh-ios-package-'))
  const module = join(root, '_ios/plugins/@test/plugin/client.js')
  const bytes = 'globalThis.module = true;'
  try {
    await mkdir(dirname(module), { recursive: true })
    for (const name of ['bootstrap.json', '_ios/carrier.js', '_ios/original-index.html', 'connect.html', 'connect.js', 'connect.css', 'stage/build/portable/boot.js']) {
      const path = join(root, name)
      await mkdir(dirname(path), { recursive: true }); await writeFile(path, 'fixture')
    }
    await writeFile(module, bytes)
    await writeFile(join(root, 'manifest.json'), JSON.stringify({ bundles: { '@test/plugin': createHash('sha256').update(bytes).digest('hex') } }))
    await mkdir(join(root, 'plugins'))
    await rm(join(root, 'plugins'), { recursive: true })
    await expect(verifyPackagedModules(root)).resolves.toBeUndefined()
    await writeFile(module, 'modified')
    await expect(verifyPackagedModules(root)).rejects.toThrow('incomplete')
    await rm(module)
    await expect(verifyPackagedModules(root)).rejects.toMatchObject({ code: 'ENOENT' })
    await writeFile(module, bytes)
    await rm(join(root, '_ios/carrier.js'))
    await expect(verifyPackagedModules(root)).rejects.toMatchObject({ code: 'ENOENT' })
  } finally { await rm(root, { recursive: true, force: true }) }
})
