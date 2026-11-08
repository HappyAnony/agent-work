/** Explicit run selection rejects queued, failed, cancelled and untrusted build sources. */
import { expect, it } from 'vitest'
import { inspectBuildRun } from './actions.ts'

const successful = { head_sha: 'a'.repeat(40), head_branch: 'develop', event: 'workflow_dispatch', status: 'completed', conclusion: 'success',
  run_attempt: 2, path: '.github/workflows/agentwork-build.yml', html_url: 'https://github.com/fixture/agent-work/actions/runs/123' }

it('selects the requested run attempt and its source-bound artifact', async () => {
  const transport: typeof fetch = async (input) => {
    expect(typeof input === 'string' ? input : input instanceof URL ? input.href : input.url).toBe('https://api.github.com/repos/fixture/agent-work/actions/runs/123/attempts/2')
    return Response.json(successful)
  }
  expect(await inspectBuildRun('fixture/agent-work', '123', '2', transport)).toMatchObject({ commit: successful.head_sha, artifact: 'agentwork-release-123-2' })
})

it.each([{ conclusion: 'failure' }, { conclusion: 'cancelled' }, { status: 'queued', conclusion: null }, { status: 'in_progress', conclusion: null }])('refuses unsuccessful builds %j', async (change) => {
  await expect(inspectBuildRun('fixture/agent-work', '123', '2', async () => Response.json({ ...successful, ...change }))).rejects.toMatchObject({ code: 'build-not-successful' })
})

it.each([{ head_branch: 'feature' }, { event: 'pull_request' }, { run_attempt: 1 }, { path: '.github/workflows/ci.yml' }])('refuses untrusted build selection %j', async (change) => {
  await expect(inspectBuildRun('fixture/agent-work', '123', '2', async () => Response.json({ ...successful, ...change }))).rejects.toMatchObject({ code: 'untrusted-build' })
})

it('rejects injected identifiers and inaccessible or expired build results', async () => {
  await expect(inspectBuildRun('fixture/agent-work', '123;evil', '2')).rejects.toMatchObject({ code: 'arguments' })
  await expect(inspectBuildRun('fixture/agent-work', '123', '2', async () => Response.json({}, { status: 403 }))).rejects.toMatchObject({ code: 'github-rejected' })
})
