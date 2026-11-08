/** Build-tool entry shared by humans, Agents and GitHub Actions; stdout is one JSON result. */
import { fileURLToPath } from 'node:url'
import { resolve } from 'node:path'
import { parseArgs, type ParseArgsConfig } from 'node:util'
import { z } from 'zod'
import { ReleaseError, targetSchema, verifyArtifact } from './model.ts'
import { assembleRelease, verifyRelease } from './release.ts'

const repository = fileURLToPath(new URL('../..', import.meta.url))
const controller = new AbortController()
const stop = () => { controller.abort() }
process.on('SIGINT', stop); process.on('SIGTERM', stop)
const string = { type: 'string' } as const
const boolean = { type: 'boolean' } as const
const options = {
  inspect: {},
  build: { target: string, out: string, frontend: string, 'release-id': string, 'allow-dirty': boolean, signing: string, 'timeout-ms': string },
  verify: { input: string, release: boolean },
  release: { artifact: { ...string, multiple: true }, out: string, require: string, 'timeout-ms': string },
  publish: { input: string, repository: string, mode: string, 'expected-commit': string },
  deploy: { input: string, config: string, root: string, 'dry-run': boolean },
  rollback: { input: string, config: string, root: string, 'dry-run': boolean },
  pack: { input: string, out: string, 'timeout-ms': string },
  unpack: { input: string, out: string },
  collect: { input: string, out: string, 'timeout-ms': string },
  run: { repository: string, id: string, attempt: string },
  runner: { labels: string, root: string, config: string },
  select: { input: string, target: string, out: string, 'expected-commit': string },
  smoke: { input: string, fixture: string, 'timeout-ms': string },
}
type Command = keyof typeof options
const required = (value: string | undefined, name: string) => {
  if (!value) throw new ReleaseError('arguments', 'Missing --' + name)
  return value
}
const operation = process.argv[2] ?? 'help'
try {
  if (operation === 'help' || operation === '--help') {
    process.stdout.write(JSON.stringify({ ok: true, schemaVersion: 1, operation: 'help', commands: Object.keys(options), usage: 'pnpm agentwork <command> [options]',
      manual: 'docs/cookbook/releasing-agentwork.md', agent: '.agents/skills/agentwork-release/SKILL.md' }) + '\n')
  } else {
    if (!Object.hasOwn(options, operation)) throw new ReleaseError('arguments', 'Unknown operation; use pnpm agentwork help')
    const selected: NonNullable<ParseArgsConfig['options']> = options[operation as Command]
    const parsed = parseArgs({ args: process.argv.slice(3), options: selected, allowPositionals: false })
    const text = z.string().optional()
    const values = z.object({ target: text, out: text, frontend: text, 'release-id': text, 'allow-dirty': z.boolean().optional(), signing: text,
      'timeout-ms': text, input: text, release: z.boolean().optional(), artifact: z.array(z.string()).optional(), require: text,
      repository: text, mode: text, 'expected-commit': text, config: text, root: text, 'dry-run': z.boolean().optional(), id: text, attempt: text, labels: text, fixture: text }).strict().parse(parsed.values)
    // Each option is validated again by its owning operation before side effects.
    const timeout = values['timeout-ms'] === undefined ? 30 * 60_000 : z.coerce.number().int().positive().parse(values['timeout-ms'])
    const execution = { cwd: repository, timeoutMs: timeout, signal: controller.signal }
    let result: unknown
    if (operation === 'build') {
      const { buildArtifact } = await import('./build.ts')
      result = await buildArtifact({ ...execution, repository, target: targetSchema.parse(required(values.target, 'target')), out: resolve(required(values.out, 'out')),
        ...values.frontend ? { frontend: resolve(values.frontend) } : {}, ...values['release-id'] ? { releaseId: values['release-id'] } : {},
        allowDirty: values['allow-dirty'] === true, signing: z.enum(['unsigned-development', 'developer-id', 'development-device']).parse(values.signing ?? 'unsigned-development') })
    } else if (operation === 'inspect') {
      const { sourceMetadata } = await import('./build.ts')
      result = { ...await sourceMetadata({ ...execution, repository, target: 'frontend', out: resolve(repository, '.artifacts/unused'), allowDirty: true, signing: 'unsigned-development' }),
        requiredTargets: (await import('./release.ts')).requiredTargets }
    } else if (operation === 'verify') {
      result = values.release ? await verifyRelease(resolve(required(values.input, 'input'))) : await verifyArtifact(resolve(required(values.input, 'input')))
    } else if (operation === 'release') {
      const inputs = values.artifact
      if (!inputs?.length) throw new ReleaseError('arguments', 'Use --artifact once per target, including frontend')
      const targets = values.require ? values.require.split(',').map(word => targetSchema.parse(word)) : undefined
      result = await assembleRelease(inputs.map(path => resolve(path)), resolve(required(values.out, 'out')), execution, targets)
    } else if (operation === 'publish') {
      const { publishRelease } = await import('./publish.ts')
      result = await publishRelease(resolve(required(values.input, 'input')), required(values.repository, 'repository'),
        z.enum(['draft', 'publish']).parse(values.mode ?? 'draft'), required(values['expected-commit'], 'expected-commit'), execution)
    } else if (operation === 'deploy' || operation === 'rollback') {
      const { deployArtifact } = await import('./deploy.ts')
      result = await deployArtifact({ ...execution, artifact: resolve(required(values.input, 'input')), config: resolve(required(values.config, 'config')),
        root: resolve(required(values.root, 'root')), dryRun: values['dry-run'] === true, rollback: operation === 'rollback' })
    } else if (operation === 'pack') {
      const { packArtifact } = await import('./transport.ts')
      result = await packArtifact(resolve(required(values.input, 'input')), resolve(required(values.out, 'out')), execution)
    } else if (operation === 'unpack') {
      const { unpackArtifact } = await import('./transport.ts')
      result = await unpackArtifact(resolve(required(values.input, 'input')), resolve(required(values.out, 'out')))
    } else if (operation === 'collect') {
      const { collectArtifacts } = await import('./transport.ts')
      result = await collectArtifacts(resolve(required(values.input, 'input')), resolve(required(values.out, 'out')), execution)
    } else if (operation === 'run') {
      const { inspectBuildRun } = await import('./actions.ts')
      result = await inspectBuildRun(required(values.repository, 'repository'), required(values.id, 'id'), required(values.attempt, 'attempt'))
    } else if (operation === 'runner') {
      const { deploymentRunner } = await import('./deploy.ts')
      result = { labels: deploymentRunner(required(values.labels, 'labels'), required(values.root, 'root'), required(values.config, 'config')) }
    } else if (operation === 'select') {
      const { selectReleaseArtifact } = await import('./transport.ts')
      result = await selectReleaseArtifact(resolve(required(values.input, 'input')), targetSchema.parse(required(values.target, 'target')),
        resolve(required(values.out, 'out')), required(values['expected-commit'], 'expected-commit'))
    } else if (operation === 'smoke') {
      const { smokeServer } = await import('./smoke.ts')
      result = await smokeServer(resolve(required(values.input, 'input')), resolve(required(values.fixture, 'fixture')), execution)
    }
    controller.signal.throwIfAborted()
    process.stdout.write(JSON.stringify({ ok: true, schemaVersion: 1, operation, result }) + '\n')
  }
} catch (error) {
  const argumentError = error instanceof Error && 'code' in error && typeof error.code === 'string' && error.code.startsWith('ERR_PARSE_ARGS_')
  const invalidInput = error instanceof z.ZodError || error instanceof SyntaxError
  const unknownOutcome = error instanceof ReleaseError && ['publication-unknown', 'deployment-incomplete'].includes(error.code)
  const cancelled = controller.signal.aborted && !unknownOutcome
  const code = cancelled ? 'cancelled' : error instanceof ReleaseError ? error.code : invalidInput ? 'invalid-input' : argumentError ? 'arguments' : 'operation-failed'
  const message = cancelled ? 'Operation cancelled; inspect any completed outputs before retrying' : error instanceof ReleaseError ? error.message : invalidInput ? 'Input or manifest is invalid; inspect the documented fields and select a supported artifact' : argumentError ? 'Unknown or invalid options; use pnpm agentwork help and the command reference' : 'Operation failed; inspect stderr and verify the selected paths, permissions and toolchain before retrying'
  if (!(error instanceof ReleaseError) && !invalidInput && !argumentError) process.stderr.write((error instanceof Error ? error.message : String(error)) + '\n')
  process.stdout.write(JSON.stringify({ ok: false, schemaVersion: 1, operation, error: { code, message } }) + '\n')
  process.exitCode = code === 'cancelled' ? 130 : code === 'arguments' || code === 'invalid-input' ? 2 : 1
} finally {
  process.off('SIGINT', stop); process.off('SIGTERM', stop)
}
