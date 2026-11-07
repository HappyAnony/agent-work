/** Startup copy for the optional remote desktop client. */
const en = {
  title: 'Connect to AgentWork', address: 'Server address', token: 'Access token',
  connect: 'Connect', connecting: 'Connecting…',
  description: 'The interface loads from this app. Knowledge and Agent tasks run on the selected server.',
  tokenHint: 'Use the token from the server launch link when connecting for the first time.',
  failed: 'Could not connect. Check the server address, access token and client version, then retry.',
  reconnect: 'Connect to server…', quit: 'Quit AgentWork',
}
const zh: typeof en = {
  title: '连接 AgentWork', address: '服务器地址', token: '访问令牌',
  connect: '连接', connecting: '正在连接…',
  description: '界面从应用本地加载，知识库和 Agent 任务在所选服务器运行',
  tokenHint: '首次连接时，填写服务器启动链接中的 token',
  failed: '连接失败，请检查服务器地址、访问令牌和客户端版本后重试',
  reconnect: '连接服务器…', quit: '退出 AgentWork',
}
/**
 * Select remote-client startup text; other languages use the English fallback.
 * @param locale - Electron language or saved shell language.
 * @returns Startup presentation text.
 */
export function remoteCopy(locale: string): typeof en { return locale.toLowerCase().startsWith('zh') ? zh : en }
