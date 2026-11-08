/** Explicit run selection rejects queued, failed, cancelled and untrusted build sources. */
import { expect, it } from 'vitest'
import { inspectBuildRun } from './actions.ts'

const successful = { head_sha: 'a'.repeat(40), head_branch: 'develop', event: 'workflow_dispatch', status: 'completed', conclusion: 'success',
  run_attempt: 2, path: '.github/workflows/agentwork-build.yml', html_url: 'https://github.com/fixture/agent-work/actions/runs/123' }

it('selects the requested run attempt and its source-bound artifact', async () => {
  const transport: typeof fetch = async (input) => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
    if (url.includes('/workflows/')) {
      expect(url).toBe(`https://api.github.com/repos/fixture/agent-work/actions/workflows/agentwork-ci.yml/runs?head_sha=${successful.head_sha}&branch=develop&per_page=100`)
      return Response.json({ workflow_runs: [{ ...successful, path: '.github/workflows/agentwork-ci.yml' }] })
    }
    expect(url).toBe('https://api.github.com/repos/fixture/agent-work/actions/runs/123/attempts/2')
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

it.each([
  { conclusion: 'failure' }, { conclusion: 'cancelled' }, { status: 'queued', conclusion: null },
  { status: 'in_progress', conclusion: null }, { event: 'pull_request' }, { head_sha: 'b'.repeat(40) },
])('blocks a green build when its source has no eligible successful CI %j', async (change) => {
  const transport: typeof fetch = async input => Response.json((typeof input === 'string' ? input : input instanceof URL ? input.href : input.url).includes('/workflows/')
    ? { workflow_runs: [{ ...successful, path: '.github/workflows/agentwork-ci.yml', ...change }] } : successful)
  await expect(inspectBuildRun('fixture/agent-work', '123', '2', transport)).rejects.toMatchObject({ code: 'checks-not-successful' })
})

it('rejects missing and inaccessible source CI instead of publishing a successful build alone', async () => {
  for (const response of [Response.json({ workflow_runs: [] }), Response.json({}, { status: 403 })]) {
    const transport: typeof fetch = async input => (typeof input === 'string' ? input : input instanceof URL ? input.href : input.url).includes('/workflows/') ? response : Response.json(successful)
    await expect(inspectBuildRun('fixture/agent-work', '123', '2', transport)).rejects.toMatchObject({
      code: response.ok ? 'checks-not-successful' : 'github-rejected',
    })
  }
})
