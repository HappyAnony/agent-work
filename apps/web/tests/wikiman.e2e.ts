/** One portable WikiMan document is edited, read by the Agent and reopened from its citation. */
import { cp, mkdir, mkdtemp, readFile, readdir, realpath, rename, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { ToolCallId, type StreamChunk } from '@deepseek-ai/dsh-llm'
import type { ReplayEntry } from '@deepseek-ai/dsh-llm-replay'
import { chromium, type Page } from 'playwright'
import { describe, expect, it } from 'vitest'
import { z } from 'zod'
import { launchWebScaffold, readPersistedEvents, webSnapshotMode, type WebScaffold } from './scaffold.ts'
import { connectFreshWorkspace, newEnglishPage, REPO_ROOT } from './support.ts'

const ironman = process.env.DSH_WIKIMAN_IRONMAN
const runtimeResources = process.env.DSH_WIKIMAN_RESOURCES ?? (ironman && join(ironman, 'src-tauri/target/package-resources'))
const portableFixture = join(REPO_ROOT, 'packages/experimental/wikiman/tests/fixtures/portable')

function requireRuntimeResources(): string {
  if (!runtimeResources) throw new Error('DSH_WIKIMAN_RESOURCES is required')
  return runtimeResources
}
const tablePageId = '1760000000000000003'
const firstWikiId = '1760000000000000001'
const secondWikiId = '1760000000000000004'
const marker = 'Release code XQ712'
const document = {
  pageId: '1760000000000000002',
  boxId: '20261019090000-box0001',
  docId: '20261019090001-doc0001',
}

function tool(id: ToolCallId, name: string, input: object): ReplayEntry {
  const args = JSON.stringify(input)
  return { kind: 'chunks', chunks: [
    { type: 'block-start', index: 0, blockType: 'tool-call' },
    { type: 'tool-call-delta', index: 0, id, name, argumentsDelta: args },
    { type: 'block-end', index: 0, block: { type: 'tool-call', id, name, arguments: args } },
    { type: 'usage', usage: { inputTokens: 10, outputTokens: 10 } },
    { type: 'finish', reason: { kind: 'tool-calls' } },
  ] }
}

function answer(): ReplayEntry {
  const text = 'The Wiki release code is XQ712. See the Wiki source in this turn.'
  const chunks: StreamChunk[] = [
    { type: 'block-start', index: 0, blockType: 'text' },
    { type: 'text-delta', index: 0, text },
    { type: 'block-end', index: 0, block: { type: 'text', text } },
    { type: 'usage', usage: { inputTokens: 20, outputTokens: 12 } },
    { type: 'finish', reason: { kind: 'stop' } },
  ]
  return { kind: 'chunks', chunks }
}

describe.skipIf(runtimeResources === undefined || webSnapshotMode() === 'record')('Web WikiMan with real portable resources', () => {
  it('persists a browser edit and opens the committed document from a logged tool result', async () => {
    const root = await realpath(await mkdtemp(join(tmpdir(), 'dsh-wikiman-e2e-')))
    try {
      const sourceRoot = join(root, 'wiki')
      const workspaceRoot = join(root, 'workspace')
      const resources = requireRuntimeResources()
      await Promise.all([
        cp(portableFixture, sourceRoot, { recursive: true }),
        mkdir(workspaceRoot),
      ])
      const overlay = join(root, 'wiki.patch.yml')
      const replayOverride = join(root, 'replay.override.json')
      await Promise.all([
        writeFile(overlay, JSON.stringify([
          { insert: [{ id: 'wikiman', name: '@deepseek-ai/dsh-wikiman', config: {
            kernelPath: join(resources, 'bin/wiki-kernel'), appDir: join(resources, 'wiki-app'), sourceRoot, workspaceRoot,
          } }] },
        ])),
        writeFile(replayOverride, JSON.stringify([
          tool(ToolCallId('wiki-search-fixture'), 'wiki_search', { query: 'XQ712' }),
          tool(ToolCallId('wiki-read-fixture'), 'wiki_read', document),
          answer(),
        ])),
      ])
      const scaffold = await launchWebScaffold({
        extraOverlayPath: overlay,
        extraInstallAnchors: [join(REPO_ROOT, 'packages/experimental/wikiman/package.json')],
        replayFixture: join(root, 'override-only.jsonl'),
        replayOverride,
      })
      let testFailure: unknown
      try {
        const browser = await chromium.launch()
        try {
          const page = await newEnglishPage(browser)
          await page.goto(scaffold.authenticatedUrl)
          await connectFreshWorkspace(page, root)
          await page.getByRole('button', { name: 'Knowledge base', exact: true }).click()
          await page.getByRole('button', { name: 'New page', exact: true }).click()
          const creation = page.getByRole('dialog', { name: 'New page' })
          await creation.getByRole('textbox', { name: 'Page name' }).fill('Browser-created Wiki page')
          await creation.getByRole('button', { name: 'Create page' }).click()
          await creation.waitFor({ state: 'hidden' })
          const search = page.getByRole('textbox', { name: 'Find pages' })
          await search.fill('Browser-created')
          await page.getByRole('button', { name: 'Browser-created Wiki page', exact: true }).waitFor()
          await search.clear()
          await page.getByRole('button', { name: '技术知识', exact: true }).first().click()
          await page.getByRole('heading', { name: '技术知识', exact: true }).waitFor()
          await page.frameLocator('iframe[title="WikiMan editor"]').locator('body[data-editor-ready="true"]').waitFor()
          await page.getByRole('button', { name: '单文档示例', exact: true }).click()
          const frame = page.frameLocator('iframe[title="WikiMan editor"]')
          const paragraph = frame.locator('[data-node-id="20261019090002-par0001"] [contenteditable="true"]')
          await paragraph.waitFor()
          const iframe = page.locator('iframe[title="WikiMan editor"]')
          const wide = page.viewportSize()
          await page.setViewportSize({ width: 390, height: 820 })
          await expect.poll(async () => {
            const bounds = await iframe.boundingBox()
            return bounds !== null && bounds.x >= 0 && bounds.x + bounds.width <= 391 && bounds.height > 0
          }).toBe(true)
          if (wide) await page.setViewportSize(wide)
          await paragraph.click()
          await page.keyboard.press('End')
          await page.keyboard.type(` ${marker}`)
          await iframe.evaluate((element) => { element.setAttribute('data-wiki-test-frame', 'original') })
          const newSession = page.getByRole('button', { name: 'New session', exact: true }).filter({ hasText: 'New Session' })
          await newSession.click()
          await iframe.waitFor({ state: 'hidden' })
          await page.getByRole('button', { name: 'Knowledge base', exact: true }).click()
          await iframe.waitFor({ state: 'visible' })
          expect(await iframe.getAttribute('data-wiki-test-frame')).toBe('original')
          await page.getByRole('button', { name: 'Save and close' }).click()
          await page.locator('iframe[title="WikiMan editor"]').waitFor({ state: 'hidden' })

          await newSession.click()
          const composer = page.locator('[data-composer-input][contenteditable="true"]')
          await composer.fill('Find the updated release code in the Wiki and cite the document.')
          const [sessionId] = await Promise.all([scaffold.whenTurnSettled(), composer.press('Enter')])
          const events = await readPersistedEvents(scaffold, sessionId)
          const toolResults = events.filter(event => event.type === 'tool/result')
          expect(toolResults).toHaveLength(2)
          expect(JSON.stringify(toolResults[0]?.data)).toContain(document.docId)
          expect(JSON.stringify(toolResults[1]?.data)).toContain(marker)
          const citation = page.getByRole('button', { name: 'Open Wiki source' })
          const process = page.locator('[data-turn-process]')
          await process.waitFor()
          if (await process.getAttribute('aria-expanded') === 'false') await process.click()
          for (const group of await page.locator('[data-process-activity][aria-expanded="false"]:visible').all()) {
            await group.click()
          }
          await citation.click()
          await expect.poll(() => paragraph.textContent()).toContain(marker)
          await page.getByRole('button', { name: 'Save and close' }).click()
          await page.locator('iframe[title="WikiMan editor"]').waitFor({ state: 'hidden' })
        } finally {
          await browser.close()
        }
      } catch (error) {
        testFailure = error
        throw error
      } finally {
        await scaffold.close().catch((error: unknown) => {
          if (testFailure !== undefined) throw new AggregateError([testFailure, error], 'WikiMan test and teardown failed')
          throw error
        })
      }
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })
})

async function loadLibrary(page: Page, scaffold: WebScaffold) {
  await page.goto(scaffold.authenticatedUrl)
  await page.getByRole('button', { name: 'Knowledge base', exact: true }).click()
  const frame = page.frameLocator('iframe[title="WikiMan editor"]')
  await frame.locator('body[data-editor-ready="true"]').waitFor()
  await page.getByRole('button', { name: 'Save and close' }).click()
  await page.locator('iframe[title="WikiMan editor"]').waitFor({ state: 'hidden' })
}

async function renamePage(page: Page, oldName: string, newName: string) {
  await page.getByRole('button', { name: oldName, exact: true }).click()
  await page.getByRole('button', { name: 'Manage page', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Manage page' })
  await dialog.getByRole('textbox', { name: 'Page name' }).fill(newName)
  await dialog.getByRole('button', { name: 'Rename page' }).click()
  await dialog.waitFor({ state: 'hidden' })
  await page.getByRole('button', { name: newName, exact: true }).waitFor()
}

async function portablePeer(root: string, resources: string, shared: string): Promise<WebScaffold> {
  const overlay = join(root, 'wikiman.patch.yml')
  await writeFile(overlay, JSON.stringify([{ insert: [{ id: 'wikiman', name: '@deepseek-ai/dsh-wikiman', config: {
    kernelPath: join(resources, 'bin/wiki-kernel'), appDir: join(resources, 'wiki-app'), sourceRoot: root,
    workspaceRoot: root, syncDirectory: shared,
  } }] }]))
  return await launchWebScaffold({
    extraOverlayPath: overlay,
    extraInstallAnchors: [join(REPO_ROOT, 'packages/experimental/wikiman/package.json')],
  })
}

async function syncFromUI(page: Page, outcome: string) {
  await page.getByRole('button', { name: 'Library tools', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Library tools' })
  await dialog.getByRole('button', { name: 'Sync now', exact: true }).click()
  const expected = dialog.getByText(outcome, { exact: true })
  await expected.or(dialog.getByRole('alert')).first().waitFor()
  if (!await expected.isVisible()) throw new Error(`Sync expected ${outcome}; received ${await dialog.getByRole('alert').innerText()}`)
  await dialog.getByRole('button', { name: 'Close', exact: true }).click()
  await dialog.waitFor({ state: 'hidden' })
}

async function legacyFixture(portable: string, legacy: string) {
  const { DatabaseSync } = await import('node:sqlite')
  await mkdir(join(legacy, 'pages'), { recursive: true })
  const db = new DatabaseSync(join(legacy, 'page.db'))
  try {
    db.exec(`CREATE TABLE pages (page_id TEXT, name TEXT, parent_id TEXT, rank TEXT, create_at INTEGER, update_at INTEGER, type INTEGER, ext TEXT);
      CREATE TABLE page_tree_tombstones (page_id TEXT);
      INSERT INTO page_tree_tombstones VALUES ('1760000000000000099');`)
    const insert = db.prepare('INSERT INTO pages VALUES (?, ?, ?, ?, ?, ?, ?, ?)')
    insert.run(firstWikiId, 'Legacy knowledge', null, '0001', 1792371600, 1792371600000, 1, '{"counter":9223372036854775807}')
    insert.run(document.pageId, '单文档示例', firstWikiId, '0001', 1792371600, 1792371600000, 0, '{}')
  } finally {
    db.close()
  }
  const data = join(legacy, 'pages', document.pageId, 'data')
  await cp(join(portable, 'pages', document.pageId, 'data'), data, { recursive: true })
  const file = join(data, document.boxId, document.docId)
  await rename(file + '.docj', file + '.sy')
}

describe.skipIf(runtimeResources === undefined || webSnapshotMode() === 'record')('WikiMan library maintenance', () => {
  it('moves and restores pages, imports a checked legacy copy and preserves a two-way sync conflict', async () => {
    const root = await realpath(await mkdtemp(join(tmpdir(), 'dsh-wikiman-maintenance-')))
    const scaffolds = new Set<WebScaffold>()
    const browser = await chromium.launch()
    try {
      const fixture = portableFixture
      const resources = requireRuntimeResources()
      const a = join(root, 'first')
      const b = join(root, 'second')
      const shared = join(root, 'shared')
      const legacy = join(root, 'legacy')
      const imported = join(root, 'imports')
      await Promise.all([
        cp(fixture, a, { recursive: true, filter: source => !source.includes('/.local') }),
        cp(fixture, b, { recursive: true, filter: source => !source.includes('/.local') }),
        mkdir(shared), mkdir(imported), legacyFixture(fixture, legacy),
      ])
      const originalDb = await readFile(join(legacy, 'page.db'))
      const first = await portablePeer(a, resources, shared)
      scaffolds.add(first)
      const second = await portablePeer(b, resources, shared)
      scaffolds.add(second)
      const left = await newEnglishPage(browser)
      const right = await newEnglishPage(browser)
      await Promise.all([loadLibrary(left, first), loadLibrary(right, second)])
      await syncFromUI(left, 'Published the local version.')
      await syncFromUI(right, 'Both copies have the same version.')

      await left.getByRole('button', { name: '空表格节点', exact: true }).click()
      await left.getByRole('button', { name: 'Manage page', exact: true }).click()
      const move = left.getByRole('dialog', { name: 'Manage page' })
      await move.getByRole('combobox', { name: 'New parent' }).selectOption(secondWikiId)
      await move.getByRole('textbox', { name: 'Sort rank', exact: true }).fill('0010')
      await move.getByRole('button', { name: 'Save position', exact: true }).click()
      await move.waitFor({ state: 'hidden' })
      const table = join('pages', tablePageId, 'page.json')
      expect(z.object({ parentId: z.string(), rank: z.string() }).parse(JSON.parse(await readFile(join(a, table), 'utf8'))))
        .toEqual({ parentId: secondWikiId, rank: '0010' })

      await renamePage(left, '空表格节点', 'Moved table')
      await left.getByRole('button', { name: 'Moved table', exact: true }).click()
      await left.getByRole('button', { name: 'Manage page', exact: true }).click()
      await move.getByRole('button', { name: 'Move to recycle bin', exact: true }).click()
      await move.getByRole('button', { name: 'Recycle page and descendants', exact: true }).click()
      await move.waitFor({ state: 'hidden' })
      await left.getByRole('button', { name: 'Moved table', exact: true }).waitFor({ state: 'hidden' })
      await left.getByRole('button', { name: 'Recycle bin', exact: true }).click()
      const bin = left.getByRole('dialog', { name: 'Recycle bin' })
      await bin.getByText('Moved table', { exact: true }).waitFor()
      await bin.getByRole('button', { name: 'Restore', exact: true }).click()
      await bin.waitFor({ state: 'hidden' })
      await left.getByRole('button', { name: 'Moved table', exact: true }).waitFor()

      await left.getByRole('button', { name: 'Library tools', exact: true }).click()
      const library = left.getByRole('dialog', { name: 'Library tools' })
      await library.getByRole('button', { name: 'Import legacy library', exact: true }).click()
      await library.getByRole('textbox', { name: 'Legacy library folder' }).fill(legacy)
      await library.getByRole('textbox', { name: 'Destination folder' }).fill(imported)
      await library.getByRole('textbox', { name: 'New library name' }).fill('Imported Wiki')
      await library.getByRole('button', { name: 'Preview import', exact: true }).click()
      await library.getByText('Legacy deletion records', { exact: true }).waitFor()
      expect(await readdir(imported)).toEqual([])
      const legacyAsset = join(legacy, 'pages', document.pageId, 'data/assets/fixture.svg')
      const asset = await readFile(legacyAsset)
      await writeFile(legacyAsset, Buffer.concat([asset, Buffer.from('\n')]))
      await library.getByRole('button', { name: 'Create portable copy', exact: true }).click()
      await library.getByText('The preview no longer matches the source. Preview again before importing.', { exact: true }).waitFor()
      expect(await readdir(imported)).toEqual([])
      await writeFile(legacyAsset, asset)
      await library.getByRole('button', { name: 'Preview import', exact: true }).click()
      await library.getByRole('button', { name: 'Create portable copy', exact: true }).click()
      await library.getByText('New portable library', { exact: true }).waitFor()
      const names = await readdir(imported)
      expect(names).toHaveLength(1)
      const output = join(imported, names[0] ?? '')
      expect(await readFile(join(output, 'pages', firstWikiId, 'page.json'), 'utf8')).toContain('9223372036854775807')
      const doc = join('pages', document.pageId, 'data', document.boxId, document.docId)
      expect(await readFile(join(output, doc + '.docj'))).toEqual(await readFile(join(legacy, doc + '.sy')))
      expect(await readFile(join(legacy, 'page.db'))).toEqual(originalDb)
      await library.getByRole('button', { name: 'Close', exact: true }).click()

      await syncFromUI(left, 'Published the local version.')
      await syncFromUI(right, 'Received and opened the shared version.')
      await right.getByRole('button', { name: 'Moved table', exact: true }).waitFor()
      expect(z.object({ name: z.string() }).parse(JSON.parse(await readFile(join(b, table), 'utf8'))).name).toBe('空表格节点')
      await renamePage(right, 'Moved table', 'Returned from peer')
      await syncFromUI(right, 'Published the local version.')
      await syncFromUI(left, 'Received and opened the shared version.')
      await left.getByRole('button', { name: 'Returned from peer', exact: true }).waitFor()

      await Promise.all([
        renamePage(left, 'Returned from peer', 'Local conflicting title'),
        renamePage(right, 'Returned from peer', 'Shared conflicting title'),
      ])
      await syncFromUI(right, 'Published the local version.')
      await syncFromUI(left, 'Both copies have different changes. The local and shared versions are preserved in the sync conflicts folder.')
      await left.getByRole('button', { name: 'Local conflicting title', exact: true }).waitFor()
      const conflicts = join(a, '.local/dsh-wikiman-sync/conflicts')
      const versions = await Promise.all((await readdir(conflicts)).map(async (digest) => {
        return z.object({ name: z.string() }).parse(JSON.parse(await readFile(join(conflicts, digest, table), 'utf8'))).name
      }))
      expect(versions.sort()).toEqual(['Local conflicting title', 'Shared conflicting title'])

      const remote = join(shared, (await readdir(shared))[0] ?? '')
      const head = z.object({ digest: z.string() }).parse(JSON.parse(await readFile(join(remote, 'HEAD.json'), 'utf8'))).digest
      await writeFile(join(remote, 'generations', head, table), '{}')
      await syncFromUI(left, 'Source validation failed. Check the portable files and retry with a verified copy.')
      await left.getByRole('button', { name: 'Local conflicting title', exact: true }).waitFor()
      await Promise.all([left.close(), right.close()])
      await second.close()
      scaffolds.delete(second)
      await first.close()
      scaffolds.delete(first)
      const restarted = await portablePeer(a, resources, shared)
      scaffolds.add(restarted)
      const reopened = await newEnglishPage(browser)
      await loadLibrary(reopened, restarted)
      await reopened.getByRole('button', { name: 'Local conflicting title', exact: true }).waitFor()
    } finally {
      const failures: unknown[] = []
      await browser.close().catch((error: unknown) => failures.push(error))
      for (const scaffold of [...scaffolds].reverse()) {
        await scaffold.close().catch((error: unknown) => failures.push(error))
      }
      await rm(root, { recursive: true, force: true })
      if (failures.length) throw new AggregateError(failures, 'WikiMan peer teardown failed')
    }
  })
})

const secondDocId = '20261019090500-doc0002'
const originalParagraph = '20261019090002-par0001'

async function siyuanFixture(portable: string, legacy: string) {
  const { DatabaseSync } = await import('node:sqlite')
  await legacyFixture(portable, legacy)
  const db = new DatabaseSync(join(legacy, 'page.db'))
  try {
    db.prepare('UPDATE pages SET type = 1, name = ? WHERE page_id = ?').run('Project overview', document.pageId)
  } finally {
    db.close()
  }
  const notebook = join(legacy, 'pages', document.pageId, 'data', document.boxId)
  await mkdir(join(notebook, '.siyuan'))
  await writeFile(join(notebook, '.siyuan/conf.json'), JSON.stringify({ name: 'TodoList', closed: false, sortMode: 15 }))
  const original = join(notebook, document.docId)
  await rename(original + '.sy', original + '.docj')
  const rootSchema = z.looseObject({ Properties: z.record(z.string(), z.string()) })
  const ast = rootSchema.parse(JSON.parse(await readFile(original + '.docj', 'utf8')))
  await writeFile(original + '.docj', JSON.stringify({ ...ast, Properties: { ...ast.Properties, title: 'Overview' } }))
  await writeFile(join(notebook, secondDocId + '.docj'), JSON.stringify({
    ID: secondDocId, Spec: '1', Type: 'NodeDocument',
    Properties: { id: secondDocId, title: 'Reading list', type: 'doc', updated: '20261019090500' },
    Children: [{
      ID: '20261019090501-par0002', Type: 'NodeParagraph', Properties: { id: '20261019090501-par0002' },
      Children: [{ Type: 'NodeText', Data: 'Original SiYuan document tab' }],
    }],
  }))
}

describe.skipIf(runtimeResources === undefined || webSnapshotMode() === 'record')('WikiMan original SiYuan workbench', () => {
  it('uses the original document tree and tabs, then reads and reopens a native edit', async () => {
    const root = await realpath(await mkdtemp(join(tmpdir(), 'dsh-siyuan-e2e-')))
    try {
      const sourceRoot = join(root, 'wikiman')
      const workspaceRoot = join(root, 'workspace')
      const resources = requireRuntimeResources()
      await Promise.all([
        siyuanFixture(portableFixture, sourceRoot),
        mkdir(workspaceRoot),
      ])
      const overlay = join(root, 'wiki.patch.yml')
      const replayOverride = join(root, 'replay.override.json')
      await Promise.all([
        writeFile(overlay, JSON.stringify([{ insert: [{ id: 'wikiman', name: '@deepseek-ai/dsh-wikiman', config: {
          runtime: 'siyuan', kernelPath: join(resources, 'bin/siyuan-kernel'), appDir: join(resources, 'siyuan-app'),
          sourceRoot, workspaceRoot,
        } }] }])),
        writeFile(replayOverride, JSON.stringify([
          tool(ToolCallId('siyuan-search-fixture'), 'wiki_search', { query: 'XQ712' }),
          tool(ToolCallId('siyuan-read-fixture'), 'wiki_read', document),
          answer(),
        ])),
      ])
      const scaffold = await launchWebScaffold({
        extraOverlayPath: overlay,
        extraInstallAnchors: [join(REPO_ROOT, 'packages/experimental/wikiman/package.json')],
        replayFixture: join(root, 'override-only.jsonl'),
        replayOverride,
      })
      let failure: unknown
      try {
        const browser = await chromium.launch()
        try {
          const page = await newEnglishPage(browser)
          await page.goto(scaffold.authenticatedUrl)
          await connectFreshWorkspace(page, root)
          await page.getByRole('button', { name: 'Knowledge base', exact: true }).click()
          const outer = page.locator('iframe[title="WikiMan editor"]')
          const frame = outer.contentFrame().frameLocator('iframe[title="SiYuan"]')
          const nav = frame.locator('.file-tree:visible')
          const notebook = nav.getByText('TodoList', { exact: true })
          await notebook.waitFor({ timeout: 15_000 }).catch(async (error: unknown) => {
            const paths = page.frames().map((item) => {
              const path = new URL(item.url() || 'about:blank').pathname
              return path.startsWith('/stage/') || path === '/check-auth' ? path : '(host or authentication)'
            })
            const alerts = await page.getByRole('alert').allTextContents()
            const headings = await page.getByRole('heading').allTextContents()
            throw new Error(`Native workbench failed to show its notebook (${paths.join(', ')}). ${alerts.join(' ')} ${headings.join(' ')}`, { cause: error })
          })
          expect(await frame.locator('html').evaluate(() => [location.pathname, new URLSearchParams(location.search).get('wikiman_reload')]))
            .toEqual(['/stage/build/desktop/', '0'])
          const files = nav.locator('li[data-node-id="' + secondDocId + '"]')
          const toggle = nav.locator('ul[data-url="' + document.boxId + '"] > li[data-type="navigation-root"] .b3-list-item__toggle')
          if (!await files.isVisible() && !await toggle.evaluate(element => element.querySelector('.b3-list-item__arrow--open') !== null)) {
            await toggle.click()
          }
          const tabs = frame.locator('.layout-tab-bar:not(.layout-tab-bar--readonly)')
          await tabs.getByText('Overview', { exact: true }).waitFor()
          await files.getByText('Reading list', { exact: true }).click()
          await tabs.filter({ hasText: 'Overview' }).getByText('Reading list', { exact: true }).waitFor()
          await tabs.getByText('Overview', { exact: true }).click()
          await page.emulateMedia({ colorScheme: 'dark' })
          await expect.poll(() => frame.locator('html').getAttribute('data-theme-mode')).toBe('dark')
          const viewport = page.viewportSize()
          await page.getByRole('button', { name: 'Collapse sidebar' }).click()
          await page.setViewportSize({ width: 390, height: 820 })
          await expect.poll(async () => {
            const bounds = await outer.boundingBox()
            return bounds !== null && bounds.x >= 0 && bounds.x + bounds.width <= 391 && bounds.width > 300
          }).toBe(true)
          if (await nav.isVisible()) await frame.locator('#dockLeft [data-type="file"]').click()
          const body = frame.locator('.protyle-wysiwyg:visible').first()
          await expect.poll(() => body.evaluate((element) => {
            const style = getComputedStyle(element)
            return element.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight)
          })).toBeGreaterThan(200)
          if (viewport) await page.setViewportSize(viewport)
          await page.getByRole('button', { name: 'Open sidebar' }).click()
          const paragraph = frame.locator('[data-node-id="' + originalParagraph + '"] [contenteditable="true"]:visible')
          await paragraph.click()
          await page.keyboard.press('End')
          await page.keyboard.type(' ' + marker)
          await tabs.getByText('Reading list', { exact: true }).click()
          await frame.getByText('Original SiYuan document tab', { exact: true }).waitFor()
          await page.getByRole('button', { name: 'Page actions' }).click()
          await page.getByRole('menuitem', { name: 'Save and close' }).click()
          await outer.waitFor({ state: 'hidden' })
          const committed = join(sourceRoot, 'pages', document.pageId, 'data', document.boxId, document.docId + '.docj')
          expect(await readFile(committed, 'utf8')).toContain(marker)

          await page.getByRole('button', { name: 'New session', exact: true }).filter({ hasText: 'New Session' }).click()
          const composer = page.locator('[data-composer-input][contenteditable="true"]')
          await composer.fill('Find the updated release code in the Wiki and cite the document.')
          const [sessionId] = await Promise.all([scaffold.whenTurnSettled(), composer.press('Enter')])
          const events = await readPersistedEvents(scaffold, sessionId)
          const toolResults = events.filter(event => event.type === 'tool/result')
          expect(toolResults).toHaveLength(2)
          expect(JSON.stringify(toolResults[0]?.data)).toContain(document.docId)
          expect(JSON.stringify(toolResults[1]?.data)).toContain(marker)
          const process = page.locator('[data-turn-process]')
          await process.waitFor()
          if (await process.getAttribute('aria-expanded') === 'false') await process.click()
          for (const group of await page.locator('[data-process-activity][aria-expanded="false"]:visible').all()) {
            await group.click()
          }
          await page.getByRole('button', { name: 'Open Wiki source' }).click()
          await expect.poll(() => paragraph.textContent()).toContain(marker)
          await page.context().close()

          const reopened = await newEnglishPage(browser)
          await reopened.goto(scaffold.authenticatedUrl)
          await reopened.getByRole('button', { name: 'Knowledge base', exact: true }).click()
          const nextEditor = reopened.locator('iframe[title="WikiMan editor"]')
          const nextFrame = nextEditor.contentFrame().frameLocator('iframe[title="SiYuan"]')
          await expect.poll(() => nextFrame.locator('[data-node-id="' + originalParagraph + '"] [contenteditable="true"]:visible').textContent())
            .toContain(marker)
          await reopened.getByRole('button', { name: 'Page actions' }).click()
          await reopened.getByRole('menuitem', { name: 'Save and close' }).click()
          await nextEditor.waitFor({ state: 'hidden' })
        } finally {
          await browser.close()
        }
      } catch (error) {
        failure = error
        throw error
      } finally {
        await scaffold.close().catch((error: unknown) => {
          if (failure !== undefined) throw new AggregateError([failure, error], 'SiYuan test and teardown failed')
          throw error
        })
      }
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })
})
