/** Build the explicit local AgentWork composition without running a Host or reading user data. */
import { createHash } from 'node:crypto'
import { readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { z } from 'zod'
import { PLATFORM_MODULES } from '../../../packages/client/web/src/platform.ts'
import { bootInjections, orderByModuleGraph, type WebBootEntry, type WebBootGraph } from '@deepseek-ai/dsh-client-modules'

const client = z.object({
  inject: z.array(z.string()).optional(), external: z.array(z.string()).optional(), immediately: z.boolean().optional(),
})
const composition = z.object({ entries: z.array(z.string()).min(1), defaultServerUrl: z.string().url() })
const digest = (text: string) => createHash('sha256').update(text).digest('hex').slice(0, 12)

/**
 * Assemble startup metadata from the selected packages' declared browser dependencies.
 * @param repository - Built checkout owning the default composition.
 * @param packages - Packaged client names and their source directories.
 * @param bundles - Digests of the exact packaged browser artifacts.
 * @returns Public local startup inputs, without permissions or backend data.
 */
export async function packagedBootstrap(
  repository: string, packages: ReadonlyMap<string, string>, bundles: Readonly<Record<string, string>>,
) {
  const selected = composition.parse(JSON.parse(await readFile(join(repository, 'apps/desktop/resources/agentwork-client.json'), 'utf8')))
  const entries: WebBootEntry[] = []
  for (const id of selected.entries) {
    const directory = packages.get(id)
    const hash = bundles[id]
    if (!directory || !hash) throw new Error('Packaged client entry is missing: ' + id)
    const pkg = z.object({ dsh: z.object({ client }) }).parse(JSON.parse(await readFile(join(directory, 'package.json'), 'utf8')))
    entries.push({ id, rev: hash.slice(0, 12), url: '/plugins/??' + id + '/client.js&rev=' + hash.slice(0, 12), ...pkg.dsh.client.inject === undefined ? {} : { inject: pkg.dsh.client.inject },
      ...pkg.dsh.client.external === undefined ? {} : { external: pkg.dsh.client.external },
      ...pkg.dsh.client.immediately === undefined ? {} : { immediately: pkg.dsh.client.immediately } })
  }
  const ordered = orderByModuleGraph(entries)
  for (const entry of ordered) {
    for (const dependency of [...entry.inject ?? [], ...entry.external ?? []]) {
      const id = dependency.replace(/\/client$/u, '')
      if (!bundles[id] && !PLATFORM_MODULES.some(word => word === dependency)) throw new Error('Packaged client dependency is missing: ' + dependency)
    }
  }
  const batches = (['bootstrap', 'application'] as const).map((phase) => {
    const ids = ordered.filter(row => (row.id === '@deepseek-ai/dsh-client-modules') === (phase === 'bootstrap')).map(row => row.id)
    const rev = digest(ids.map(id => bundles[id]).join(':'))
    return { phase, entries: ids, rev, url: '/plugins/??' + ids.map(id => id + '/client.js').join(',') + '&rev=' + rev }
  }).filter(batch => batch.entries.length > 0)
  const graph: WebBootGraph = { rev: digest(JSON.stringify({ entries: ordered, batches })), entries: ordered, batches }
  return { defaultServerUrl: selected.defaultServerUrl, injections: [
    { kind: 'global' as const, name: '__DSH_MODELS_ONBOARDING__', value: { credentialOnboarding: false } },
    ...bootInjections(graph),
  ] }
}
