/** Select an explicit successful Actions attempt before using its release artifacts. */
import { z } from 'zod'
import { ReleaseError } from './model.ts'

const runSchema = z.object({ head_sha: z.string().regex(/^[a-f0-9]{40}$/u), head_branch: z.string(), event: z.string(),
  status: z.string(), conclusion: z.string().nullable(), run_attempt: z.number().int().positive(), path: z.string(),
  html_url: z.url() })

/**
 * Inspect a specific build attempt without silently falling back to another run.
 * @param repository - GitHub owner/repository.
 * @param id - Numeric workflow run ID.
 * @param attempt - Explicit positive attempt number.
 * @param fetcher - HTTP transport; fixtures own it without changing global fetch.
 * @returns Trusted source commit and artifact name after the same source passes AgentWork CI.
 */
export async function inspectBuildRun(repository: string, id: string, attempt: string, fetcher: typeof fetch = fetch) {
  if (!/^[a-zA-Z0-9-]+\/[a-zA-Z0-9._-]+$/u.test(repository) || !/^[1-9][0-9]*$/u.test(id) || !/^[1-9][0-9]*$/u.test(attempt)) throw new ReleaseError('arguments', 'Use owner/repository, a numeric run ID and an explicit positive attempt')
  const token = process.env.GH_TOKEN ?? process.env.GITHUB_TOKEN
  const response = await fetcher(`https://api.github.com/repos/${repository}/actions/runs/${id}/attempts/${attempt}`, {
    headers: { Accept: 'application/vnd.github+json', ...token ? { Authorization: 'Bearer ' + token } : {} }, signal: AbortSignal.timeout(30_000),
  })
  if (!response.ok) throw new ReleaseError('github-rejected', 'Build attempt cannot be read; verify run ID, attempt and Actions permissions')
  const run = runSchema.parse(await response.json())
  if (run.head_branch !== 'develop' || !['workflow_dispatch', 'push'].includes(run.event) || run.path !== '.github/workflows/agentwork-build.yml'
    || run.run_attempt !== Number(attempt)) throw new ReleaseError('untrusted-build', 'Select an explicit AgentWork build attempt dispatched from develop')
  if (run.status !== 'completed' || run.conclusion !== 'success') throw new ReleaseError('build-not-successful', `Build is ${run.status}/${run.conclusion ?? 'pending'}; wait for completion or rerun all jobs before release`)
  const checksResponse = await fetcher(`https://api.github.com/repos/${repository}/actions/workflows/agentwork-ci.yml/runs?head_sha=${run.head_sha}&branch=develop&per_page=100`, {
    headers: { Accept: 'application/vnd.github+json', ...token ? { Authorization: 'Bearer ' + token } : {} }, signal: AbortSignal.timeout(30_000),
  })
  if (!checksResponse.ok) throw new ReleaseError('github-rejected', 'Source CI cannot be read; verify Actions permissions before publication')
  const checks = z.object({ workflow_runs: z.array(runSchema) }).parse(await checksResponse.json()).workflow_runs
  const sourceChecks = checks.find(check => check.head_sha === run.head_sha && check.head_branch === 'develop'
    && ['workflow_dispatch', 'push'].includes(check.event) && check.path === '.github/workflows/agentwork-ci.yml')
  if (!sourceChecks || sourceChecks.status !== 'completed' || sourceChecks.conclusion !== 'success') {
    throw new ReleaseError('checks-not-successful', 'The selected source must pass AgentWork CI; wait or repair its latest CI run before publication')
  }
  return { commit: run.head_sha, runId: id, attempt, artifact: `agentwork-release-${id}-${attempt}`, url: run.html_url }
}
