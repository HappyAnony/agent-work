/** Assemble the iOS app's verified local resources without backend executables or credentials. */
import { cp, mkdir, mkdtemp, rename, rm } from 'node:fs/promises'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { build } from 'tsdown'
import { prepareRemoteFrontend } from '../../desktop/scripts/prepare-remote-frontend.ts'

const repository = fileURLToPath(new URL('../../..', import.meta.url))
const app = join(repository, 'apps/ios')
const temporary = await mkdtemp(join(app, '.prepare-'))
try {
  const inventory = join(temporary, 'inventory')
  const count = await prepareRemoteFrontend(repository, inventory)
  const www = join(temporary, 'www')
  await cp(join(inventory, 'web'), www, { recursive: true })
  await mkdir(join(www, '_ios'), { recursive: true })
  await cp(join(www, 'index.html'), join(www, '_ios/original-index.html'))
  for (const name of ['stage', 'appearance']) await cp(join(inventory, 'wiki', name), join(www, name), { recursive: true })
  await cp(join(inventory, 'plugins'), join(www, '_ios/plugins'), { recursive: true })
  for (const name of ['notices', 'manifest.json']) await cp(join(inventory, name), join(www, name), { recursive: true })
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
