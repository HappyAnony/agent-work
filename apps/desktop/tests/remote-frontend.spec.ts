/** Local resources and build compatibility of a remote-only desktop client. */
import { createHash } from 'node:crypto'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { expect, it, onTestFinished } from 'vitest'
import { RemoteFrontend, readRemoteBootstrap, remoteServerOrigin } from '../src/remote-frontend.ts'

const hash = (value: string) => createHash('sha256').update(value).digest('hex')

async function fixture() {
  const root = await mkdtemp(join(tmpdir(), 'agentwork-remote-assets-'))
  onTestFinished(() => rm(root, { recursive: true, force: true }))
  const bundle = 'globalThis.localPlugin = true;'
  const index = '<html><head></head><body>Local frontend</body></html>'
  await Promise.all([mkdir(join(root, 'web'), { recursive: true }), mkdir(join(root, 'plugins/@test/plugin'), { recursive: true }),
    mkdir(join(root, 'wiki/stage/build/portable'), { recursive: true })])
  const manifest = { protocolVersion: 1 as const, frontendHash: hash(index), bundles: { '@test/plugin': hash(bundle) }, wikiHash: hash('wiki') }
  await Promise.all([writeFile(join(root, 'web/index.html'), index), writeFile(join(root, 'plugins/@test/plugin/client.js'), bundle),
    writeFile(join(root, 'wiki/stage/build/portable/boot.js'), 'wiki'),
    writeFile(join(root, 'wiki/stage/build/portable/index.html'), '<body>Local Wiki</body>'),
    writeFile(join(root, 'manifest.json'), JSON.stringify(manifest))])
  return { root, manifest, frontend: await RemoteFrontend.load(root) }
}

it('loads the shell, plugin factories and Wiki editor locally while delegating backend requests', async () => {
  const { frontend, manifest } = await fixture()
  await frontend.verify({ ...manifest, injections: [] })
  expect(await (await frontend.resource(new Request('dsh-app://app/')))?.text()).toContain('Local frontend')
  expect(await (await frontend.resource(new Request('dsh-app://app/plugins/??@test/plugin/client.js&rev=server')))?.text()).toContain('localPlugin')
  expect(await (await frontend.resource(new Request('dsh-app://app/stage/build/portable/')))?.text()).toContain('Local Wiki')
  expect(await (await frontend.resource(new Request('dsh-app://app/stage/build/portable/#parentOrigin=dsh-app%3A%2F%2Fapp&channelId=fixture')))?.text()).toContain('Local Wiki')
  expect(await frontend.resource(new Request('dsh-app://app/api/wikiman/v1/content'))).toBeUndefined()
  expect((await frontend.resource(new Request('dsh-app://app/plugins/??@test/missing/client.js&rev=x')))?.status).toBe(404)
  expect((await frontend.resource(new Request('dsh-app://app/plugins/@test/plugin/%2e%2e%2fsecret.js')))?.status).toBe(404)
})

it('refuses incompatible or altered local code before activating a remote composition', async () => {
  const { frontend, manifest, root } = await fixture()
  await expect(frontend.verify({ ...manifest, frontendHash: hash('other'), injections: [] })).rejects.toThrow('remote-frontend-version')
  await expect(frontend.verify({ ...manifest, bundles: { '@test/absent': hash('other') }, injections: [] })).rejects.toThrow('remote-plugin-version')
  await expect(frontend.verify({ ...manifest, injections: [{ name: '__DSH_WIKIMAN_ASSETS__', value: hash('other') }] })).rejects.toThrow('remote-wiki-version')
  await writeFile(join(root, 'wiki/stage/build/portable/boot.js'), 'altered')
  await expect(frontend.verify({ ...manifest, injections: [{ name: '__DSH_WIKIMAN_ASSETS__', value: manifest.wikiHash }] })).rejects.toThrow('remote-wiki-version')
  await writeFile(join(root, 'plugins/@test/plugin/client.js'), 'altered')
  await expect(frontend.verify({ ...manifest, injections: [] })).rejects.toThrow('remote-plugin-integrity')
})

it.each(['https://agentwork.internal', 'http://127.0.0.1:3080', 'http://[::1]:3080'])('accepts the configured server %s', (address) => {
  expect(remoteServerOrigin(address)).toBe(address)
})

it('bounds the bootstrap stream before activation and rejects malformed startup data', async () => {
  let cancelled = false
  let supplied = 0
  const body = new ReadableStream<Uint8Array>({
    pull(controller) { supplied++; controller.enqueue(new Uint8Array(1024 * 1024)) },
    cancel() { cancelled = true },
  })
  await expect(readRemoteBootstrap(new Response(body))).rejects.toMatchObject({ code: 'remote-bootstrap-limit' })
  expect(cancelled).toBe(true)
  expect(supplied).toBeLessThanOrEqual(10)
  await expect(readRemoteBootstrap(new Response('{}'))).rejects.toMatchObject({ code: 'remote-bootstrap-invalid' })
  await expect(readRemoteBootstrap(new Response('invalid'))).rejects.toMatchObject({ code: 'remote-bootstrap-invalid' })
  const { manifest } = await fixture()
  const bootstrap = { ...manifest, injections: [] }
  expect(await readRemoteBootstrap(Response.json(bootstrap))).toMatchObject({ frontendHash: manifest.frontendHash, injections: [] })
})
it.each(['http://agentwork.internal', 'https://user:secret@agentwork.internal', 'https://agentwork.internal/path',
  'https://agentwork.internal/?token=secret', 'file:///tmp/server', 'https://agentwork.internal/#secret'])('rejects the unsupported server address %s', (address) => {
  expect(() => remoteServerOrigin(address)).toThrow()
})
