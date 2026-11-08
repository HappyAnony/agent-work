/** Source-owned build-tool output is checked through its real entry; it is not a shipped Node application. */
import { execa } from 'execa'
import { mkdir, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { join } from 'node:path'
import { expect, it, onTestFinished } from 'vitest'
import { temporary } from './fixture.ts'

const root = fileURLToPath(new URL('../..', import.meta.url))
async function cli(args: string[]) {
  const child = execa(process.execPath, ['--import', 'tsx/esm', 'scripts/agentwork/cli.ts', ...args], {
    cwd: root, reject: false, timeout: 30_000, killDescendants: true,
  })
  onTestFinished(async () => { child.kill(); await child })
  return child
}

it('records the public command list and machine-readable help', async () => {
  const result = await cli(['help'])
  expect(result.exitCode).toBe(0)
  await expect(JSON.stringify(JSON.parse(result.stdout), null, 2) + '\n').toMatchFileSnapshot(fileURLToPath(new URL('./tests/expected/help.json', import.meta.url)))
})

it('returns argument failures and invalid JSON without exposing input contents', async () => {
  const unknown = await cli(['verify', '--unexpected=fixture-private-value'])
  expect(unknown.exitCode).toBe(2)
  expect(JSON.parse(unknown.stdout)).toMatchObject({ ok: false, error: { code: 'arguments' } })
  expect(unknown.stderr).not.toContain('fixture-private-value')
  const dir = await temporary()
  await mkdir(join(dir, 'payload'))
  await writeFile(join(dir, 'artifact.json'), 'fixture-private-value')
  const malformed = await cli(['verify', '--input', dir])
  expect(malformed.exitCode).toBe(2)
  expect(malformed.stdout + malformed.stderr).not.toContain('fixture-private-value')
})
