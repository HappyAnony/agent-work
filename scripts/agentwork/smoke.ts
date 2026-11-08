/** Boot the packaged named profile with isolated fixture data and verify authenticated served resources. */
import { createHash } from 'node:crypto'
import { execa } from 'execa'
import { cp, mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { z } from 'zod'
import { ReleaseError, verifyArtifact } from './model.ts'
import type { Execution } from './process.ts'

/**
 * Check a real standalone server without touching a user's home, library or live service.
 * @param input - Verified native server artifact.
 * @param fixture - Disposable portable source fixture to copy into the isolated home.
 * @param execution - Cancellation and process duration budget.
 * @returns Authenticated boot and served-byte evidence; no tokens, cookies or fixture content.
 */
export async function smokeServer(input: string, fixture: string, execution: Execution) {
  const manifest = await verifyArtifact(input)
  if (!manifest.target.startsWith('server-') || manifest.platform !== process.platform || manifest.arch !== process.arch) throw new ReleaseError('wrong-platform', 'Run server acceptance on its native OS and architecture')
  const root = await mkdtemp(join(tmpdir(), 'agentwork-server-smoke-'))
  try {
    const home = join(root, 'home'), library = join(root, 'library'), workspace = join(root, 'workspace')
    await cp(fixture, library, { recursive: true })
    await mkdir(home); await mkdir(workspace)
    const config = join(root, 'server.json')
    await writeFile(config, JSON.stringify({ home, library, workspace, host: '127.0.0.1', port: 0 }), { mode: 0o600 })
    const runtime = join(input, 'payload/server')
    const child = execa(join(runtime, 'bin/node'), [join(runtime, 'start.mjs'), '--config', config], {
      cwd: root, stdout: 'pipe', stderr: 'pipe', reject: false, timeout: execution.timeoutMs,
      killDescendants: true, forceKillAfterDelay: 10_000, ...execution.signal ? { cancelSignal: execution.signal } : {},
      env: { ...execution.env, DSH_TELEMETRY_DISABLED: '1' },
    })
    // A rejected transport still has an observer while readiness is being awaited.
    const completion = child.then(value => value, (error: unknown) => error)
    try {
      const launch = await new Promise<string>((accept, reject) => {
        let text = ''
        child.stdout.setEncoding('utf8')
        const data = (part: string) => {
          text = (text + part).slice(-65_536)
          const ready = /^dsh web: (http:\/\/127\.0\.0\.1:\d+\/\S*)$/mu.exec(text)
          const address = ready?.[1]
          if (address) { child.stdout.off('data', data); child.stdout.resume(); accept(address) }
        }
        child.stdout.on('data', data)
        void completion.then(() => {
          reject(new ReleaseError('runtime-smoke', 'Packaged profile exited before readiness; inspect its configuration and run a private local service log for diagnosis'))
        })
      })
      const url = new URL(launch)
      const base = url.origin
      const exchange = await fetch(url, { redirect: 'manual', signal: AbortSignal.timeout(execution.timeoutMs) })
      const cookie = exchange.headers.getSetCookie().map(value => value.split(';')[0]).join('; ')
      if (exchange.status !== 303 || !cookie) throw new ReleaseError('runtime-smoke', 'Packaged server did not complete its authenticated browser session exchange')
      const bootstrapResponse = await fetch(base + '/api/client/bootstrap', { headers: { cookie, origin: base }, signal: AbortSignal.timeout(execution.timeoutMs) })
      if (!bootstrapResponse.ok) throw new ReleaseError('runtime-smoke', 'Packaged server bootstrap is unavailable after authentication')
      const bootstrap = z.object({ protocolVersion: z.literal(1), frontendHash: z.string(), bundles: z.record(z.string(), z.string()) })
        .parse(await bootstrapResponse.json())
      if (!bootstrap.bundles['@deepseek-ai/dsh-wikiman']) throw new ReleaseError('runtime-smoke', 'Standalone profile did not activate WikiMan')
      const dist = 'payload/server/node_modules/@deepseek-ai/dsh-web-frontend/dist/'
      const files = new Map(manifest.files.map(row => [row.path, row]))
      const index = files.get(dist + 'index.html')
      if (index?.kind !== 'file' || index.sha256 !== bootstrap.frontendHash) throw new ReleaseError('runtime-smoke', 'Server bootstrap reports different frontend bytes')
      for (const [name, hash] of Object.entries(bootstrap.bundles)) {
        const file = files.get('payload/server/node_modules/' + name + '/lib/client.js')
        if (file?.kind !== 'file' || file.sha256 !== hash) throw new ReleaseError('runtime-smoke', 'Active client module differs from the packaged inventory: ' + name)
      }
      let checked = 0
      for (const file of manifest.files) {
        if (file.kind !== 'file' || !file.path.startsWith(dist) || file.path === dist + 'index.html') continue
        const path = file.path.slice(dist.length).split('/').map(encodeURIComponent).join('/')
        const response = await fetch(base + '/' + path, { headers: { cookie }, signal: AbortSignal.timeout(execution.timeoutMs) })
        if (!response.ok || createHash('sha256').update(Buffer.from(await response.arrayBuffer())).digest('hex') !== file.sha256) throw new ReleaseError('runtime-smoke', 'Served Web resource differs from the selected artifact: ' + path)
        checked++
      }
      return { target: manifest.target, commit: manifest.commit, authenticated: true, wikiPluginActive: true, webFilesChecked: checked,
        frontendHash: bootstrap.frontendHash, activePluginCount: Object.keys(bootstrap.bundles).length }
    } finally { child.kill(); await completion }
  } finally { await rm(root, { recursive: true, force: true }) }
}
