/** A transport-owned fake GitHub verifies resumable drafts without publishing external releases. */
import { createHash } from 'node:crypto'
import { join } from 'node:path'
import { expect, it } from 'vitest'
import { artifact, cohort, temporary } from './fixture.ts'
import { publishRelease } from './publish.ts'
import { assembleRelease } from './release.ts'

async function fixture() {
  const root = await temporary()
  const out = join(root, 'release')
  const execution = { cwd: root, timeoutMs: 30_000, env: { GH_TOKEN: 'fixture-only-not-a-credential' } }
  await assembleRelease(await cohort(root), out, execution)
  const assets: { name: string; digest: string }[] = []
  let created = false, draft = true, uploads = 0, failAfterUpload = false
  const response = () => ({ id: 1, tag_name: 'agentwork-v1.0.0-rc.1', target_commitish: 'a'.repeat(40), draft,
    upload_url: 'https://uploads.github.com/repos/fixture/agent-work/releases/1/assets{?name,label}',
    html_url: 'https://github.com/fixture/agent-work/releases/tag/agentwork-v1.0.0-rc.1', assets })
  const transport: typeof fetch = async (input, init) => {
    const url = new URL(typeof input === 'string' ? input : input instanceof URL ? input.href : input.url)
    if (url.pathname.includes('/git/ref/')) return Response.json({}, { status: 404 })
    if (url.hostname === 'uploads.github.com') {
      if (!(init?.body instanceof Blob)) throw new Error('Upload must use a file-backed blob')
      const asset = { name: url.searchParams.get('name')!, digest: 'sha256:' + createHash('sha256').update(Buffer.from(await init.body.arrayBuffer())).digest('hex') }
      assets.push(asset); uploads++
      if (failAfterUpload) { failAfterUpload = false; throw new Error('connection lost after upload') }
      return Response.json(asset, { status: 201 })
    }
    if (init?.method === 'POST') { created = true; return Response.json(response(), { status: 201 }) }
    if (!created) return Response.json({}, { status: 404 })
    if (init?.method === 'PATCH') draft = false
    return Response.json(response())
  }
  return { out, execution, assets, transport, uploads: () => uploads, fail: () => { failAfterUpload = true } }
}

it('creates a verified draft and retries identical assets without overwriting them', async () => {
  const f = await fixture()
  expect(await publishRelease(f.out, 'fixture/agent-work', 'draft', 'a'.repeat(40), f.execution, f.transport)).toMatchObject({ draft: true })
  const uploaded = f.uploads()
  expect(uploaded).toBe(7)
  await publishRelease(f.out, 'fixture/agent-work', 'draft', 'a'.repeat(40), f.execution, f.transport)
  expect(f.uploads()).toBe(uploaded)
  expect(await publishRelease(f.out, 'fixture/agent-work', 'publish', 'a'.repeat(40), f.execution, f.transport)).toMatchObject({ draft: false })
})

it('reports an unknown upload outcome and resumes its preserved draft safely', async () => {
  const f = await fixture()
  f.fail()
  await expect(publishRelease(f.out, 'fixture/agent-work', 'draft', 'a'.repeat(40), f.execution, f.transport)).rejects.toMatchObject({ code: 'publication-unknown' })
  await publishRelease(f.out, 'fixture/agent-work', 'draft', 'a'.repeat(40), f.execution, f.transport)
  expect(f.uploads()).toBe(7)
})

it.each(['create-json', 'upload-json', 'publish-json', 'create-server-error'])('preserves ambiguous write results: %s', async (failure) => {
  const f = await fixture()
  const transport: typeof fetch = async (input, init) => {
    const response = await f.transport(input, init)
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
    const selected = failure.startsWith('upload') ? url.startsWith('https://uploads.github.com/')
      : failure.startsWith('publish') ? init?.method === 'PATCH' : init?.method === 'POST'
    if (!selected) return response
    return failure.endsWith('server-error') ? new Response('', { status: 500 }) : new Response('truncated-json', { status: 201 })
  }
  const mode = failure.startsWith('publish') ? 'publish' : 'draft'
  await expect(publishRelease(f.out, 'fixture/agent-work', mode, 'a'.repeat(40), f.execution, transport))
    .rejects.toMatchObject({ code: 'publication-unknown' })
  expect(await publishRelease(f.out, 'fixture/agent-work', mode, 'a'.repeat(40), f.execution, f.transport))
    .toMatchObject({ draft: mode === 'draft' })
  expect(f.uploads()).toBe(7)
})

it('rejects a wrong build commit and different existing asset content', async () => {
  const f = await fixture()
  await expect(publishRelease(f.out, 'fixture/agent-work', 'draft', 'd'.repeat(40), f.execution, f.transport)).rejects.toMatchObject({ code: 'incompatible' })
  expect(f.uploads()).toBe(0)
  await publishRelease(f.out, 'fixture/agent-work', 'draft', 'a'.repeat(40), f.execution, f.transport)
  f.assets[0]!.digest = 'sha256:' + 'e'.repeat(64)
  await expect(publishRelease(f.out, 'fixture/agent-work', 'draft', 'a'.repeat(40), f.execution, f.transport)).rejects.toMatchObject({ code: 'version-exists' })
})

it('does not publish a locally declared partial cohort as the complete product', async () => {
  const root = await temporary()
  const frontend = await artifact(root)
  const out = join(root, 'partial')
  const execution = { cwd: root, timeoutMs: 30_000 }
  await assembleRelease([frontend.directory], out, execution, ['frontend'])
  await expect(publishRelease(out, 'fixture/agent-work', 'draft', 'a'.repeat(40), execution,
    async () => { throw new Error('Publication must not reach the network') })).rejects.toMatchObject({ code: 'missing-target' })
})
