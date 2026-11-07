/** The WikiMan page tree and the seat occupied by the native editor. */
import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react'
import {
  Button, IconChevronDownOutlineRegular, IconChevronRightOutlineRegular, IconEllipsisOutlineRegular,
  Input, Menu, MenuItemButton, Tooltip,
} from '@deepseek-ai/dsh-client-ui-primitives'
import type { InjectFace, PropsLocale } from '@deepseek-ai/dsh-client-ui-slots'
import type { WikiCreatePage, WikiListing, WikiPage, WikiPageChange, WikiRuntime } from '../types.ts'
import { LibraryTools, type LibraryActions } from './library-tools.tsx'
import { PageMaintenance } from './page-maintenance.tsx'
import { NewPageDialog } from './new-page.tsx'
import { PageTypeIcon, pageTypeLabels } from './page-type.tsx'
import type { EditorActions, WorkbenchSnapshot } from './work-area.tsx'
import css from './wiki.module.css'

/** Page listing, selection and editor placement supplied by the WikiMan owner. */
export interface PanelActions extends Pick<EditorActions, 'subscribe' | 'select'>, LibraryActions {
  readonly runtime: WikiRuntime
  readonly getSnapshot: () => WorkbenchSnapshot
  readonly pages: (query: string, cursor: string, signal: AbortSignal) => Promise<WikiListing<WikiPage>>
  readonly create: (request: WikiCreatePage, signal: AbortSignal) => Promise<WikiPage>
  readonly maintenancePages: (signal: AbortSignal) => Promise<readonly WikiPage[]>
  readonly change: (request: WikiPageChange, signal: AbortSignal) => Promise<void>
  readonly attach: (anchor: HTMLDivElement | null) => void
}

type PanelProps = InjectFace<PanelActions> & PropsLocale<'wikiman'>
type TreeRow = { readonly page: WikiPage; readonly depth: number; readonly hasChildren: boolean }

function treeRows(pages: readonly WikiPage[], query: string, collapsed: ReadonlySet<string>): TreeRow[] {
  if (query) return pages.map(page => ({ page, depth: 0, hasChildren: false }))
  const ids = new Set(pages.map(page => page.pageId))
  const children = new Map<string | null, WikiPage[]>()
  for (const page of pages) {
    const parent = page.parentId && ids.has(page.parentId) ? page.parentId : null
    const siblings = children.get(parent) ?? []
    siblings.push(page)
    children.set(parent, siblings)
  }
  for (const siblings of children.values()) {
    siblings.sort((a, b) => a.rank < b.rank ? -1 : a.rank > b.rank ? 1 : a.pageId.localeCompare(b.pageId))
  }
  const rowOf = (page: WikiPage, depth: number): TreeRow => ({ page, depth, hasChildren: children.has(page.pageId) })
  const pending: TreeRow[] = (children.get(null) ?? []).map(page => rowOf(page, 0)).reverse()
  const rows: TreeRow[] = []
  for (let row = pending.pop(); row; row = pending.pop()) {
    rows.push(row)
    if (collapsed.has(row.page.pageId)) continue
    const descendants = children.get(row.page.pageId) ?? []
    for (const page of descendants.toReversed()) pending.push(rowOf(page, row.depth + 1))
  }
  return rows
}

/**
 * Browse the WikiMan page hierarchy beside the original editor.
 * @param props - localized controls and authorized page operations.
 * @returns the WikiMan workbench.
 */
export function WikiPanel({
  runtime, pages, create, maintenancePages, change, importLibrary, syncLibrary, select, subscribe, getSnapshot, attach, t,
}: PanelProps) {
  const { target } = useSyncExternalStore(subscribe, getSnapshot)
  const [tree, setTree] = useState<WikiListing<WikiPage>>({ items: [], nextCursor: '' })
  const [cursor, setCursor] = useState('')
  const [query, setQuery] = useState('')
  const [refresh, setRefresh] = useState(0)
  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading')
  const choseInitial = useRef(false)
  const [menu, setMenu] = useState(false)
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(() => new Set())
  const [libraryTools, setLibraryTools] = useState(false)
  const [creating, setCreating] = useState(false)
  const [maintenance, setMaintenance] = useState<{ page: WikiPage | null } | null>(null)
  const titleQuery = query.trim()
  const rows = useMemo(() => treeRows(tree.items, titleQuery, collapsed), [tree.items, titleQuery, collapsed])
  const pageId = target?.kind === 'document' ? target.document.pageId : target?.page.pageId

  const activePage = tree.items.find(page => page.pageId === pageId)

  useEffect(() => {
    const abort = new AbortController()
    setStatus('loading')
    void pages(titleQuery, cursor, abort.signal).then((value) => {
      if (abort.signal.aborted) return
      setTree(previous => cursor ? { ...value, items: [...previous.items, ...value.items] } : value)
      setStatus('ready')
    }, () => { if (!abort.signal.aborted) setStatus('error') })
    return () => { abort.abort() }
  }, [pages, titleQuery, cursor, refresh])

  useEffect(() => {
    if (target) { choseInitial.current = true; return }
    if (choseInitial.current || titleQuery || tree.items.length === 0) return
    const parents = new Set(tree.items.map(item => item.parentId))
    const wiki = tree.items.find(item => item.type === 'wiki' && !parents.has(item.pageId))
    const docx = tree.items.find(item => item.type === 'docx')
    const page = wiki ?? docx ?? tree.items.find(item => item.type === 'wiki')
    choseInitial.current = true
    if (page) select({ kind: 'page', page })
  }, [select, target, tree.items, titleQuery])

  useEffect(() => {
    if (!pageId) return
    const ancestors = new Set<string>()
    const byId = new Map(tree.items.map(page => [page.pageId, page]))
    let id = byId.get(pageId)?.parentId
    while (id && !ancestors.has(id)) {
      ancestors.add(id)
      id = byId.get(id)?.parentId
    }
    setCollapsed((previous) => {
      if (!Array.from(ancestors).some(id => previous.has(id))) return previous
      return new Set(Array.from(previous).filter(id => !ancestors.has(id)))
    })
  }, [pageId, tree.items])

  function refreshPages() {
    setCursor('')
    setRefresh(value => value + 1)
  }

  function manage(page: WikiPage | null) {
    setMaintenance({ page })
    select(null)
  }

  return <main className={css.siyuanPage}>
    <div className={css.columns}>
      <nav className={css.list} aria-label={t('pages')}>
        {<header className={css.pageTreeHeader}>
          <h1>{t('title')}</h1>
          <Menu open={menu} onClose={() => { setMenu(false) }} portal autoFocus align="end" anchor={
            <Tooltip label={t('pageActions')} portal>
              <Button size="sm" aria-label={t('pageActions')} aria-haspopup="menu" aria-expanded={menu}
                onClick={() => { setMenu(value => !value) }}>
                <IconEllipsisOutlineRegular size={16} aria-hidden="true" />
              </Button>
            </Tooltip>
          }>
            <MenuItemButton onSelect={() => { setMenu(false); setCreating(true) }}>{t('newPage')}</MenuItemButton>
            <MenuItemButton disabled={!activePage} onSelect={() => {
              setMenu(false)
              if (activePage) manage(activePage)
            }}>{t('managePage')}</MenuItemButton>
            <MenuItemButton onSelect={() => { setMenu(false); manage(null) }}>{t('trash')}</MenuItemButton>
            {runtime === 'portable' && <MenuItemButton onSelect={() => { setMenu(false); setLibraryTools(true); select(null) }}>{t('wikiLibraryTools')}</MenuItemButton>}
            <MenuItemButton onSelect={() => { setMenu(false); refreshPages() }}>{t('refresh')}</MenuItemButton>
            {target && <MenuItemButton onSelect={() => { setMenu(false); select(null) }}>{t('saveClose')}</MenuItemButton>}
          </Menu>
        </header>}
        <Input aria-label={t('findPages')} placeholder={t('findPages')} value={query}
          onChange={(event) => { setCursor(''); setQuery(event.target.value) }} />
        {status === 'loading' && <p role="status">{t('loading')}</p>}
        {status === 'error' && <p role="alert">{t('listError')}</p>}
        {status === 'ready' && rows.length === 0 && <p>{query ? t('noPagesFound') : t('emptyPages')}</p>}
        {rows.map(({ page, depth, hasChildren }) => <div key={page.pageId} className={css.treeRow}
          style={{ marginInlineStart: Math.min(depth, 6) * 12 }}>
          {hasChildren ? <Button size="sm" className={css.disclosure} title={collapsed.has(page.pageId) ? t('expandPage') : t('collapsePage')}
            aria-label={collapsed.has(page.pageId) ? t('expandPage') : t('collapsePage')} aria-expanded={!collapsed.has(page.pageId)}
            onClick={() => {
              setCollapsed((previous) => {
                const next = new Set(previous)
                if (next.has(page.pageId)) next.delete(page.pageId)
                else next.add(page.pageId)
                return next
              })
            }}>
            {collapsed.has(page.pageId)
              ? <IconChevronRightOutlineRegular size={12} aria-hidden="true" />
              : <IconChevronDownOutlineRegular size={12} aria-hidden="true" />}
          </Button> : <span className={css.disclosureGap} />}
          <Button size="sm" className={css.row}
            icon={<PageTypeIcon type={page.type} />}
            aria-pressed={page.pageId === pageId}
            title={`${page.name} · ${t(pageTypeLabels[page.type])}`}
            onClick={() => { if (page.pageId !== pageId) select({ kind: 'page', page }) }}>
            <span>{page.name}</span>
          </Button>
        </div>)}
        {tree.nextCursor && <Button size="sm" disabled={status === 'loading'} onClick={() => { setCursor(tree.nextCursor) }}>{t('more')}</Button>}
      </nav>
      <div ref={attach} className={css.anchor}>
        {!target && <section className={css.emptyEditor}><p>{t('choosePage')}</p></section>}
      </div>
    </div>
    {libraryTools && <LibraryTools importLibrary={importLibrary} syncLibrary={syncLibrary} t={t}
      onClose={() => { setLibraryTools(false) }} onSync={() => {
        setCursor('')
        setRefresh(value => value + 1)
      }} />}
    {maintenance && <PageMaintenance page={maintenance.page} load={maintenancePages} change={change}
      onClose={() => { setMaintenance(null) }} onChanged={() => {
        setMaintenance(null)
        setCursor('')
        setRefresh(value => value + 1)
      }} t={t} />}
    {creating && <NewPageDialog parent={activePage} create={create}
      onClose={() => { setCreating(false) }} onCreated={(page) => {
        setCreating(false)
        setCursor('')
        setQuery('')
        setRefresh(value => value + 1)
        select({ kind: 'page', page })
      }} t={t} />}
  </main>
}
