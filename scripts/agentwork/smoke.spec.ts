/** A static frontend registration cannot substitute for an active WikiMan owner API. */
import { expect, it } from 'vitest'
import { checkWikiRuntime } from './smoke.ts'

it('reads fixture pages through an authenticated owner RPC', async () => {
  const transport: typeof fetch = async (input, init) => {
    expect(input).toBe('http://127.0.0.1:1234/api/wikiman/pages')
    expect(init?.headers).toMatchObject({ cookie: 'fixture-session', origin: 'http://127.0.0.1:1234' })
    if (typeof init?.body !== 'string') throw new Error('Expected a JSON request body')
    expect(JSON.parse(init.body)).toMatchObject({ type: 'client-request', method: 'wikiman/pages', payload: { args: { cursor: '' } } })
    return Response.json({ type: 'server-response', result: { ok: true, value: { items: [{ pageId: 'fixture' }], nextCursor: '' } } })
  }
  expect(await checkWikiRuntime('http://127.0.0.1:1234', 'fixture-session', 1000, transport)).toBe(1)
})

it.each([
  new Response('missing gateway', { status: 404 }), new Response('<html>frontend</html>'),
  Response.json({ type: 'server-response', result: { ok: false, error: { code: 'unavailable' } } }),
  Response.json({ type: 'server-response', result: { ok: true, value: { items: [], nextCursor: '' } } }),
])('rejects a missing or inactive runtime despite available frontend assets', async (response) => {
  await expect(checkWikiRuntime('http://127.0.0.1:1234', 'fixture-session', 1000, async () => response)).rejects.toMatchObject({ code: 'runtime-smoke' })
})
