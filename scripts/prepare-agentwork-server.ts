/** Prepare a separate DSH profile for an existing portable library; application startup remains owned by dsh. */
import { spawn } from 'node:child_process'
import { access, mkdir, readFile, writeFile } from 'node:fs/promises'
import { constants } from 'node:fs'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parseArgs } from 'node:util'

const repository = fileURLToPath(new URL('..', import.meta.url))
const { values } = parseArgs({ options: {
  home: { type: 'string' }, store: { type: 'string' }, workspace: { type: 'string' },
  runtime: { type: 'string' }, profile: { type: 'string', default: 'agentwork-server' },
} })
if (!values.home || !values.store || !values.workspace || !/^[a-z][a-z0-9-]+$/u.test(values.profile)) {
  throw new Error('Use --home, --store and --workspace; --profile accepts a lowercase profile name')
}
if (['desktop', 'web', 'headless', 'sdk', 'acp', 'sdk-minimal'].includes(values.profile)) throw new Error('Use a new custom profile name')
const home = resolve(values.home)
const sourceRoot = resolve(values.store)
const workspaceRoot = resolve(values.workspace)
const runtime = resolve(values.runtime ?? join(repository, 'native/wikiman/out'))
await access(join(runtime, 'bin/wiki-kernel'), constants.X_OK)
await access(join(runtime, 'wiki-app/stage/build/portable/boot.js'))
JSON.parse(await readFile(join(sourceRoot, 'store.json'), 'utf8'))
await mkdir(workspaceRoot, { recursive: true })
const profile = join(home, 'profiles', values.profile)
try {
  await access(profile)
  throw new Error('Profile already exists; preserve its configuration and choose another --profile')
} catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error }

async function cli(args: string[]): Promise<void> {
  const child = spawn('pnpm', ['dsh', ...args], { cwd: repository, env: { ...process.env, DSH_HOME: home }, stdio: ['ignore', 'ignore', 'pipe'] })
  let diagnostic = ''
  child.stderr.setEncoding('utf8').on('data', (text: string) => { diagnostic = (diagnostic + text).slice(-8192) })
  await new Promise<void>((accept, reject) => {
    child.once('error', reject)
    child.once('close', (code) => { if (code === 0) accept(); else reject(new Error('DSH profile preparation failed: ' + diagnostic)) })
  })
}
await cli(['--profile', values.profile, '--from-default-profile', 'web', '--dump-config'])
await cli(['plugin', '--profile', values.profile, 'add', '--workspace-root', 'link:./packages/experimental/wikiman'])
await writeFile(join(profile, 'cordis.patch.yml'), JSON.stringify([
  { id: 'session-log-deepseek', disabled: true },
  { id: 'directory-picker', name: '@deepseek-ai/dsh-host-directory-picker-browse', config: {} },
  { insert: [{ id: 'wikiman', name: '@deepseek-ai/dsh-wikiman', config: {
    runtime: 'portable', kernelPath: join(runtime, 'bin/wiki-kernel'), appDir: join(runtime, 'wiki-app'), sourceRoot, workspaceRoot,
  } }] },
], null, 2) + '\n', { mode: 0o600 })
process.stdout.write(`Prepared ${values.profile} in ${home}. Launch it with DSH_HOME set to this directory and pnpm dsh --profile ${values.profile}.\n`)
