/** Native startup never blocks local settings or silently switches the selected Host. */
import { expect, it, vi } from 'vitest'
import { PackagedServer, PackagedServerError, type PackagedServerBootstrap } from '../src/packaged-server.ts'

const bootstrap: PackagedServerBootstrap = { globals: {}, graph: {} }

it('starts with a saved server and retains its failed selection for an explicit retry', async () => {
  const connect = vi.fn().mockRejectedValueOnce(new PackagedServerError('authentication')).mockResolvedValue(bootstrap)
  const server = new PackagedServer({ readAddress: async () => 'https://selected.test', connect }, 'http://127.0.0.1:18686')
  try {
    await server.start()
    expect(server.getSnapshot()).toMatchObject({ address: 'https://selected.test', phase: 'unavailable', failure: 'authentication' })
    server.requestConfiguration(); server.requestConfiguration()
    expect(server.getSnapshot().configurationRequested).toBe(true)
    await server.retry()
    expect(server.getSnapshot()).toMatchObject({ phase: 'connected', configurationRequested: false, bootstrap })
    expect(connect.mock.calls).toEqual([['https://selected.test', ''], ['https://selected.test', '']])
  } finally { server.dispose() }
})

it('keeps a new iOS installation usable without a default server', async () => {
  const connect = vi.fn()
  const server = new PackagedServer({ readAddress: async () => '', connect }, '')
  try {
    await server.start()
    expect(server.getSnapshot().phase).toBe('unconfigured')
    expect(connect).not.toHaveBeenCalled()
    server.requestConfiguration(); server.dismissConfiguration()
    expect(server.getSnapshot().configurationRequested).toBe(false)
  } finally { server.dispose() }
})

it('shares a pending connection and ignores its completion after disposal', async () => {
  const result = Promise.withResolvers<PackagedServerBootstrap>()
  const connect = vi.fn(() => result.promise)
  const server = new PackagedServer({ readAddress: async () => '', connect }, '')
  const notify = vi.fn()
  server.subscribe(notify)
  const first = server.configure('https://selected.test', 'credential')
  const second = server.configure('https://other.test', '')
  expect(first).toBe(second)
  await Promise.resolve()
  expect(connect).toHaveBeenCalledTimes(1)
  server.dispose()
  const notifications = notify.mock.calls.length
  result.resolve(bootstrap)
  await first
  expect(notify).toHaveBeenCalledTimes(notifications)
  expect(server.getSnapshot()).toMatchObject({ address: 'https://selected.test', phase: 'connecting' })
})

it('does not overwrite a new selection when the initial native read finishes later', async () => {
  const saved = Promise.withResolvers<string>()
  const connect = vi.fn(async () => bootstrap)
  const server = new PackagedServer({ readAddress: () => saved.promise, connect }, '')
  try {
    const startup = server.start()
    await server.configure('https://new.test', '')
    saved.resolve('https://old.test')
    await startup
    expect(server.getSnapshot().address).toBe('https://new.test')
    expect(connect).toHaveBeenCalledTimes(1)
  } finally { saved.resolve(''); server.dispose() }
})

it('keeps settings available after a native credential read failure without contacting the default server', async () => {
  const connect = vi.fn()
  const server = new PackagedServer({
    readAddress: async () => { throw new PackagedServerError('storage') }, connect,
  }, 'http://127.0.0.1:18686')
  try {
    await server.start()
    expect(server.getSnapshot()).toMatchObject({ phase: 'unavailable', failure: 'storage', configurationRequested: false })
    expect(connect).not.toHaveBeenCalled()
    server.requestConfiguration()
    expect(server.getSnapshot().configurationRequested).toBe(true)
  } finally { server.dispose() }
})
