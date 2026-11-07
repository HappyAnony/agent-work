/** OS trust roots are admitted without disabling TLS identity verification. */
import { execFile, type ChildProcess } from 'node:child_process'
import { readFile, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { createServer } from 'node:https'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, it, onTestFinished } from 'vitest'
import { materializeTlsFixture } from '../../../packages/host/webserver/tests/tls-fixture.ts'

it('accepts a system-trusted issuer and rejects an unrelated hostname in an isolated process', async () => {
  const root = await mkdtemp(join(tmpdir(), 'remote-desktop-tls-'))
  const owned: { server?: ReturnType<typeof createServer>; child?: ChildProcess } = {}
  let closed = Promise.resolve()
  onTestFinished(async () => {
    const { server, child } = owned
    if (child?.exitCode === null && child.signalCode === null) child.kill()
    await closed
    if (server) {
      server.closeAllConnections()
      if (server.listening) await new Promise<void>((resolve, reject) => server.close((error) => {
        if (error) reject(error)
        else resolve()
      }))
    }
    await rm(root, { recursive: true, force: true })
  })
  const fixture = await materializeTlsFixture(root)
  const server = createServer({ cert: await readFile(fixture.certFile), key: await readFile(fixture.keyFile) }, (_request, response) => {
    response.end('trusted server')
  })
  owned.server = server
  await new Promise<void>((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve) })
  const address = server.address()
  if (!address || typeof address === 'string') throw new Error('TLS fixture did not bind TCP')
  const probe = join(root, 'probe.mjs')
  await writeFile(probe, String.raw`
import tls from 'node:tls'
import { syncBuiltinESMExports } from 'node:module'
import { readFile } from 'node:fs/promises'
import { get } from 'node:https'
const [url, caFile, source] = process.argv.slice(2)
const ca = await readFile(caFile, 'utf8')
const readRoots = tls.getCACertificates
tls.getCACertificates = type => type === 'system' ? [ca] : readRoots(type)
syncBuiltinESMExports()
let untrusted = false
try { await fetch(url) } catch (error) {
  untrusted = ['DEPTH_ZERO_SELF_SIGNED_CERT', 'UNABLE_TO_VERIFY_LEAF_SIGNATURE'].includes(error.cause?.code)
}
if (!untrusted) throw new Error('The unconfigured issuer was not rejected')
const { configureRemoteTls } = await import(source)
configureRemoteTls()
const response = await fetch(url)
if (await response.text() !== 'trusted server') throw new Error('The OS issuer was not accepted')
await new Promise((resolve, reject) => {
  const request = get(url, { servername: 'unrelated.invalid' }, response => {
    response.resume()
    reject(new Error('A mismatched hostname was accepted'))
  })
  request.once('error', error => error.code === 'ERR_TLS_CERT_ALTNAME_INVALID' ? resolve() : reject(error))
})
console.log('system issuer accepted; unrelated hostname rejected')
`)
  const stdout = await new Promise<string>((resolve, reject) => {
    const child = execFile(process.execPath, ['--import', 'tsx/esm', probe, `https://127.0.0.1:${address.port}`,
      fixture.caFile, fileURLToPath(new URL('../src/remote-tls.ts', import.meta.url))], (error, stdout) => {
      if (error) reject(new Error(error.message, { cause: error }))
      else resolve(stdout)
    })
    owned.child = child
    closed = new Promise<void>((resolve) => { child.once('close', () => { resolve() }) })
  })
  expect(stdout.trim()).toBe('system issuer accepted; unrelated hostname rejected')
})
