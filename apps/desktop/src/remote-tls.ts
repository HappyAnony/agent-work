/** Remote desktop HTTPS uses the certificates the device owner trusts in the operating system. */
import { getCACertificates, setDefaultCACertificates } from 'node:tls'

/**
 * Add operating-system trust roots to Node's configured roots before remote requests start.
 * The remote-only desktop main thread owns this setting for its lifetime.
 * Certificate expiry, chain and hostname verification remain enabled.
 */
export function configureRemoteTls(): void {
  setDefaultCACertificates([...new Set([...getCACertificates('default'), ...getCACertificates('system')])])
}
