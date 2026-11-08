/** Private packaged-server adapter: prepares one named profile and launches the normal dsh CLI. */
import { spawn } from 'node:child_process'
import { readFile, mkdir, writeFile, unlink, open, access } from 'node:fs/promises'
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parseArgs } from 'node:util'
import { z } from 'zod'

const absolute = z.string().refine(isAbsolute, 'Use an absolute data path')
const schema = z.object({
  home: absolute, library: absolute, workspace: absolute,
  host: z.string().min(1).default('127.0.0.1'), port: z.number().int().min(0).max(65535).default(18686),
  publicUrl: z.string().url().optional(), trustedHosts: z.array(z.string().min(1)).default([]),
  tlsCert: absolute.optional(), tlsKey: absolute.optional(),
}).strict().refine(c => Boolean(c.tlsCert) === Boolean(c.tlsKey), 'Configure both tlsCert and tlsKey')
const runtime = dirname(fileURLToPath(import.meta.url))
const release = JSON.parse(await readFile(join(runtime, 'release.json'), 'utf8'))
if (process.platform === 'linux') {
  const actual = process.report.getReport().header.glibcVersionRuntime
  const required = release.tools.glibc
  const parts = value => /^\d+\.\d+$/u.test(value ?? '') ? value.split('.').map(Number) : undefined
  const a = parts(actual), b = parts(required)
  if (!a || !b || a[0] < b[0] || (a[0] === b[0] && a[1] < b[1])) throw new Error('Server requires GNU libc at least ' + required + '; build on a compatible Linux baseline')
}
const { values } = parseArgs({ options: { config: { type: 'string' }, check: { type: 'boolean', default: false } } })
if (!values.config) throw new Error('Use --config with a private server JSON configuration file')
const config = schema.parse(JSON.parse(await readFile(resolve(values.config), 'utf8')))
const data = [config.home, config.library, config.workspace].map(p => resolve(p))
const contains = (a, b) => { const r = relative(a, b); return r === '' || (r !== '..' && !r.startsWith('..' + sep) && !isAbsolute(r)) }
for (const path of data) {
  if (contains(runtime, path) || contains(path, runtime)) throw new Error('Keep data directories separate from the program runtime')
  for (const other of data) if (other !== path && (contains(path, other) || contains(other, path))) throw new Error('Home, library and workspace must be separate directories')
}
if (new Set(data).size !== 3) throw new Error('Home, library and workspace must be distinct')
const store = z.object({ schemaVersion: z.literal(1), layoutVersion: z.literal(1), documentFormat: z.literal('docj'), storeId: z.string().min(1) })
  .passthrough().parse(JSON.parse(await readFile(join(config.library, 'store.json'), 'utf8')))
const cli = join(runtime, 'node_modules/@deepseek-ai/dsh/lib/bin.js')
const env = { ...process.env, DSH_HOME: config.home, DSH_TELEMETRY_DISABLED: '1' }
if (values.check) {
  process.stdout.write(JSON.stringify({ ok: true, profile: 'agentwork-server', storeFormatVersion: store.schemaVersion }) + '\n')
} else {
  await mkdir(config.home, { recursive: true, mode: 0o700 })
  await mkdir(config.workspace, { recursive: true })
  const lockPath = join(config.home, '.agentwork-server.pid')
  try {
    await access(join(config.home, '.agentwork-deploy.guard'))
    throw new Error('Deployment is in progress; wait for its recorded result before starting the service')
  } catch (error) { if (error.code !== 'ENOENT') throw error }
  const lock = await open(lockPath, 'wx', 0o600)
  let child
  let stopped = false
  const stop = () => { stopped = true; if (child) child.kill('SIGTERM') }
  process.on('SIGTERM', stop)
  process.on('SIGINT', stop)
  const invoke = async (args, stdio) => {
    if (stopped) throw new Error('Server startup cancelled')
    child = spawn(process.execPath, [cli, ...args], { cwd: runtime, env, stdio })
    await new Promise((accept, reject) => {
      let error
      child.once('error', e => { error = e })
      child.once('close', (code, signal) => {
        child = undefined
        if (error) reject(error)
        else if (code === 0 || (stopped && signal === 'SIGTERM')) accept()
        else reject(new Error('dsh profile process failed; inspect the private service log'))
      })
    })
  }
  try {
    await lock.writeFile(String(process.pid) + '\n')
    try {
      await access(join(config.home, '.agentwork-deploy.guard'))
      throw new Error('Deployment started during startup; wait before restarting the service')
    } catch (error) { if (error.code !== 'ENOENT') throw error }
    const profile = join(config.home, 'profiles/agentwork-server')
    const owner = join(profile, '.agentwork-packaged-profile')
    let exists = true
    try { await access(profile) }
    catch (error) { if (error.code === 'ENOENT') exists = false; else throw error }
    if (exists) {
      if ((await readFile(owner, 'utf8')).trim() !== '1') throw new Error('Existing profile is not owned by this packaged server; choose a separate home')
    } else {
      await invoke(['--profile', 'agentwork-server', '--from-default-profile', 'web', '--dump-config'], ['ignore', 'ignore', 'inherit'])
      await writeFile(owner, '1\n', { flag: 'wx', mode: 0o600 })
    }
    const overlay = join(config.home, 'agentwork-runtime.patch.yml')
    await writeFile(overlay, JSON.stringify([
      { id: 'session-log-deepseek', disabled: true }, { id: 'ui-settings-account', disabled: true },
      { id: 'ui-settings-models', config: { credentialOnboarding: false } },
      { id: 'directory-picker', name: '@deepseek-ai/dsh-host-directory-picker-browse', config: {} },
      { id: 'client-hmr', disabled: true },
      { insert: [{ id: 'wikiman', name: '@deepseek-ai/dsh-wikiman', config: {
        runtime: 'portable', kernelPath: join(runtime, 'wiki/bin/wiki-kernel'), appDir: join(runtime, 'wiki/wiki-app'),
        sourceRoot: config.library, workspaceRoot: config.workspace,
      } }] },
    ], null, 2) + '\n', { mode: 0o600 })
    const args = ['--profile', 'agentwork-server', '--patch', overlay, '--host', config.host, '--port', String(config.port), '--no-open']
    if (config.publicUrl) args.push('--public-url', config.publicUrl)
    if (config.tlsCert) args.push('--tls-cert', config.tlsCert, '--tls-key', config.tlsKey)
    for (const host of config.trustedHosts) args.push('--trusted-host', host)
    await invoke(args, 'inherit')
  } finally {
    process.off('SIGTERM', stop); process.off('SIGINT', stop)
    await lock.close()
    await unlink(lockPath)
  }
}
