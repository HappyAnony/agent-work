/** Build-only assembly of platform-independent resources for the remote desktop carrier. */
import { createHash } from 'node:crypto'
import { cp, mkdir, mkdtemp, readFile, readdir, rename, rm, writeFile } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { parseArgs } from 'node:util'
import { z } from 'zod'
import { packagedBootstrap } from './packaged-bootstrap.ts'

const packageSchema = z.object({ name: z.string(),
  dsh: z.object({ client: z.object({ platform: z.string() }).passthrough().optional() }).passthrough().optional(),
  exports: z.record(z.string(), z.unknown()).optional(),
})
const hash = (bytes: Uint8Array) => createHash('sha256').update(bytes).digest('hex')

/**
 * Assemble local Web, plugin and Wiki resources without a backend runtime or personal data.
 * @param repository - Built DSH checkout with native Wiki resources.
 * @param destination - New absolute output directory; existing outputs are preserved.
 * @returns Number of packaged browser entry modules.
 */
export async function prepareRemoteFrontend(repository: string, destination: string): Promise<number> {
  await mkdir(dirname(destination), { recursive: true })
  const staged = await mkdtemp(join(dirname(destination), '.remote-frontend-'))
  try {
    await cp(join(repository, 'apps/web/dist'), join(staged, 'web'), { recursive: true })
    await cp(join(repository, 'native/wikiman/out/wiki-app'), join(staged, 'wiki'), {
      recursive: true, filter: path => !path.endsWith('.map') && !path.includes('/pandoc/'),
    })
    await cp(join(repository, 'native/wikiman/out/notices'), join(staged, 'notices'), { recursive: true })
    const bundles: Record<string, string> = {}
    const directories = new Map<string, string>()
    const packages = join(repository, 'packages')
    for (const group of await readdir(packages, { withFileTypes: true })) {
      if (!group.isDirectory()) continue
      for (const entry of await readdir(join(packages, group.name), { withFileTypes: true })) {
        if (!entry.isDirectory()) continue
        const directory = join(packages, group.name, entry.name)
        let text: string
        try { text = await readFile(join(directory, 'package.json'), 'utf8') }
        catch (error) { if ((error as NodeJS.ErrnoException).code === 'ENOENT') continue; throw error }
        const pkg = packageSchema.parse(JSON.parse(text))
        if (pkg.dsh?.client?.platform !== 'web') continue
        const client = pkg.exports?.['./client']
        const exported = typeof client === 'string' ? client : typeof client === 'object' && client !== null && 'default' in client ? client.default : undefined
        if (typeof exported !== 'string' || !exported.startsWith('./lib/')) throw new Error('Unsupported client export: ' + pkg.name)
        const output = join(staged, 'plugins', pkg.name)
        await mkdir(output, { recursive: true })
        const bytes = await readFile(join(directory, exported))
        bundles[pkg.name] = hash(bytes)
        directories.set(pkg.name, directory)
        await writeFile(join(output, 'client.js'), bytes)
        for (const file of await readdir(join(directory, 'lib'), { withFileTypes: true })) {
          if (file.isFile() && /^client\..+\.js$/u.test(file.name)) await cp(join(directory, 'lib', file.name), join(output, file.name))
          if (file.isDirectory() && file.name === 'assets') await cp(join(directory, 'lib/assets'), join(output, 'assets'), { recursive: true })
        }
      }
    }
    const manifest = { protocolVersion: 1,
      frontendHash: hash(await readFile(join(staged, 'web/index.html'))), bundles,
      wikiHash: hash(await readFile(join(staged, 'wiki/stage/build/portable/boot.js'))),
    }
    await writeFile(join(staged, 'manifest.json'), JSON.stringify(manifest, null, 2) + '\n')
    await writeFile(join(staged, 'bootstrap.json'), JSON.stringify({ ...manifest, ...await packagedBootstrap(repository, directories, bundles) }) + '\n')
    await rename(staged, destination)
    return Object.keys(bundles).length
  } finally { await rm(staged, { recursive: true, force: true }) }
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const { values } = parseArgs({ options: { out: { type: 'string' } } })
  if (!values.out) throw new Error('Use --out with a new output directory')
  const repository = fileURLToPath(new URL('../../..', import.meta.url))
  const count = await prepareRemoteFrontend(repository, resolve(values.out))
  process.stdout.write(`Packaged ${count} client modules at ${resolve(values.out)}\n`)
}
