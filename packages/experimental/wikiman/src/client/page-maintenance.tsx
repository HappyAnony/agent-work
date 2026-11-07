/** Workbench controls for native page moves, names and recycle-bin entries. */
import { useEffect, useRef, useState } from 'react'
import { Button, Input, Modal } from '@deepseek-ai/dsh-client-ui-primitives'
import type { PropsLocale } from '@deepseek-ai/dsh-client-ui-slots'
import type { WikiPage, WikiPageChange } from '../types.ts'
import { remoteReason } from './remote-reason.ts'
import css from './wiki.module.css'

type Props = PropsLocale<'wikiman'> & {
  readonly page: WikiPage | null
  readonly load: (signal: AbortSignal) => Promise<readonly WikiPage[]>
  readonly change: (request: WikiPageChange, signal: AbortSignal) => Promise<void>
  readonly onChanged: () => void
  readonly onClose: () => void
}

function descendants(root: string, pages: readonly WikiPage[]): Set<string> {
  const excluded = new Set([root])
  const byId = new Map(pages.map(page => [page.pageId, page]))
  for (const page of pages) {
    let parent = page.parentId
    while (parent) {
      if (parent === root) { excluded.add(page.pageId); break }
      parent = byId.get(parent)?.parentId ?? null
    }
  }
  return excluded
}

/**
 * Manage one page or restore a deleted subtree with the native kernel.
 * @param props - selected page or recycle bin, localized controls and commit callbacks.
 * @returns the maintenance dialog.
 */
export function PageMaintenance({ page, load, change, onChanged, onClose, t }: Props) {
  const [pages, setPages] = useState<readonly WikiPage[] | null>(null)
  const [name, setName] = useState(page?.name ?? '')
  const [parentId, setParentId] = useState(page?.parentId ?? '')
  const [rank, setRank] = useState(page?.rank ?? '')
  const [confirm, setConfirm] = useState(false)
  const [failure, setFailure] = useState<'listError' | 'pageChangeError' | 'closeBeforeMaintenance' | 'pageConflict' | null>(null)
  const [busy, setBusy] = useState(false)
  const operation = useRef<AbortController | null>(null)
  useEffect(() => {
    const abort = new AbortController()
    void load(abort.signal).then((value) => {
      if (!abort.signal.aborted) setPages(value)
    }, () => { if (!abort.signal.aborted) setFailure('listError') })
    return () => { abort.abort(); operation.current?.abort() }
  }, [load])

  async function submit(request: WikiPageChange) {
    if (operation.current) return
    const abort = new AbortController()
    operation.current = abort
    setBusy(true)
    setFailure(null)
    try {
      await change(request, abort.signal)
      if (!abort.signal.aborted) onChanged()
    } catch (error) {
      if (!abort.signal.aborted) {
        const reason = remoteReason(error)
        setFailure(reason === 'editor_busy' ? 'closeBeforeMaintenance' : reason === 'source_conflict' ? 'pageConflict' : 'pageChangeError')
      }
    } finally {
      operation.current = null
      if (!abort.signal.aborted) setBusy(false)
    }
  }

  const current = pages?.find(item => item.pageId === page?.pageId && item.deletedAt === null)
  const base = current && { pageId: current.pageId, hash: current.hash }
  const excluded = current && pages ? descendants(current.pageId, pages) : undefined
  const destinations = pages?.filter(item => item.deletedAt === null && !excluded?.has(item.pageId))
  const names = new Map<string, number>()
  for (const item of destinations ?? []) names.set(item.name, (names.get(item.name) ?? 0) + 1)
  const trashed = pages?.filter(item => item.deletedAt !== null)
  return <Modal open title={page ? t('managePage') : t('trash')} closeLabel={t('close')} onClose={() => { if (!busy) onClose() }}>
    {pages === null && !failure && <p role="status">{t('loading')}</p>}
    {failure && <p role="alert">{t(failure)}</p>}
    {current && base && <>
      <div className={css.maintenanceField}>
        <label>{t('pageName')} <Input value={name} maxLength={256} onChange={(event) => { setName(event.target.value) }} /></label>
        <Button size="sm" disabled={busy || !name.trim() || name.trim() === current.name}
          onClick={() => { void submit({ ...base, kind: 'rename', name }) }}>{t('renamePage')}</Button>
      </div>
      <div className={css.maintenanceField}>
        <label>{t('moveDestination')} <select value={parentId} onChange={(event) => { setParentId(event.target.value) }}>
          <option value="">{t('rootPages')}</option>
          {destinations?.map(item => <option key={item.pageId} value={item.pageId}>
            {names.get(item.name) === 1 ? item.name : `${item.name} · ${item.pageId}`}
          </option>)}
        </select></label>
        <label>{t('pageRank')} <Input value={rank} maxLength={128} onChange={(event) => { setRank(event.target.value) }} /></label>
        <Button size="sm" disabled={busy || !rank.trim() || parentId === (current.parentId ?? '') && rank.trim() === current.rank}
          onClick={() => { void submit({ ...base, kind: 'move', parentId: parentId || null, rank }) }}>{t('movePage')}</Button>
      </div>
      {confirm && <p>{t('trashDescendants')}</p>}
      <Button size="sm" disabled={busy} onClick={() => {
        if (confirm) void submit({ ...base, kind: 'trash' })
        else setConfirm(true)
      }}>{confirm ? t('confirmTrash') : t('trashPage')}</Button>
    </>}
    {page && pages && !current && <p role="alert">{t('pageConflict')}</p>}
    {!page && trashed?.length === 0 && <p>{t('emptyTrash')}</p>}
    {!page && trashed?.map((item) => {
      const parentDeleted = trashed.some(parent => parent.pageId === item.parentId)
      return <div className={css.maintenanceField} key={item.pageId}>
        <span>{item.name}</span>
        <Button size="sm" disabled={busy || parentDeleted} title={parentDeleted ? t('restoreParentFirst') : undefined}
          onClick={() => { void submit({ pageId: item.pageId, hash: item.hash, kind: 'restore' }) }}>{t('restorePage')}</Button>
      </div>
    })}
  </Modal>
}
