/** Presentation-only connection form; native code validates and stores credentials. */
void (async () => {
  const { copy, serverUrl, locale } = await window.agentworkConnect.state()
  document.documentElement.lang = locale
  document.title = copy.title
  document.querySelector('h1').textContent = copy.title
  for (const [id, key] of [['description', 'description'], ['addressLabel', 'address'], ['tokenLabel', 'token'], ['hint', 'tokenHint']]) {
    document.getElementById(id).textContent = copy[key]
  }
  const address = document.getElementById('address')
  const token = document.getElementById('token')
  const error = document.getElementById('error')
  const button = document.querySelector('button')
  address.value = serverUrl
  button.textContent = copy.connect
  document.querySelector('form').addEventListener('submit', async event => {
    event.preventDefault()
    button.disabled = true
    button.textContent = copy.connecting
    error.hidden = true
    try {
      const result = await window.agentworkConnect.connect(address.value, token.value)
      if (!result.ok) {
        error.textContent = copy.failed
        error.hidden = false
      } else token.value = ''
    } catch (_error) {
      error.textContent = copy.failed
      error.hidden = false
    } finally {
      button.disabled = false
      button.textContent = copy.connect
    }
  })
})()
