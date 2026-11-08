/** Explicit GitHub Release publication with immutable assets and resumable draft uploads. */
import { openAsBlob } from 'node:fs'
import { readdir } from 'node:fs/promises'
import { join } from 'node:path'
import { z } from 'zod'
import { ReleaseError, hashFile } from './model.ts'
import { requiredTargets, verifyRelease } from './release.ts'
import { run, type Execution } from './process.ts'
import { installationReleaseNotes } from './installation.ts'

const assetSchema = z.object({ name: z.string(), digest: z.string().nullable().optional() })
const remoteSchema = z.object({ id: z.number().int(), tag_name: z.string(), target_commitish: z.string(), draft: z.boolean(),
  upload_url: z.string(), html_url: z.url(), assets: z.array(assetSchema) })
const refSchema = z.object({ object: z.object({ type: z.enum(['commit', 'tag']), sha: z.string().regex(/^[a-f0-9]{40}$/u) }) })

async function mutationResult<T>(response: Response, schema: z.ZodType<T>): Promise<T> {
  try { return schema.parse(await response.json()) }
  catch (_error) {
    throw new ReleaseError('publication-unknown', 'GitHub returned an unreadable mutation result; inspect the preserved release before retrying')
  }
}

/**
 * Publish or resume one exact release; errors retain the remote draft for inspection.
 * @param directory - Locally verified release directory.
 * @param repository - Explicit GitHub owner/repository destination.
 * @param mode - Keep a draft or explicitly make the verified release public.
 * @param expectedCommit - Trusted workflow run or operator-selected source commit.
 * @param execution - Cancellation, request duration and optional GH_TOKEN environment.
 * @param fetcher - HTTP transport; fixtures can own it without changing global fetch.
 * @returns Remote URL and final draft state; network, server or unreadable write responses report an unknown outcome.
 */
export async function publishRelease(directory: string, repository: string, mode: 'draft' | 'publish', expectedCommit: string,
  execution: Execution, fetcher: typeof fetch = fetch) {
  if (!/^[a-zA-Z0-9-]+\/[a-zA-Z0-9._-]+$/u.test(repository)) throw new ReleaseError('arguments', 'Use an explicit owner/repository publication destination')
  const release = await verifyRelease(directory)
  for (const target of requiredTargets) if (!release.artifacts.some(row => row.target === target)) throw new ReleaseError('missing-target', 'Product publication requires target: ' + target)
  if (release.schemaVersion !== 2) throw new ReleaseError('missing-installation-guide', 'New publication requires a release with checksummed installation guides')
  if (release.commit !== expectedCommit) throw new ReleaseError('incompatible', 'Release source differs from the selected successful build run')
  if (release.artifacts.some(row => row.signing === 'development-device')) throw new ReleaseError('signing', 'Install Personal Team device builds locally through Xcode; they are not public release assets')
  if (!release.version.includes('-') && release.artifacts.some(row => row.target.startsWith('mac-') && row.signing !== 'developer-id')) throw new ReleaseError('signing', 'Stable Mac releases require verified Developer ID signing and notarization')
  const token = execution.env?.GH_TOKEN ?? execution.env?.GITHUB_TOKEN ?? process.env.GH_TOKEN ?? process.env.GITHUB_TOKEN
    ?? await run('gh', ['auth', 'token'], execution, true)
  if (!token.trim()) throw new ReleaseError('authentication', 'Authenticate gh or set GH_TOKEN before publication')
  const tag = 'agentwork-v' + release.version
  const base = `https://api.github.com/repos/${repository}`
  async function request(url: string, method = 'GET', body?: BodyInit): Promise<Response> {
    const host = new URL(url).hostname
    if (host !== 'api.github.com' && host !== 'uploads.github.com') throw new ReleaseError('github-response', 'GitHub returned an unsupported upload host')
    try {
      const response = await fetcher(url, { method, headers: { Authorization: 'Bearer ' + token,
        Accept: 'application/vnd.github+json', 'X-GitHub-Api-Version': '2022-11-28',
        ...body ? { 'Content-Type': typeof body === 'string' ? 'application/json' : 'application/octet-stream' } : {} },
      ...body ? { body } : {}, redirect: 'error', signal: execution.signal ? AbortSignal.any([execution.signal, AbortSignal.timeout(execution.timeoutMs)]) : AbortSignal.timeout(execution.timeoutMs) })
      if (method !== 'GET' && response.status >= 500) throw new ReleaseError('publication-unknown', 'GitHub returned a server error after a write; inspect the release before retrying')
      if (response.status !== 404 && !response.ok) throw new ReleaseError('github-rejected', `GitHub rejected ${method} with status ${response.status}; inspect the remote release before retrying`)
      return response
    } catch (error) {
      if (error instanceof ReleaseError) throw error
      throw new ReleaseError(method === 'GET' ? 'github-unavailable' : 'publication-unknown', 'GitHub request did not complete; inspect the release and its assets before retrying, since a remote mutation may have succeeded')
    }
  }
  const tagResponse = await request(base + '/git/ref/tags/' + tag)
  const tagExists = tagResponse.ok
  if (tagExists) {
    let object = refSchema.parse(await tagResponse.json()).object
    for (let depth = 0; object.type === 'tag' && depth < 8; depth++) object = refSchema.parse(await (await request(base + '/git/tags/' + object.sha)).json()).object
    if (object.type !== 'commit' || object.sha !== release.commit) throw new ReleaseError('version-exists', 'Release tag already points to different source; choose a new product version')
  }
  const existing = await request(base + '/releases/tags/' + tag)
  let remote = existing.ok ? remoteSchema.parse(await existing.json()) : undefined
  if (!remote) {
    // Tag lookup excludes drafts; authenticated release listings include them.
    const seen = new Set<number>()
    for (let page = 1; ; page++) {
      const listing = await request(base + '/releases?per_page=100&page=' + String(page))
      if (!listing.ok) throw new ReleaseError('github-response', 'Cannot inspect existing drafts; check repository permissions before retrying')
      const rows = z.array(remoteSchema).parse(await listing.json())
      for (const row of rows) {
        if (seen.has(row.id)) throw new ReleaseError('github-response', 'Release listing repeated entries; inspect GitHub before retrying')
        seen.add(row.id)
        if (row.tag_name !== tag) continue
        if (remote) throw new ReleaseError('version-exists', 'Multiple releases use this version; inspect existing drafts before retrying')
        remote = row
      }
      if (rows.length < 100) break
    }
  }
  if (!remote) {
    const response = await request(base + '/releases', 'POST', JSON.stringify({ tag_name: tag, target_commitish: release.commit, name: 'AgentWork ' + release.version,
      draft: true, prerelease: release.version.includes('-'), body: installationReleaseNotes(release, repository) }))
    remote = await mutationResult(response, remoteSchema)
  }
  if (remote.tag_name !== tag || (!tagExists && remote.target_commitish !== release.commit)) throw new ReleaseError('version-exists', 'Existing draft belongs to different source; it will not be replaced')
  const names = (await readdir(directory)).sort()
  if (remote.assets.some(row => !names.includes(row.name))) throw new ReleaseError('version-exists', 'Existing release has unlisted assets; inspect it instead of deleting or overwriting assets')
  for (const name of names) {
    const digest = 'sha256:' + await hashFile(join(directory, name))
    const uploaded = remote.assets.find(row => row.name === name)
    if (uploaded) {
      if (uploaded.digest !== digest) throw new ReleaseError('version-exists', 'Existing release asset has different or unverified content: ' + name)
      continue
    }
    if (!remote.draft) throw new ReleaseError('version-exists', 'Published release is incomplete; publication will not modify its assets')
    const url = new URL(z.url().parse(remote.upload_url.split('{')[0]))
    url.searchParams.set('name', name)
    const result = await mutationResult(await request(url.href, 'POST', await openAsBlob(join(directory, name))), assetSchema)
    if (result.name !== name || result.digest !== digest) throw new ReleaseError('publication-unknown', 'Uploaded asset digest is unverified; keep the draft and inspect GitHub before retrying')
  }
  remote = remoteSchema.parse(await (await request(base + '/releases/' + String(remote.id))).json())
  if (remote.assets.length !== names.length) throw new ReleaseError('publication-unknown', 'Remote release asset list changed during publication; inspect the draft')
  for (const asset of remote.assets) if (!names.includes(asset.name) || asset.digest !== 'sha256:' + await hashFile(join(directory, asset.name))) throw new ReleaseError('publication-unknown', 'Remote release verification failed; inspect the draft')
  if (mode === 'publish' && remote.draft) {
    remote = await mutationResult(await request(base + '/releases/' + String(remote.id), 'PATCH', JSON.stringify({ draft: false })), remoteSchema)
  }
  return { url: remote.html_url, tag, draft: remote.draft }
}
