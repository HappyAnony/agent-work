import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import { inspectDshPackageLicenses } from './verify-dsh-package-licenses.ts'

const roots: string[] = []

afterEach(() => {
  for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true })
})

function writeManifest(root: string, file: string, manifest: Record<string, unknown>): void {
  const path = join(root, file)
  mkdirSync(dirname(path), { recursive: true })
  writeFileSync(path, `${JSON.stringify(manifest, null, 2)}\n`)
}

function createWorkspace(): string {
  const root = mkdtempSync(join(tmpdir(), 'dsh-package-licenses-'))
  roots.push(root)
  writeManifest(root, 'package.json', {
    name: '@deepseek-ai/dsh-root',
    license: 'MIT',
    workspaces: ['apps/*', 'packages/*/*', 'vendor/*'],
  })
  return root
}

describe('DSH package license gate', () => {
  it('checks root, unhyphenated CLI, and dsh-prefixed package names while ignoring other families', () => {
    const root = createWorkspace()
    writeManifest(root, 'apps/cli/package.json', { name: '@deepseek-ai/dsh', license: 'MIT' })
    writeManifest(root, 'packages/core/agent/package.json', {
      name: '@deepseek-ai/dsh-agent',
      license: 'BSD-3-Clause',
    })
    writeManifest(root, 'vendor/cordis/package.json', {
      name: '@deepseek-ai/cordis',
      license: 'BSD-3-Clause',
    })

    expect(inspectDshPackageLicenses(root)).toEqual({
      packageCount: 3,
      failures: [
        'packages/core/agent/package.json: @deepseek-ai/dsh-agent must declare "license": "MIT"; found "BSD-3-Clause".',
      ],
    })
  })

  it('rejects a missing license declaration', () => {
    const root = createWorkspace()
    writeManifest(root, 'packages/core/agent/package.json', { name: '@deepseek-ai/dsh-agent' })

    expect(inspectDshPackageLicenses(root).failures).toEqual([
      'packages/core/agent/package.json: @deepseek-ai/dsh-agent must declare "license": "MIT"; found undefined.',
    ])
  })

  it('requires both source licenses only for the imported WikiMan package', () => {
    const root = createWorkspace()
    const file = 'packages/experimental/wikiman/package.json'
    writeManifest(root, file, { name: '@deepseek-ai/dsh-wikiman', license: 'MIT AND AGPL-3.0-or-later' })
    expect(inspectDshPackageLicenses(root).failures).toEqual([])
    writeManifest(root, file, { name: '@deepseek-ai/dsh-wikiman', license: 'MIT' })
    expect(inspectDshPackageLicenses(root).failures).toEqual([
      `${file}: @deepseek-ai/dsh-wikiman must declare "license": "MIT AND AGPL-3.0-or-later"; found "MIT".`,
    ])
    writeManifest(root, file, { name: '@deepseek-ai/dsh-other', license: 'MIT AND AGPL-3.0-or-later' })
    expect(inspectDshPackageLicenses(root).failures).toEqual([
      `${file}: @deepseek-ai/dsh-other must declare "license": "MIT"; found "MIT AND AGPL-3.0-or-later".`,
    ])
  })

})
