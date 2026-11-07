/** Sync Capacitor resources and preserve the application's fractional deployment target. */
import { spawnSync } from 'node:child_process'
import { readFile, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { join } from 'node:path'
import { verifyPackagedModules } from './verify-frontend.ts'

const app = fileURLToPath(new URL('..', import.meta.url))
const result = spawnSync('pnpm', ['exec', 'cap', 'sync', 'ios'], { cwd: app, stdio: 'inherit' })
if (result.error !== undefined) throw result.error
if (result.status !== 0) throw new Error('Capacitor resource sync failed')
const packageFile = new URL('../ios/App/CapApp-SPM/Package.swift', import.meta.url)
const source = await readFile(packageFile, 'utf8')
if (!source.includes('.iOS(.v18)')) throw new Error('Review the generated Capacitor deployment target')
await writeFile(packageFile, source.replace('.iOS(.v18)', '.iOS("18.4")'))
await verifyPackagedModules(join(app, 'ios/App/App/public'))
