/** Adapter discovery follows package metadata rather than a directory group. */
import { mkdir, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { expect, it } from 'vitest'
import { temporary } from './agentwork/fixture.ts'
import { clientNodeAdapterWorkspaces } from './client-node-adapter-workspaces.ts'

it('includes API, Typert and experimental Clients and excludes ordinary Host workspaces', async () => {
  const root = await temporary()
  const directories = ['packages/client/ui', 'packages/api/gateway', 'packages/typert/registry', 'packages/experimental/wiki', 'packages/core/agent']
  for (const directory of directories) {
    await mkdir(join(root, directory), { recursive: true })
    await writeFile(join(root, directory, 'package.json'), JSON.stringify({ name: directory, ...directory.endsWith('/agent') ? {} : { dsh: { client: {} } } }))
  }
  expect(clientNodeAdapterWorkspaces(root, directories)).toEqual(directories.slice(0, -1))
  await writeFile(join(root, directories[0]!, 'package.json'), JSON.stringify({ dsh: { client: 'invalid' } }))
  expect(() => clientNodeAdapterWorkspaces(root, directories)).toThrow()
})
