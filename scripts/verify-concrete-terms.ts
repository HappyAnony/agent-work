/** Reject one ambiguous origin label from maintained tracked files. */

import { execFileSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { lstatSync, readFileSync, readlinkSync } from 'node:fs'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'
import { historicalSchemaRegion } from './historical-schema-region.ts'

const root = resolve(import.meta.dirname, '..')
const blockedTerm = 'prove' + 'nance'
const excludedPrefixes = ['vendor/', '.agents/notes/archived/'] as const

// Unchanged imported notices and source retain their pinned upstream names.
// Any local edit restores terminology checks for the complete file.
const pinnedWikiSources: Readonly<Record<string, string>> = {
  'native/wikiman/wiki-kernel/README.md': '44d9971c75f4b4bbab5ac4ae02a9650f528bbda83fc23d6cc48b4511bed49b76',
  'native/wikiman/wiki-kernel/THIRD_PARTY_NOTICES.md': '2bbbc42875fdb863aa5af5f06f852750c13f5360df00770ad481a705a09541da',
  [`native/wikiman/wiki-kernel/internal/flashcard/riff/${blockedTerm.toUpperCase()}.md`]: 'f00f4542babe5b2543c4fac4887f4ba5103e0ba97a26862b1c0af9b7111b9d55',
  [`native/wikiman/wiki-kernel/internal/pollwatch/backend/${blockedTerm.toUpperCase()}.md`]: 'e5bb48a5469fb7e41c17f4c133315db7efeb4bd33e28614090226c35c6cc9c3c',
  'native/wikiman/wiki-kernel/internal/pollwatch/backend/watcher.go': 'aa487d6c08815287f3c56a4210c687d56e74ed52ab41b0851ef5af1c60c8b054',
  'native/wikiman/wiki-kernel/internal/sourcetx/README.md': 'dbf72eef28f085bc7f46c936a2f425563db4689730f8eec220ff01192dca90b4',
  'native/wikiman/wiki-kernel/internal/sourcetx/preferences_snapshot.go': 'd11d4d0b9b461b256d49803fa51141eb439a3ce77774f7f2223af4d39aac313b',
  'native/wikiman/wiki-kernel/internal/workspace/README.md': 'e40cd748cd03e7059d5a53fcdb9acba91e6e59875a8422381cad37694e87c523',
}


/** One blocked term occurrence in a tracked path or text line. */
export interface ConcreteTermViolation {
  /** Repository-relative tracked path. */
  file: string
  /** One-based source line, or null when the path contains the term. */
  line: number | null
}

function isExcluded(file: string, source: string): boolean {
  return excludedPrefixes.some(prefix => file.startsWith(prefix))
    || file === 'native/wikiman/source-snapshot.json'
    || pinnedWikiSources[file] !== undefined && pinnedWikiSources[file] === createHash('sha256').update(source).digest('hex')
    // Release snapshots retain the identifiers present in their pinned source;
    // historical-format pairing records key sections by those identifiers' headings.
    || /^docs\/persistence-changes\/releases\/dsh-v\d+\.\d+\.\d+-(?:alpha|rc)\.\d+\.schema\.json$/u.test(file)
    || /^docs\/persistence-changes\/historical-formats\/v(?:0|[1-9]\d*)\.(?:schema\.json|i18n\.yaml)$/u.test(file)
}

function containsBlockedTerm(value: string): boolean {
  return value.normalize('NFKC').toLowerCase().includes(blockedTerm)
}

/**
 * Find the blocked term in one maintained tracked file.
 * @param file - repository-relative tracked path.
 * @param source - text contents or symlink target.
 * @returns violations outside vendored or byte-pinned imported sources, frozen Agent Notes and historical schema regions.
 */
export function findConcreteTermViolations(file: string, source: string): ConcreteTermViolation[] {
  if (isExcluded(file, source)) return []
  const violations: ConcreteTermViolation[] = []
  if (containsBlockedTerm(file)) violations.push({ file, line: null })
  const lines = source.split(/\r?\n/u)
  const schemaRegion = historicalSchemaRegion(file, source)
  for (const [index, line] of lines.entries()) {
    if (schemaRegion !== undefined && index >= schemaRegion[0] && index < schemaRegion[1]) continue
    if (containsBlockedTerm(line)) violations.push({ file, line: index + 1 })
  }
  return violations
}

function trackedFiles(repoRoot: string): string[] {
  const files = execFileSync('git', ['ls-files', '-z'], { cwd: repoRoot, encoding: 'utf8', maxBuffer: 16 * 1024 * 1024 })
    .split('\0')
    .filter(file => file !== '')
  if (!files.includes('AGENTS.md')
    || !files.some(file => file.startsWith('packages/'))
    || !files.some(file => file.startsWith('docs/'))
    || !files.some(file => file.startsWith('vendor/'))
    || !files.some(file => file.startsWith('.agents/notes/archived/'))) {
    throw new Error('verify-concrete-terms: tracked-file discovery omitted a required repository area')
  }
  return files
}

/**
 * Read one tracked file without following a symlink to its target.
 * @param repoRoot - Repository root containing the tracked path.
 * @param file - Repository-relative tracked path.
 * @returns File text, the symlink target, or undefined when the path is absent or not a file.
 */
export function readTrackedSource(repoRoot: string, file: string): string | undefined {
  const path = resolve(repoRoot, file)
  const stat = lstatSync(path, { throwIfNoEntry: false })
  if (stat === undefined) return undefined
  if (stat.isSymbolicLink()) return readlinkSync(path)
  return stat.isFile() ? readFileSync(path, 'utf8') : undefined
}

function scanRepository(repoRoot: string): ConcreteTermViolation[] {
  const violations: ConcreteTermViolation[] = []
  for (const file of trackedFiles(repoRoot)) {
    const source = readTrackedSource(repoRoot, file)
    if (source === undefined) continue
    violations.push(...findConcreteTermViolations(file, source))
  }
  return violations
}

const invokedPath = process.argv[1]
const isMain = invokedPath !== undefined && import.meta.url === pathToFileURL(resolve(invokedPath)).href
if (isMain) {
  const violations = scanRepository(root)
  if (violations.length === 0) {
    console.log(`verify-concrete-terms: maintained tracked files contain no ${blockedTerm}.`)
  } else {
    console.error(`verify-concrete-terms: ${blockedTerm} is forbidden; name the exact source, field, identity, or evidence:`)
    for (const violation of violations) {
      console.error(violation.line === null
        ? `  ${violation.file} (path)`
        : `  ${violation.file}:${String(violation.line)}`)
    }
    process.exitCode = 1
  }
}
