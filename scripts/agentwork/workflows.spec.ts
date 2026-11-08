/** Actions orchestration isolates upstream jobs and passes one same-attempt frontend to native builds. */
import { readFileSync, readdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { join } from 'node:path'
import { load } from 'js-yaml'
import { z } from 'zod'
import { expect, it } from 'vitest'
import { requiredTargets } from './release.ts'

const root = fileURLToPath(new URL('../../.github/workflows', import.meta.url))
const workflowSchema = z.object({ permissions: z.union([z.string(), z.record(z.string(), z.string())]).optional(),
  jobs: z.record(z.string(), z.object({
    if: z.string().optional(), needs: z.union([z.string(), z.array(z.string())]).optional(),
    permissions: z.record(z.string(), z.string()).optional(),
  }).loose()) }).loose()

it('guards every inherited workflow job without deleting upstream runner routing', () => {
  const files = readdirSync(root).filter(name => name.endsWith('.yml') && !name.startsWith('agentwork-'))
  expect(files).toHaveLength(32)
  let count = 0
  for (const name of files) {
    const workflow = workflowSchema.parse(load(readFileSync(join(root, name), 'utf8')))
    for (const job of Object.values(workflow.jobs)) {
      expect(job.if).toContain("github.repository == 'deepseek-ai/deepseek-harness'")
      count++
    }
  }
  expect(count).toBe(78)
})

it('uses hosted native runners, same-attempt downloads and the complete product cohort', () => {
  const text = readFileSync(join(root, 'agentwork-build.yml'), 'utf8')
  const entry = z.object({ target: z.string(), runner: z.string() })
  const matrix = z.object({ include: z.array(entry) })
  const workflow = z.object({ jobs: z.object({ native: z.object({ strategy: z.object({ matrix }) }) }) }).parse(load(text))
  expect(workflow.jobs.native.strategy.matrix.include.map(row => row.target).sort()).toEqual(requiredTargets.filter(target => target !== 'frontend').sort())
  for (const row of workflow.jobs.native.strategy.matrix.include) expect(['ubuntu-22.04', 'macos-26']).toContain(row.runner)
  expect(text).toContain('agentwork-artifact-frontend-${{ github.run_id }}-${{ github.run_attempt }}')
  expect(text.match(/agentwork build --target frontend/gu)).toHaveLength(1)
  expect(text).not.toContain('secrets: inherit')
})

it('grants publication write permission only in the explicit release workflow', () => {
  for (const name of ['agentwork-ci.yml', 'agentwork-build.yml', 'agentwork-release.yml']) {
    const workflow = workflowSchema.parse(load(readFileSync(join(root, name), 'utf8')))
    expect(z.object({ contents: z.literal('read') }).loose().parse(workflow.permissions).contents).toBe('read')
    for (const job of Object.values(workflow.jobs)) if (job.permissions?.contents === 'write') expect(name).toBe('agentwork-release.yml')
  }
})
