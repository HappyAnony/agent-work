/** Verify DSH modules after Capacitor has rewritten its own plugin resources. */
import { createHash } from 'node:crypto'
import { access, readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { z } from 'zod'

/**
 * Check packaged entry modules and required carrier files before Xcode builds.
 * @param root - Capacitor's copied public directory.
 * @returns Resolves after every recorded module matches its digest.
 */
export async function verifyPackagedModules(root: string): Promise<void> {
  const manifest = z.object({ bundles: z.record(z.string().regex(/^@[a-z0-9._-]+\/[a-z0-9._-]+$/u), z.string().regex(/^[a-f0-9]{64}$/u)) })
    .parse(JSON.parse(await readFile(join(root, 'manifest.json'), 'utf8')))
  for (const [name, digest] of Object.entries(manifest.bundles)) {
    const bytes = await readFile(join(root, '_ios/plugins', name, 'client.js'))
    if (createHash('sha256').update(bytes).digest('hex') !== digest) throw new Error('Packaged iOS module is incomplete: ' + name)
  }
  for (const path of ['bootstrap.json', '_ios/carrier.js', '_ios/original-index.html', 'connect.html', 'connect.js', 'connect.css', 'stage/build/portable/boot.js']) {
    await access(join(root, path))
  }
}
