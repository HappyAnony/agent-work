---
description: "AgentWork iOS 在本地打包 DSH 和 WikiMan 界面，并连接同一个 Linux 或 macOS 后端。"
---

# AgentWork iOS

[English](README.md) | 中文

## 概述

Capacitor 应用在 WKWebView 的 `dsh-app://app` 地址中加载本地打包的 DSH、插件和 WikiMan 资源，复用 Mac 编辑器的来源地址校验。所选服务端负责 Agent、工具、知识库和已保存文档。本文说明原生客户端；后端配置见[服务端部署](../../docs/cookbook/deploying-agentwork-server.zh.md)。

## 目录

- [构建和安装](#build-and-install)
- [连接服务端](#connect)
- [运行和限制](#runtime-and-limits)
- [开发检查](#developer-checks)

<a id="build-and-install"></a>
## 构建和安装

需要安装完整 Xcode 26 或更新版本的 Mac、Node 24.9 或更新版本以及 pnpm。应用要求 iOS 18.4 或更新版本。Xcode 必须支持所连接的设备；安装前先验证配对和平台支持。

将完整 Xcode 设为当前开发目录，或在运行 Apple 构建工具前，将 `DEVELOPER_DIR` 设为其 `Contents/Developer` 目录。

在仓库根目录构建与服务端匹配的资源，并准备原生工程：

```sh
pnpm install
bash native/wikiman/build.sh
pnpm run build:artifacts
pnpm --filter @deepseek-ai/dsh-ios prepare:frontend
pnpm --filter @deepseek-ai/dsh-ios sync
```

在 Xcode 中打开 `apps/ios/ios/App/App.xcodeproj`，选择 App target，在 Signing & Capabilities 中选择自己的 Personal Team，连接并信任 iPhone，将其设为运行设备后启动。Apple 账号输入、协议、证书授权和设备开发者模式需要设备所有者亲自处理。Personal Team 描述文件七天后过期；重新构建安装可续期。签名后的应用与设备绑定；未签名 IPA 无法通过在手机上打开来安装。

<a id="connect"></a>
## 连接服务端

填写可信的根 HTTPS 服务端地址，以及其启动链接中的访问令牌。原生进程将令牌交换为服务端 Cookie，校验打包前端指纹，并将凭据保存在设备钥匙串中。以后连接同一地址时可留空令牌。界面无法取得 Cookie，也不带本地后端。

手机必须通过局域网或 VPN 访问服务端。私有证书机构需要在手机上安装描述文件并开启完全根证书信任。证书链、有效期和主机名校验保持开启。客户端和服务端的前端版本必须一致；Web、插件或 Wiki 资源变更后需要重新打包。连接失败时保留连接表单。

<a id="runtime-and-limits"></a>
## 运行和限制

HTTP 正文和 WebSocket 消息经过原生认证连接，静态前端资源从本地加载。返回前台时检查现有连接；共用客户端负责重连。页面导航会关闭上一文档的请求。安全区域和键盘避让由原生窗口负责。

iOS 进入后台时可能暂停界面。Agent 任务继续在服务端运行，知识库编辑需要连接正常且保存成功。应用不提供离线编辑、后台同步和同一块的并发协同编辑。使用现有[验收流程](../../docs/cookbook/deploying-agentwork-server.zh.md#verify-the-workflow)，在已安装应用中验证保存、Agent 读取和原文跳转。

<a id="developer-checks"></a>
## 开发检查

JavaScript 连接层测试直接从源码构建；Swift 包在 macOS 上验证原生地址和资源校验：

```sh
pnpm exec vitest run apps/ios/tests/carrier.spec.ts
swift test --package-path apps/ios
```

`sync` 保留带小数的 iOS 最低版本，并校验复制到 Capacitor 预留 `plugins` 目录之外的模块。个人签名配置可放在忽略的 `ios/App/Signing.local.xcconfig` 中，写为 `DEVELOPMENT_TEAM = <team id>`。本地前端产物、凭据、描述文件和构建产物不纳入版本管理。应用复用现有 DSH 图标；Wiki 资源保留随包 AGPL 声明，源码位于 [native/wikiman](../../native/wikiman/README.zh.md)。

## 开发说明

无。
