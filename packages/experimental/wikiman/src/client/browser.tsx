/** Browser entry for the WikiMan panel, editor overlay and citation button. */
import type { Context } from '@deepseek-ai/cordis'
import type {} from '@deepseek-ai/dsh-api-remotes/client'
import type {} from '@deepseek-ai/dsh-client-locale/client'
import type { MainPanelId } from '@deepseek-ai/dsh-client-ui-layout/client'
import type {} from '@deepseek-ai/dsh-client-ui-sidebar/client'
import type {} from '@deepseek-ai/dsh-client-ui-renderer/client'
import { Button, IconFolderOpenOutlineRegular } from '@deepseek-ai/dsh-client-ui-primitives'
import type { ToolCallViewProps } from '@deepseek-ai/dsh-client-ui-tool/client'
import type { InjectFace, PropsLocale, PropsRuntime } from '@deepseek-ai/dsh-client-ui-slots'
import type { RemoteResult } from '@deepseek-ai/dsh-typert-protocol'
import remote from '@deepseek-ai/dsh-wikiman/remote'
import type { WikiDocument } from '../types.ts'
import { WikiWorkArea, type EditorActions, type WikiTarget, type WorkbenchSnapshot } from './work-area.tsx'
import { en, zh } from './locales.ts'
import { WikiPanel, type PanelActions } from './panel.tsx'
import { PortableEditor } from './portable-workbench.ts'

declare module '@deepseek-ai/dsh-client-ui-slots' {
  interface LocaleNamespaceMap {
    /** WikiMan workbench and editor copy. */
    wikiman: keyof typeof en
  }
}

/** Browser services supplying the panel, editor and recorded citation row. */
export const inject = ['remote', 'slots', 'locale', 'layout']

const panelId = 'wikiman' as MainPanelId

function Icon({ size }: PropsRuntime<'sidebar.panellist'>) {
  return <IconFolderOpenOutlineRegular size={size} />
}

function sourceOf(meta: unknown): WikiDocument | null {
  if (typeof meta !== 'object' || meta === null || !('source' in meta)) return null
  const value = meta.source
  if (typeof value !== 'object' || value === null || !('pageId' in value) || !('boxId' in value)
    || !('storeId' in value) || !('docId' in value) || !('title' in value) || !('hash' in value)) return null
  const { pageId, boxId, docId, storeId, title, hash } = value
  return typeof pageId === 'string' && typeof boxId === 'string' && typeof docId === 'string'
    && typeof storeId === 'string' && typeof title === 'string' && typeof hash === 'string'
    ? { pageId, boxId, docId, storeId, title, hash }
    : null
}

function ReadRow(props: ToolCallViewProps & PropsLocale<'wikiman'> & InjectFace<{ openCitation: (document: WikiDocument) => void }>) {
  if (props.phase === 'preparing') return <span>{props.t('read')}</span>
  const { block, openCitation, t } = props
  if (!('kind' in block)) return <span>{t('read')}</span>
  if (block.isError) return <span>{t('readError')}</span>
  const source = sourceOf(block.meta)
  return source ? <Button size="sm" onClick={() => { openCitation(source) }}>{t('original')}</Button> : <span>{t('read')}</span>
}

async function unwrap<T>(result: Promise<RemoteResult<T>>): Promise<T> {
  const outcome = await result
  if (!outcome.ok) throw outcome.error
  return outcome.value
}

/**
 * Mount WikiMan Remotes and their panel, editor and result-card contributions.
 * @param ctx - the browser plugin context.
 * @returns the disposer that releases the contributions.
 */
export async function apply(ctx: Context): Promise<() => Promise<void>> {
  const unmount = await ctx.remote.$mount(remote)
  const ui = ctx.inject(['remote.wikiman'], async (scope) => {
    const runtime = await unwrap(scope.remote.wikiman.runtime())
    scope.effect(() => scope.locale.register('wikiman', { en, zh }), 'wikiman: locale')
    const t = scope.locale.bind('wikiman')
    let snapshot: WorkbenchSnapshot = { target: null, anchor: null, changed: false }
    const subscribers = new Set<() => void>()
    const update = (value: WorkbenchSnapshot) => {
      snapshot = value
      for (const listener of subscribers) listener()
    }
    const select = (target: WikiTarget | null) => { update({ ...snapshot, target, changed: false }) }
    const portable = new PortableEditor({
      openEditor: async (_store, _session, pageId) => {
        if (offered?.view.pageId === pageId) {
          const value = offered.view
          takeOffered()
          return value
        }
        return (await unwrap(scope.remote.wikiman.openPage(pageId, lifetime.signal))).view
      },
      closeEditor: view => unwrap(scope.remote.wikiman.closeEditor(view.viewId)),
      reportEditorDirty: () => Promise.resolve(),
      documents: (target, cursor) => unwrap(scope.remote.wikiman.documents(target.pageId, cursor, lifetime.signal)),
      resolve: (target, blockId) => unwrap(scope.remote.wikiman.resolveLink(target.pageId, blockId, lifetime.signal)),
      locale: () => scope.locale.getLocale().active,
      label: kind => t(kind === 'editor' ? 'editor' : 'resizePanes'),
    }, undefined, {
      current: (origin) => {
        const current = snapshot.target
        return (current?.kind === 'page' ? current.page.pageId : current?.document.pageId) === origin.pageId
      },
      select: (document) => { select({ kind: 'document', document, blockId: document.blockId }) },
    })
    const lifetime = new AbortController()
    let offered: import('../types.ts').WikiPageEditor | undefined
    const takeOffered = (): import('../types.ts').WikiPageEditor | undefined => {
      const value = offered
      offered = undefined
      return value
    }
    let opening: Promise<void> = Promise.resolve()
    let selection = 0
    scope.effect(() => async () => {
      lifetime.abort()
      ++selection
      await opening.catch((_error: unknown) => { /* Selection failures are reported by the work area. */ })
      await portable.dispose()
    }, 'wikiman: portable workbench')
    const editor: EditorActions = {
      runtime,
      portable,
      openPortable: (target) => {
        const epoch = ++selection
        const run = opening.catch((_error: unknown) => { /* A failed selection can be retried. */ }).then(async () => {
          if (epoch !== selection) return
          if (!target || target.kind === 'page' && target.page.type === 'table') {
            await portable.release(() => Promise.resolve())
            return
          }
          const prepared = target.kind === 'page'
            ? await unwrap(scope.remote.wikiman.openPage(target.page.pageId, lifetime.signal))
            : await unwrap(scope.remote.wikiman.openEditor(target.document, lifetime.signal))
          offered = prepared
          try {
            if (epoch !== selection) return
            if (snapshot.target === target) update({ ...snapshot, changed: target.kind === 'document' && prepared.document?.hash !== target.document.hash })
            const { baseUrl, view } = prepared
            await portable.open({ baseUrl, ...view }, target.kind === 'document' ? target.document : prepared.document ?? undefined,
              target.kind === 'document' && target.blockId && prepared.document?.hash === target.document.hash
                ? { blockId: target.blockId, hash: target.document.hash } : undefined)
          } finally {
            const unused = takeOffered()
            if (unused !== undefined) await unwrap(scope.remote.wikiman.closeEditor(unused.view.viewId))
          }
        })
        opening = run
        return run
      },
      getSnapshot: () => snapshot,
      subscribe: (listener) => { subscribers.add(listener); return () => { subscribers.delete(listener) } },
      select,
      open: (target, signal) => {
        if (runtime === 'siyuan') {
          const reference = target.kind === 'document' ? target.document : null
          const pageId = target.kind === 'page' ? target.page.pageId : target.document.pageId
          return unwrap(scope.remote.wikiman.openSiyuan(pageId, location.origin, reference, scope.locale.getLocale().active, signal))
        }
        return target.kind === 'page'
          ? unwrap(scope.remote.wikiman.openPage(target.page.pageId, signal))
          : unwrap(scope.remote.wikiman.openEditor(target.document, signal))
      },
      resolve: (pageId, blockId, signal) => unwrap(scope.remote.wikiman.resolveLink(pageId, blockId, signal)),
      release: viewId => unwrap(scope.remote.wikiman.closeEditor(viewId)),
    }
    const panel: PanelActions = {
      runtime,
      getSnapshot: editor.getSnapshot,
      subscribe: editor.subscribe,
      pages: (query, cursor, signal) => query
        ? unwrap(scope.remote.wikiman.searchPages(query, cursor, signal))
        : unwrap(scope.remote.wikiman.pages(cursor, signal)),
      create: (request, signal) => unwrap(scope.remote.wikiman.createPage(request, signal)),
      maintenancePages: signal => unwrap(scope.remote.wikiman.maintenancePages(signal)),
      change: (request, signal) => unwrap(scope.remote.wikiman.changePage(request, signal)),
      importLibrary: (request, confirmation, signal) => unwrap(scope.remote.wikiman.importLibrary(request, confirmation, signal)),
      syncLibrary: signal => unwrap(scope.remote.wikiman.syncLibrary(signal)),
      select,
      attach: (anchor) => { if (snapshot.anchor !== anchor) update({ ...snapshot, anchor }) },
    }
    const openCitation = (document: WikiDocument) => {
      select({ kind: 'document', document })
      scope.layout.selectPanel(panelId)
    }
    scope.slots.inject('sidebar.panellist', () => scope.slots.register({
      name: 'sidebar.panellist', id: panelId, order: 12, locale: 'wikiman', label: () => t('title'),
    }, Icon))
    scope.slots.inject('main', () => scope.slots.register({
      name: 'main', key: panelId, locale: 'wikiman', inject: () => panel,
    }, WikiPanel))
    scope.slots.inject('shell.overlay', () => scope.slots.register({
      name: 'shell.overlay', id: 'wikiman.editor', locale: 'wikiman', inject: () => editor,
    }, WikiWorkArea))
    scope.slots.inject('tool.call.toolview', () => scope.slots.register({
      name: 'tool.call.toolview', key: 'wiki_read', locale: 'wikiman', inject: () => ({ openCitation }),
    }, ReadRow))
  })
  try {
    await ui
  } catch (error) {
    await ui.dispose()
    await unmount()
    throw error
  }
  return async () => {
    await ui.dispose()
    await unmount()
  }
}
