/** Page creation in the same source used by the native editor. */
import { useEffect, useRef, useState } from 'react'
import { Button, Input, Modal } from '@deepseek-ai/dsh-client-ui-primitives'
import type { PropsLocale } from '@deepseek-ai/dsh-client-ui-slots'
import { randomUUID } from '@deepseek-ai/dsh-util-crypto'
import type { WikiCreatePage, WikiPage } from '../types.ts'
import { PageTypeIcon } from './page-type.tsx'
import css from './wiki.module.css'

type Props = PropsLocale<'wikiman'> & {
  readonly parent: WikiPage | undefined
  readonly create: (request: WikiCreatePage, signal: AbortSignal) => Promise<WikiPage>
  readonly onCreated: (page: WikiPage) => void
  readonly onClose: () => void
}

/**
 * Create a native WikiMan page, retaining the operation identity if confirmation fails.
 * @param props - page destination, localized controls and commit callbacks.
 * @returns the creation dialog.
 */
export function NewPageDialog({ parent, create, onCreated, onClose, t }: Props) {
  const [name, setName] = useState('')
  const [type, setType] = useState<WikiCreatePage['type']>('docx')
  const [child, setChild] = useState(false)
  const [error, setError] = useState(false)
  const [busy, setBusy] = useState(false)
  const operation = useRef<AbortController | null>(null)
  const request = useRef<WikiCreatePage | null>(null)
  const trimmed = name.trim()

  useEffect(() => () => { operation.current?.abort() }, [])

  async function submit() {
    if (operation.current || !trimmed) return
    const parentId = child ? parent?.pageId ?? null : null
    const previous = request.current
    const current = previous?.name === trimmed && previous.type === type && previous.parentId === parentId ? previous : {
      name: trimmed, type, parentId, operationId: randomUUID(), createdAt: new Date().toISOString(),
    }
    const abort = new AbortController()
    operation.current = abort
    request.current = current
    setBusy(true)
    setError(false)
    try {
      const page = await create(current, abort.signal)
      if (!abort.signal.aborted) onCreated(page)
    } catch (_error) {
      if (!abort.signal.aborted) setError(true)
    } finally {
      operation.current = null
      if (!abort.signal.aborted) setBusy(false)
    }
  }

  const dismiss = () => { if (!operation.current) onClose() }
  return <Modal open title={t('newPage')} closeLabel={t('close')} onClose={dismiss}
    footer={<Button variant="primary" disabled={busy || !trimmed} onClick={() => { void submit() }}>{t('createPage')}</Button>}>
    <form className={css.createForm} onSubmit={(event) => { event.preventDefault(); void submit() }}>
      <label>
        {t('pageName')}
        <Input value={name} maxLength={256} onChange={(event) => { setName(event.target.value) }} disabled={busy} data-modal-autofocus />
      </label>
      <fieldset disabled={busy}>
        <legend>{t('pageType')}</legend>
        <label><input type="radio" checked={type === 'docx'} onChange={() => { setType('docx') }} /><PageTypeIcon type="docx" />{t('docxPage')}</label>
        <label><input type="radio" checked={type === 'wiki'} onChange={() => { setType('wiki') }} /><PageTypeIcon type="wiki" />{t('wikiPage')}</label>
        <label><input type="radio" checked={type === 'table'} onChange={() => { setType('table') }} /><PageTypeIcon type="table" />{t('tablePage')}</label>
      </fieldset>
      {parent && <label><input type="checkbox" checked={child} disabled={busy} onChange={(event) => { setChild(event.target.checked) }} />
        {t('childPage')}</label>}
      {error && <p role="alert">{t('createError')}</p>}
    </form>
  </Modal>
}
