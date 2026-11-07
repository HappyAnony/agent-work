/** Localized recovery actions for native connection failures. */
const en = {
  certificate: 'The server certificate is not trusted. Install and fully trust its root certificate on this device, then retry.',
  frontend_resource: 'Local app resources are incomplete. Rebuild and reinstall the app.',
  frontend_version: 'The app and server use different frontend versions. Install a matching app build.',
  credential_storage: 'Could not save the connection. Install the app with a valid development signature and retry.',
  server_address: 'Enter a root HTTPS server address without a path or query.',
  authentication: 'Check the access token in the server launch link and reconnect.',
}
const zh: typeof en = {
  certificate: '服务端证书尚未受信任，请在此设备上安装并完全信任根证书后重试',
  frontend_resource: '应用本地资源不完整，请重新构建并安装应用',
  frontend_version: '应用与服务端的前端版本不一致，请安装匹配版本',
  credential_storage: '无法保存连接，请使用有效开发签名安装应用后重试',
  server_address: '请填写不带路径和查询参数的根 HTTPS 服务端地址',
  authentication: '请检查服务端启动链接中的访问令牌后重新连接',
}
/**
 * Describe a known native failure without exposing credential or URL details.
 * @param code - Native failure discriminant.
 * @param locale - Device language.
 * @param fallback - Shared connection recovery text for unknown failures.
 * @returns Localized recovery action.
 */
export function connectionFailure(code: string, locale: string, fallback: string): string {
  const copy = locale.toLowerCase().startsWith('zh') ? zh : en
  return Object.hasOwn(copy, code) ? copy[code as keyof typeof en] : fallback
}
