/** Connection settings copy for the native AgentWork carriers. */
export const serverEn = {
  title: 'Service connection', address: 'Server address', token: 'Access token',
  description: 'Knowledge and Agent tasks use the selected server. Connection settings remain available without a connection.',
  tokenHint: 'Leave blank to reuse the saved credential for this address.',
  connect: 'Save and connect', retry: 'Retry', cancel: 'Cancel', configure: 'Configure service', close: 'Close',
  unconfigured: 'No server configured', connecting: 'Connecting…', connected: 'Connected', unavailable: 'Server unavailable',
  addressError: 'Enter a root server address without a path or query.',
  authentication: 'Authentication failed. Enter a valid access token for this server.',
  certificate: 'The server certificate could not be verified. Check its trust, hostname and expiry.',
  version: 'The client and server versions differ. Install a matching client or update the server.',
  unsaved: 'The current editor has unsaved changes. Restore its connection and save them before changing servers.',
  network: 'The server is unreachable. Check its address, network and whether the service is running.',
  promptTitle: 'Service connection required',
  promptDescription: 'This action needs a server connection. Your current input is retained. Configure the service or retry.',
} as const

/** Simplified Chinese service connection copy. */
export const serverZh = {
  title: '服务连接', address: '服务端地址', token: '访问令牌',
  description: '知识库和 Agent 任务使用所选服务端，未连接时仍可打开设置并修改服务地址',
  tokenHint: '留空可复用该地址已保存的访问凭据',
  connect: '保存并连接', retry: '重试', cancel: '取消', configure: '配置服务', close: '关闭',
  unconfigured: '尚未配置服务端', connecting: '正在连接…', connected: '已连接', unavailable: '服务端不可用',
  addressError: '请输入服务端根地址，不要包含页面路径或查询参数',
  authentication: '认证失败，请填写该服务端的有效访问令牌',
  certificate: '无法验证服务端证书，请检查证书信任、主机名和有效期',
  version: '客户端与服务端版本不匹配，请安装匹配的客户端或更新服务端',
  unsaved: '当前编辑器有未保存内容，请先恢复原服务连接并保存，再切换服务端',
  network: '无法访问服务端，请检查地址、网络以及服务是否已启动',
  promptTitle: '需要连接服务端',
  promptDescription: '此操作需要服务端连接，当前输入会保留，请配置服务或重试',
} satisfies Record<keyof typeof serverEn, string>
