/** Owner controls for previewing a legacy import and syncing a portable library. */
import { useEffect, useRef, useState } from 'react'
import { Button, Input, Modal } from '@deepseek-ai/dsh-client-ui-primitives'
import type { PropsLocale } from '@deepseek-ai/dsh-client-ui-slots'
import type { WikiImportConfirmation, WikiImportReport, WikiLegacyImport, WikiSyncResult } from '../types.ts'
import type { en } from './locales.ts'
import { remoteReason } from './remote-reason.ts'
import css from './wiki.module.css'

/** Maintenance requests available to the local owner. */
export interface LibraryActions {
  readonly importLibrary: (
    request: WikiLegacyImport, confirmation: WikiImportConfirmation | null, signal: AbortSignal,
  ) => Promise<WikiImportReport>
  readonly syncLibrary: (signal: AbortSignal) => Promise<WikiSyncResult>
}

const messages: Readonly<Record<string, keyof typeof en>> = {
  editor_busy: 'closeBeforeMaintenance',
  source_busy: 'wikiSourceBusy',
  source_in_use: 'wikiSourceInUse',
  source_recovery_required: 'wikiRecoveryRequired',
  source_limit_exceeded: 'wikiSourceLimit',
  source_changed: 'wikiSourceChanged',
  source_operation_timeout: 'wikiSourceTimeout',
  source_validation_failed: 'wikiSourceInvalid',
  replica_invalid: 'wikiSourceInvalid',
  sync_not_configured: 'wikiSyncNotConfigured',
  sync_remote_missing: 'wikiRemoteMissing',
  sync_peer_busy: 'wikiSyncPeerBusy',
  legacy_database_in_use: 'wikiLegacyInUse',
  legacy_database_missing: 'wikiLegacyMissing',
  legacy_orphaned_pages: 'wikiLegacyOrphans',
  legacy_docx_entry_ambiguous: 'wikiLegacyDocx',
  import_destination_exists: 'wikiImportExists',
  import_preview_changed: 'wikiImportChanged',
  import_destination_overlap: 'wikiImportOverlap',
  absolute_paths_required: 'wikiImportAbsolute',
}

type Props = PropsLocale<'wikiman'> & LibraryActions & { readonly onClose: () => void; readonly onSync: () => void }

/**
 * Show import and sync controls, validation summaries and preserved conflicts.
 * @param props - authorized maintenance operations and local labels.
 * @returns the library tools dialog.
 */
export function LibraryTools({ importLibrary, syncLibrary, onClose, onSync, t }: Props) {
  const [request, setRequest] = useState<WikiLegacyImport>({ source: '', destination: '', name: '' })
  const [report, setReport] = useState<WikiImportReport | null>(null)
  const [sync, setSync] = useState<WikiSyncResult | null>(null)
  const [mode, setMode] = useState<'sync' | 'import'>('sync')
  const [busy, setBusy] = useState(false)
  const [failure, setFailure] = useState<keyof typeof en | null>(null)
  const operation = useRef<AbortController | null>(null)
  useEffect(() => () => { operation.current?.abort() }, [])

  function update(field: keyof WikiLegacyImport, value: string) {
    setRequest(previous => ({ ...previous, [field]: value }))
    setReport(null)
    setFailure(null)
  }

  async function run(action: (signal: AbortSignal) => Promise<void>) {
    if (operation.current) return
    const abort = new AbortController()
    operation.current = abort
    setBusy(true)
    setFailure(null)
    try {
      await action(abort.signal)
    } catch (error) {
      if (!abort.signal.aborted) {
        const reason = remoteReason(error)
        setFailure(Object.hasOwn(messages, reason) ? messages[reason] ?? 'wikiMaintenanceFailed' : 'wikiMaintenanceFailed')
        if (reason === 'import_preview_changed') setReport(null)
      }
    } finally {
      operation.current = null
      if (!abort.signal.aborted) setBusy(false)
    }
  }

  const input = { source: request.source.trim(), destination: request.destination.trim(), name: request.name.trim() }
  const incomplete = !input.source || !input.destination || !input.name
  const checked = report && { storeId: report.storeId, digest: report.digest }
  const syncMessageKey = sync?.kind === 'received' ? 'wikiSyncReceived' : sync?.kind === 'published' ? 'wikiSyncPublished' : 'wikiSyncUnchanged'
  return <Modal open title={t('wikiLibraryTools')} closeLabel={t('close')} onClose={() => { if (!busy) onClose() }}>
    <div className={css.headingActions}>
      <Button size="sm" disabled={busy} aria-pressed={mode === 'sync'} onClick={() => { setMode('sync'); setFailure(null) }}>{t('wikiSync')}</Button>
      <Button size="sm" disabled={busy} aria-pressed={mode === 'import'} onClick={() => { setMode('import'); setFailure(null) }}>{t('wikiImport')}</Button>
    </div>
    {busy && <p role="status">{t('loading')}</p>}
    {failure && <p role="alert">{t(failure)}</p>}
    {mode === 'sync' && <>
      <p>{t('wikiSyncHelp')}</p>
      <Button size="sm" variant="primary" disabled={busy} onClick={() => {
        setSync(null)
        void run(async (signal) => {
          const result = await syncLibrary(signal)
          if (signal.aborted) return
          setSync(result)
          if (result.kind === 'received') onSync()
        })
      }}>{t('wikiSyncNow')}</Button>
      {sync?.kind === 'conflict' ? <div role="alert">
        <p>{t('wikiSyncConflict')}</p>
        <dl className={css.libraryResult}>
          <dt>{t('wikiLocalVersion')}</dt><dd><code>{sync.localDigest}</code></dd>
          <dt>{t('wikiSharedVersion')}</dt><dd><code>{sync.remoteDigest}</code></dd>
        </dl>
      </div> : sync && <div role="status">
        <p>{t(syncMessageKey)}</p>
        <dl className={css.libraryResult}><dt>{t('wikiWarnings')}</dt><dd>{sync.warnings}</dd></dl>
      </div>}
    </>}
    {mode === 'import' && <>
      <p>{t('wikiImportHelp')}</p>
      <div className={css.libraryFields}>
        <label>{t('wikiLegacyPath')} <Input value={request.source} disabled={busy} onChange={(event) => { update('source', event.target.value) }} /></label>
        <label>{t('wikiOutputPath')} <Input value={request.destination} disabled={busy} onChange={(event) => { update('destination', event.target.value) }} /></label>
        <label>{t('wikiLibraryName')} <Input value={request.name} maxLength={256} disabled={busy} onChange={(event) => { update('name', event.target.value) }} /></label>
      </div>
      <div className={css.headingActions}>
        <Button size="sm" disabled={busy || incomplete} onClick={() => {
          setReport(null)
          void run(async (signal) => {
            const next = await importLibrary(input, null, signal)
            if (!signal.aborted) setReport(next)
          })
        }}>{t('wikiPreviewImport')}</Button>
        <Button size="sm" variant="primary" disabled={busy || !checked || !!report.destination} onClick={() => {
          if (!checked) return
          void run(async (signal) => {
            const next = await importLibrary(input, checked, signal)
            if (!signal.aborted) setReport(next)
          })
        }}>{t('wikiConfirmImport')}</Button>
      </div>
      {report && <div role="status">
        <dl className={css.libraryResult}>
          <dt>{t('pages')}</dt><dd>{report.pages}</dd>
          <dt>{t('wikiDocuments')}</dt><dd>{report.documents}</dd>
          <dt>{t('wikiLegacyDeleted')}</dt><dd>{report.deletedLegacyPages}</dd>
          <dt>{t('wikiWarnings')}</dt><dd>{report.warnings}</dd>
          {report.destination && <><dt>{t('wikiImportedPath')}</dt><dd><code>{report.destination}</code></dd></>}
        </dl>
        {report.destination && <p>{t('wikiActivateImport')}</p>}
      </div>}
    </>}
  </Modal>
}
