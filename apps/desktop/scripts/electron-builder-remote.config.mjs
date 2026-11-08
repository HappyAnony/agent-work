/** Remote desktop distribution with local browser resources and no bundled backend runtime. */
import { existsSync } from 'node:fs'
import { resolve } from 'node:path'
import { createElectronBuilderConfig } from './electron-builder-config.mjs'

const frontend = process.env.DSH_REMOTE_FRONTEND_DIR
if (!frontend || !existsSync(resolve(frontend, 'manifest.json'))) throw new Error('DSH_REMOTE_FRONTEND_DIR must name prepared remote frontend resources')
const config = createElectronBuilderConfig({ ...process.env, DSH_DESKTOP_APP_ID: process.env.DSH_DESKTOP_APP_ID ?? 'local.agentwork.remote' })
config.productName = 'AgentWork'
config.extraMetadata = { ...config.extraMetadata, dshRemoteClient: true }
config.files = ['lib/main.js', 'lib/preload-remote.cjs', 'package.json']
config.extraResources = [{ from: resolve(frontend), to: 'frontend' }]
config.afterPack = async context => {
  const resources = context.packager.getResourcesDir(context.appOutDir)
  const { RemoteFrontend } = await import('../lib/types/remote-frontend.js')
  const frontend = await RemoteFrontend.load(resolve(resources, 'frontend'))
  await frontend.localBootstrap()
}
if (process.env.DSH_REMOTE_OUTPUT_DIR) config.directories = { ...config.directories, output: resolve(process.env.DSH_REMOTE_OUTPUT_DIR) }
config.artifactName = 'AgentWork-remote-${version}-${os}-${arch}.${ext}'
export default config
