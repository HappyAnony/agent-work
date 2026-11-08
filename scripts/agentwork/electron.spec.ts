/** Pinned Electron downloads own their temporary files and publish complete cache entries atomically. */
import { createHash } from 'node:crypto'
import { mkdir, readFile, readdir, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { expect, it } from 'vitest'
import { prepareElectronArchive } from './electron.ts'
import { temporary } from './fixture.ts'
import type { run } from './process.ts'

async function fixture() {
  const root = await temporary()
  const pkg = join(root, 'apps/desktop/node_modules/electron')
  await mkdir(pkg, { recursive: true })
  const name = 'electron-v1.0.0-darwin-arm64.zip', bytes = 'verified fixture archive'
  const sha256 = createHash('sha256').update(bytes).digest('hex')
  await writeFile(join(pkg, 'package.json'), JSON.stringify({ version: '1.0.0' }))
  await writeFile(join(pkg, 'checksums.json'), JSON.stringify({ [name]: sha256 }))
  return { root, name, bytes, cache: join(root, 'cache'), execution: { cwd: root, timeoutMs: 30_000 } }
}

function downloader(bytes: string): typeof run {
  return async (command, args) => {
    expect(command).toBe('curl')
    expect(args).toContain('https://github.com/electron/electron/releases/download/v1.0.0/electron-v1.0.0-darwin-arm64.zip')
    expect(args).toContain('--retry')
    expect(args).not.toContain('--insecure')
    const output = args[args.indexOf('--output') + 1]
    if (!output) throw new Error('Fixture needs the download output')
    await writeFile(output, bytes)
    return ''
  }
}

it('verifies a download, retains its complete cache and reuses it without network access', async () => {
  const f = await fixture()
  const path = await prepareElectronArchive(f.root, 'arm64', f.cache, f.execution, downloader(f.bytes))
  expect(await readFile(path, 'utf8')).toBe(f.bytes)
  expect(await readdir(f.cache)).toEqual([f.name])
  expect(await prepareElectronArchive(f.root, 'arm64', f.cache, f.execution, async () => { throw new Error('No download expected') })).toBe(path)
})

it('rejects corrupt downloads and removes only its incomplete temporary output', async () => {
  const f = await fixture()
  await expect(prepareElectronArchive(f.root, 'arm64', f.cache, f.execution, downloader('corrupt'))).rejects.toMatchObject({ code: 'electron-integrity' })
  expect(await readdir(f.cache)).toEqual([])
})

it('preserves an invalid existing cache entry for explicit operator recovery', async () => {
  const f = await fixture()
  await mkdir(f.cache)
  await writeFile(join(f.cache, f.name), 'invalid cache')
  await expect(prepareElectronArchive(f.root, 'arm64', f.cache, f.execution, downloader(f.bytes))).rejects.toMatchObject({ code: 'electron-integrity' })
  expect(await readFile(join(f.cache, f.name), 'utf8')).toBe('invalid cache')
})

it('admits concurrent complete downloads without exposing a partially published cache file', async () => {
  const f = await fixture()
  let arrivals = 0
  const { promise: barrier, resolve: release } = Promise.withResolvers<undefined>()
  const execute: typeof run = async (...args) => {
    arrivals++
    if (arrivals === 2) release(undefined)
    await barrier
    return downloader(f.bytes)(...args)
  }
  const paths = await Promise.all([1, 2].map(() => prepareElectronArchive(f.root, 'arm64', f.cache, f.execution, execute)))
  expect(paths[0]).toBe(paths[1])
  expect(await readdir(f.cache)).toEqual([f.name])
})

it('rejects missing architecture checksums and relative cache configuration before download', async () => {
  const f = await fixture()
  await expect(prepareElectronArchive(f.root, 'x64', f.cache, f.execution, downloader(f.bytes))).rejects.toMatchObject({ code: 'electron-integrity' })
  await expect(prepareElectronArchive(f.root, 'arm64', 'relative', f.execution, downloader(f.bytes))).rejects.toMatchObject({ code: 'arguments' })
})
