/** The native editor stays mounted while the DSH shell switches between Wiki and Chat. */
import { useCallback, useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore } from 'react'
import { Button } from '@deepseek-ai/dsh-client-ui-primitives'
import type { InjectFace, PropsLocale } from '@deepseek-ai/dsh-client-ui-slots'
import type { WikiDocument, WikiLink, WikiPage, WikiPageEditor, WikiRuntime, WikiSiyuanView } from '../types.ts'
import { mountEditor, type EditorChannel } from './editor-channel.ts'
import { mountSiyuan } from './siyuan-channel.ts'
import css from './wiki.module.css'

/** A top-level page selection, document-tree click or recorded citation. */
export type WikiTarget =
  | { readonly kind: 'page'; readonly page: WikiPage }
  | { readonly kind: 'document'; readonly document: WikiDocument; readonly blockId?: string }

/** Keeps the navigation request and the active panel's editor seat together. */
export interface WorkbenchSnapshot {
  readonly target: WikiTarget | null
  readonly anchor: HTMLDivElement | null
}

/** Browser operations supplied by the workbench owner. */
export interface EditorActions {
  readonly runtime: WikiRuntime
  readonly getSnapshot: () => WorkbenchSnapshot
  readonly subscribe: (listener: () => void) => () => void
  readonly select: (target: WikiTarget | null) => void
  readonly open: (target: WikiTarget, signal: AbortSignal) => Promise<WikiPageEditor | WikiSiyuanView>
  readonly resolve: (pageId: string, blockId: string, signal: AbortSignal) => Promise<WikiLink | null>
  readonly release: (viewId: string) => Promise<void>
}

type Props = InjectFace<EditorActions> & PropsLocale<'wikiman'>
type Position = Pick<DOMRect, 'left' | 'top' | 'width' | 'height'>

/**
 * Park the same native editor while Chat is active and drain it before a Wiki selection changes.
 * @param props - workbench owner and localized controls.
 * @returns the anchored native editor when a page is open.
 */
export function WikiWorkArea(props: Props) {
  const { target, anchor } = useSyncExternalStore(props.subscribe, props.getSnapshot)
  const [active, setActive] = useState(target)
  const [position, setPosition] = useState<Position | null>(null)
  const activate = useCallback((next: WikiTarget | null) => { setActive(next) }, [])

  useEffect(() => {
    if (!active || (active.kind === 'page' && active.page.type === 'table')) setActive(target)
  }, [active, target])

  useLayoutEffect(() => {
    if (!anchor) return
    const measure = () => {
      const { left, top, width, height } = anchor.getBoundingClientRect()
      setPosition(previous => previous?.left === left && previous.top === top
        && previous.width === width && previous.height === height ? previous : { left, top, width, height })
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(anchor)
    window.addEventListener('resize', measure)
    window.addEventListener('scroll', measure, true)
    return () => {
      observer.disconnect()
      window.removeEventListener('resize', measure)
      window.removeEventListener('scroll', measure, true)
    }
  }, [anchor])

  if (!active) return null
  const key = active.kind === 'page' ? active.page.pageId : active.document.docId
  return <div className={css.workArea} hidden={anchor === null || position === null} style={position ?? undefined}>
    {active.kind === 'page' && active.page.type === 'table'
      ? <section className={css.emptyEditor}><h2>{active.page.name}</h2><p>{props.t('tableUnsupported')}</p></section>
      : <NativeEditor key={key} {...props} current={active} next={target} activate={activate} />}
  </div>
}

function NativeEditor({
  current, next, open, release, resolve, select, activate, runtime, t,
}: Props & { current: WikiTarget; next: WikiTarget | null; activate: (target: WikiTarget | null) => void }) {
  const [status, setStatus] = useState<'loading' | 'ready' | 'openFailed' | 'closing' | 'saveFailed'>('loading')
  const [changed, setChanged] = useState(false)
  const [dirty, setDirty] = useState(false)
  const frame = useRef<HTMLIFrameElement>(null)
  const closing = useRef(false)
  const nextTarget = useRef(next)
  nextTarget.current = next
  const active = useRef<{ viewId: string; channel: EditorChannel | undefined; drained: boolean } | null>(null)

  useEffect(() => {
    const abort = new AbortController()
    setStatus('loading')
    setDirty(false)
    void (async () => {
      try {
        const editor = await open(current, abort.signal)
        const viewId = 'url' in editor ? editor.viewId : editor.view.viewId
        if (abort.signal.aborted) {
          await release(viewId)
          return
        }
        const owner: NonNullable<typeof active.current> = { viewId, drained: false, channel: undefined }
        active.current = owner
        if (!frame.current) throw new Error('editor_frame_missing')
        if ('url' in editor) {
          setChanged(editor.changed)
          owner.channel = mountSiyuan(frame.current, editor)
        } else {
          const changed = current.kind === 'document' && editor.document?.hash !== current.document.hash
          setChanged(changed)
          owner.channel = mountEditor(frame.current, editor, setDirty, {
            focus: current.kind === 'document' && current.blockId && !changed
              ? { blockId: current.blockId, hash: current.document.hash }
              : undefined,
            resolve: (blockId, signal) => resolve(editor.view.pageId, blockId, signal),
            open: ({ document, blockId }) => { select({ kind: 'document', document, ...(blockId !== document.docId && { blockId }) }) },
          })
        }
        await owner.channel.ready
        abort.signal.throwIfAborted()
        setStatus('ready')
      } catch (_error) {
        if (!abort.signal.aborted) setStatus('openFailed')
      }
    })()
    return () => {
      abort.abort()
      const owner = active.current
      active.current = null
      owner?.channel?.close()
      if (owner) void release(owner.viewId).catch((_error: unknown) => { /* Host disposal also revokes the kernel generation. */ })
    }
  }, [current, open, release, resolve, select])

  useEffect(() => {
    if (!dirty) return
    const warn = (event: BeforeUnloadEvent) => { event.preventDefault() }
    window.addEventListener('beforeunload', warn)
    return () => { window.removeEventListener('beforeunload', warn) }
  }, [dirty])

  useEffect(() => {
    if (next === current || closing.current || status === 'loading') return
    closing.current = true
    setStatus('closing')
    void (async () => {
      try {
        const owner = active.current
        if (owner) {
          if (status !== 'openFailed' && !owner.drained && owner.channel) {
            await owner.channel.drain()
            owner.drained = true
          }
          await release(owner.viewId)
          owner.channel?.close()
          active.current = null
        }
        activate(nextTarget.current)
      } catch (_error) {
        select(current)
        setStatus('saveFailed')
      } finally {
        closing.current = false
      }
    })()
  }, [activate, current, next, release, select, status])

  const title = current.kind === 'page' ? current.page.name : current.document.title
  const knownTitle = current.kind === 'page' || current.document.docId !== title
  return <section className={css.editor} aria-label={t('editor')}>
    {runtime === 'portable' && <header className={css.editorBar}>
      <h2>{knownTitle ? title : t('editor')}</h2>
      {dirty && <span role="status">{t('unsaved')}</span>}
      <Button size="sm" variant="outline" disabled={status === 'closing' || status === 'loading'} onClick={() => { select(null) }}>
        {status === 'openFailed' ? t('close') : t('saveClose')}
      </Button>
    </header>}
    {status === 'loading' && <p role="status">{t('loading')}</p>}
    {status === 'closing' && <p role="status">{t('saving')}</p>}
    {status === 'openFailed' && <div className={css.editorBar}>
      <p role="alert">{t('openError')}</p>
      <Button size="sm" onClick={() => { select({ ...current }) }}>{t('retryEditor')}</Button>
    </div>}
    {status === 'saveFailed' && <p role="alert">{t('saveError')}</p>}
    {changed && <p role="status">{t('versionChanged')}</p>}
    <iframe ref={frame} title={t('editor')} referrerPolicy="no-referrer" className={css.frame} />
  </section>
}
