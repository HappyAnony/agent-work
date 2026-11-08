/** Local service configuration and one actionable prompt, independent of Host settings. */
import { useEffect, useId, useState } from 'react'
import { Button, Modal } from '@deepseek-ai/dsh-client-ui-primitives'
import type { HostObservable, InjectFace, PropsLocale } from '@deepseek-ai/dsh-client-ui-slots'
import type { ConnectionState } from '@deepseek-ai/dsh-client-connection/client'
import type { PackagedServerSnapshot } from '@deepseek-ai/dsh-client-connection/packaged-server'
import type { serverEn } from './server-locales.ts'
import css from './ServerConnection.module.css'

declare module '@deepseek-ai/dsh-client-ui-slots' {
  interface LocaleNamespaceMap {
    /** Device-local service connection settings. */
    'settings.server': keyof typeof serverEn
  }
}

/** Local settings callbacks and their observable sources. */
export interface ServerConnectionInjected {
  configure(address: string, token: string): Promise<boolean>
  retry(): Promise<void>
  dismiss(): void
  openSettings(): void
  hooks: { serverState: HostObservable<PackagedServerSnapshot>; connectionState: HostObservable<ConnectionState | undefined> }
}
type Props = PropsLocale<'settings.server'> & InjectFace<ServerConnectionInjected>

/** @param props - Native connection owner and translated controls. @returns Service address, authentication and status controls. */
export function ServerConnectionSection({ configure, retry, useServerState, useConnectionState, t }: Props) {
  const state = useServerState(value => value)
  const connection = useConnectionState(value => value)
  const [address, setAddress] = useState(state.address)
  const [token, setToken] = useState('')
  const [changed, setChanged] = useState(false)
  const addressId = useId(), tokenId = useId()
  useEffect(() => { if (!changed) setAddress(state.address) }, [changed, state.address])
  const phase = state.phase === 'connected' && connection !== 'connected'
    ? connection === 'disconnected' ? 'unavailable' : 'connecting' : state.phase
  const failed = phase === 'unconfigured' || phase === 'unavailable'
  return <section className={css.section}>
    <h2>{t('title')}</h2>
    <p>{t('description')}</p>
    <p role="status" className={failed ? css.failure : css.status}>{t(phase)}</p>
    <form className={css.form} onSubmit={(event) => {
      event.preventDefault()
      void configure(address, token).then((connected) => { if (connected) { setToken(''); setChanged(false) } })
    }}>
      <label htmlFor={addressId}>{t('address')}</label>
      <input id={addressId} type="url" value={address} autoCapitalize="none" autoCorrect="off" spellCheck={false}
        disabled={state.phase === 'connecting'} onChange={(event) => { setChanged(true); setAddress(event.target.value) }} />
      <label htmlFor={tokenId}>{t('token')}</label>
      <input id={tokenId} type="password" value={token} autoComplete="off" disabled={state.phase === 'connecting'}
        onChange={(event) => { setToken(event.target.value) }} />
      <p>{t('tokenHint')}</p>
      {state.failure && <p role="alert" className={css.failure}>{t(state.failure === 'address' ? 'addressError' : state.failure)}</p>}
      <div className={css.actions}>
        <Button type="submit" disabled={state.phase === 'connecting' || !address.trim()}>{t('connect')}</Button>
        <Button type="button" variant="outline" disabled={state.phase === 'connecting' || !state.address}
          onClick={() => { void retry() }}>{t('retry')}</Button>
      </div>
    </form>
  </section>
}

/** @param props - Connection prompt state and settings navigation. @returns One modal for a backend-dependent action. */
export function ServerConnectionPrompt({ dismiss, retry, useServerState, openSettings, t }: Props) {
  const state = useServerState(value => value)
  return <Modal open={state.configurationRequested} onClose={dismiss}
    title={t('promptTitle')} description={t('promptDescription')} closeLabel={t('close')}
    footer={<>
      <Button variant="outline" onClick={dismiss}>{t('cancel')}</Button>
      <Button variant="outline" disabled={!state.address || state.phase === 'connecting'} onClick={() => { void retry() }}>{t('retry')}</Button>
      <Button onClick={() => { dismiss(); openSettings() }}>{t('configure')}</Button>
    </>} />
}
