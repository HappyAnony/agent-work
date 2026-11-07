/** Local connection form; tokens are sent directly to the native credential owner. */
import { remoteCopy } from '../../desktop/src/remote-copy.ts'
import { nativeAgentWork } from './native.ts'
import { connectionFailure } from './errors.ts'

const copy = remoteCopy(navigator.language)
document.documentElement.lang = navigator.language
const text: Record<string, string> = { title: copy.title, description: copy.description, addressLabel: copy.address,
  tokenLabel: copy.token, hint: copy.tokenHint, error: copy.failed }
for (const [id, value] of Object.entries(text)) {
  const node = document.getElementById(id)
  if (node !== null) node.textContent = value
}
const form = document.querySelector('form')
const address = document.querySelector<HTMLInputElement>('#address')
const token = document.querySelector<HTMLInputElement>('#token')
const button = document.querySelector('button')
const error = document.getElementById('error')
if (form === null || address === null || token === null || button === null || error === null) throw new Error('connection_form')
function reportFailure(reason: unknown): void {
  const code = reason instanceof Error && /^[a-z_]{1,50}$/u.test(reason.message) ? reason.message : 'native_bridge'
  if (error !== null) { error.textContent = `${connectionFailure(code, navigator.language, copy.failed)} (${code})`; error.hidden = false }
}
button.textContent = copy.connect
error.hidden = !new URLSearchParams(location.search).has('error')
void nativeAgentWork().state().then((state) => { address.value = state.serverUrl }).catch(reportFailure)
form.addEventListener('submit', (event) => {
  event.preventDefault()
  button.disabled = true; button.textContent = copy.connecting; error.hidden = true
  void nativeAgentWork().connect({ serverUrl: address.value.trim(), token: token.value.trim() }).then(() => {
    token.value = ''
    location.replace('/index.html')
  }).catch((reason: unknown) => {
    reportFailure(reason); button.disabled = false; button.textContent = copy.connect
  })
})
