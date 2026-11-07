/** Extract the stable WikiMan failure reason from a browser Remote error. */

/**
 * Read the owner's public failure identifier without exposing private error details.
 * @param error - failure reported by a browser Remote.
 * @returns the identifier or an empty string when unavailable.
 */
export function remoteReason(error: unknown): string {
  if (typeof error !== 'object' || error === null || !('details' in error)) return ''
  const details = error.details
  return typeof details === 'object' && details !== null && 'reason' in details && typeof details.reason === 'string'
    ? details.reason
    : ''
}
