/** Remote client packaging consumes local frontend resources independently of the bundled DSH backend. */
import { existsSync } from 'node:fs'
import { createRequire } from 'node:module'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { resolveMacOSSigningEnvironment, resolveMacOSNotarizationEnvironment } from './desktop-release-environment.mjs'

const require = createRequire(import.meta.url)
const electronChecksums = require('electron/checksums.json')

const frontend = process.env.DSH_REMOTE_FRONTEND_DIR
if (!frontend || !existsSync(resolve(frontend, 'manifest.json'))) throw new Error('DSH_REMOTE_FRONTEND_DIR must name prepared remote frontend resources')
if (process.env.DSH_DESKTOP_UNSIGNED !== undefined && !['0', '1'].includes(process.env.DSH_DESKTOP_UNSIGNED)) throw new Error('DSH_DESKTOP_UNSIGNED must be 1 for development, or 0 for configured Developer ID signing')
const unsigned = process.env.DSH_DESKTOP_UNSIGNED === '1'
const signing = unsigned ? undefined : resolveMacOSSigningEnvironment(process.env)
if (!unsigned) resolveMacOSNotarizationEnvironment(process.env)

export default {
  appId: process.env.DSH_DESKTOP_APP_ID ?? 'local.agentwork.remote',
  productName: 'AgentWork',
  electronDownload: { checksums: electronChecksums },
  extraMetadata: { dshRemoteClient: true },
  asar: true,
  files: ['lib/main.js', 'lib/preload-remote.cjs', 'package.json'],
  extraResources: [{ from: resolve(frontend), to: 'frontend' }],
  directories: { output: resolve(process.env.DSH_REMOTE_OUTPUT_DIR ?? '.desktop-build/agentwork-remote') },
  artifactName: 'AgentWork-remote-${version}-${os}-${arch}.${ext}',
  mac: {
    icon: fileURLToPath(new URL('../resources/icon-macos.png', import.meta.url)),
    category: 'public.app-category.developer-tools',
    minimumSystemVersion: '15.0',
    identity: unsigned ? '-' : signing.signingIdentity,
    forceCodeSigning: !unsigned,
    hardenedRuntime: true,
    entitlements: fileURLToPath(new URL('./macos-entitlements.plist', import.meta.url)),
    entitlementsInherit: fileURLToPath(new URL('./macos-entitlements.plist', import.meta.url)),
    notarize: !unsigned,
    extendInfo: { CFBundleLocalizations: ['en', 'zh_CN', 'ja', 'es', 'fr', 'de', 'ru', 'pt'] },
    target: unsigned ? ['dmg'] : ['dmg', 'zip'],
  },
  afterPack: async context => {
    const resources = context.packager.getResourcesDir(context.appOutDir)
    const { RemoteFrontend } = await import('../lib/types/remote-frontend.js')
    await (await RemoteFrontend.load(resolve(resources, 'frontend'))).localBootstrap()
  },
}
