/**
 * Assemble verified iOS resources without backend executables or credentials.
 * Shared-artifact imports do not load the source frontend compiler.
 */
import { cp, mkdir, mkdtemp, rename, rm } from 'node:fs/promises'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parseArgs } from 'node:util'
import { build } from 'tsdown'

const repository = fileURLToPath(new URL('../../..', import.meta.url))
const { values } = parseArgs({ options: { frontend: { type: 'string' } } })
const app = join(repository, 'apps/ios')
const temporary = await mkdtemp(join(app, '.prepare-'))
try {
  const inventory = join(temporary, 'inventory')
  let count: number
  if (values.frontend) {
    const { verifyArtifact } = await import('../../../scripts/agentwork/model.ts')
    const source = resolve(values.frontend)
    const manifest = await verifyArtifact(source)
    if (manifest.target !== 'frontend') throw new Error('Use a verified shared frontend artifact')
    await cp(join(source, 'payload'), inventory, { recursive: true })
    count = manifest.files.filter(row => /^payload\/plugins\/@[^/]+\/[^/]+\/client\.js$/u.test(row.path)).length
  } else {
    const { prepareRemoteFrontend } = await import('../../desktop/scripts/prepare-remote-frontend.ts')
    count = await prepareRemoteFrontend(repository, inventory)
  }
  const www = join(temporary, 'www')
  await cp(join(inventory, 'web'), www, { recursive: true })
  await mkdir(join(www, '_ios'), { recursive: true })
  await cp(join(www, 'index.html'), join(www, '_ios/original-index.html'))
  for (const name of ['stage', 'appearance']) await cp(join(inventory, 'wiki', name), join(www, name), { recursive: true })
  await cp(join(inventory, 'plugins'), join(www, '_ios/plugins'), { recursive: true })
  for (const name of ['notices', 'manifest.json', 'bootstrap.json']) await cp(join(inventory, name), join(www, name), { recursive: true })
  await cp(join(app, 'renderer'), www, { recursive: true })
  await build({ entry: [join(app, 'src/carrier.ts')], outDir: join(www, '_ios'), format: 'iife', platform: 'browser', target: 'safari18.4', deps: { alwaysBundle: ['@deepseek-ai/dsh-util-crypto', '@deepseek-ai/dsh-brand'] }, config: false, clean: false })
  await build({ entry: [join(app, 'src/connect.ts')], outDir: www, format: 'iife', platform: 'browser', target: 'safari18.4', config: false, clean: false })
  await rename(join(www, '_ios/carrier.iife.js'), join(www, '_ios/carrier.js'))
  await rename(join(www, 'connect.iife.js'), join(www, 'connect.js'))
  const destination = join(app, 'www')
  await rm(destination, { recursive: true, force: true })
  await rename(www, destination)
  process.stdout.write(`Packaged ${count} iOS client modules\n`)
} finally { await rm(temporary, { recursive: true, force: true }) }
