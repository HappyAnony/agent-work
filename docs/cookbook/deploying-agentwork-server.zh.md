---
description: "在 Linux 或 macOS 部署 AgentWork 后端，并连接 Web、移动浏览器和本地加载前端资源的 Mac 客户端。"
---

# 部署 AgentWork 服务端

[English](deploying-agentwork-server.md) | 中文

## 概述

将 Agent、工具、Wiki 内核和数据集中运行在一台 Linux 或 macOS 服务端。Web 和移动浏览器加载服务端前端；可选 Mac 发行包从应用包加载匹配的前端资源。本教程通过私有 SSH 隧道访问，HTTPS 入口需单独部署。

不可覆盖的服务端运行包、配套 Mac/iOS 构建和 GitHub Actions 见[构建与发布 AgentWork](releasing-agentwork.zh.md)。下列依赖工作区的命令仍用于源码开发。

## 目录

- [准备服务端](#prepare-the-server)
- [通过 SSH 访问](#connect-through-ssh)
- [打包 Mac 客户端](#package-the-mac-client)
- [安装 iOS 客户端](#install-the-ios-client)
- [验证使用闭环](#verify-the-workflow)
- [运行和限制](#operation-and-limits)

-----

<a id="prepare-the-server"></a>
## 准备服务端

先准备已构建的 DSH 工作区、Node 24.9 或更新版本、pnpm、原生模块指定的 Go 工具链和 C 编译器，参见 [开发环境](../development.zh.md) 和 [Wiki 运行时构建](../../native/wikiman/README.zh.md)。各服务端平台分别编译原生程序。Web、插件和 Wiki 前端产物须与 Mac 安装包使用的资源一致。

复制现有 portable 知识库到服务端前，先关闭写入该库的程序。保留原库，并为 DSH home、知识库副本和 Agent 工作区分别选择目录。portable 库含 `store.json`；旧 `page.db` 库先通过 [知识库工具](../../packages/experimental/wikiman/README.zh.md#import-a-legacy-library) 导入。不要跨机器复制平台原生 `node_modules` 或 macOS AppleDouble `._*` 文件。

从仓库根目录运行准备工具，将三个数据路径替换为服务端绝对路径。工具创建新 profile、保留已有 profile、挂载 portable WikiMan、关闭完整 Session 日志上传，并选用服务端目录浏览器。模型请求仍会将选定上下文发往配置的模型提供方。

```sh
pnpm exec tsx scripts/prepare-agentwork-server.ts \
  --home /absolute/server-home \
  --store /absolute/portable-library \
  --workspace /absolute/workspace
DSH_HOME=/absolute/server-home pnpm dsh --profile agentwork-server \
  --host 127.0.0.1 --port 18681 --no-open
```

终端会输出包含 owner 访问令牌的启动链接，请妥善保管。服务端重启后须使用新启动链接中的 token；Mac 保存的 cookie 被拒绝时，可在**设置 → 服务连接**中填写当前 token 重新认证。服务监听回环地址，Wiki 内核也保持私有。home 保存模型凭据、Session 和设置；知识库目录保存知识及附件；工作区保存 Agent 文件。三者都需要备份。

<a id="connect-through-ssh"></a>
## 通过 SSH 访问

在 Mac 上将一个回环端口转发到服务端监听端口，将 SSH 目标替换为你的服务端账号。使用 Web 或 Mac 客户端期间保持该连接。

```sh
ssh -N -o ExitOnForwardFailure=yes -o ServerAliveInterval=30 \
  -L 127.0.0.1:18680:127.0.0.1:18681 user@server
```

Web 访问时，将服务端启动链接的主机和端口替换为 `127.0.0.1:18680`，保留 token。Mac 客户端的**设置 → 服务连接**中填写 `http://127.0.0.1:18680` 和该 token。认证后交换为会话 cookie，Mac 使用 Electron safeStorage 加密保存 cookie。切换服务器需要对应服务器的 token。

所有设备连接同一个后端才能共享修改。使用知识库副本的 Linux 部署和 Mac 部署仍是独立的数据存储。在**设置 → 服务连接**中确认当前服务端地址。比较更新时还需选择同一个 Wiki 空间；不同空间可以同名。新增笔记本会自动更新其他已连接客户端的文档树。

通过页面内目录选择器添加服务端工作区。选择准备工具配置的精确 `workspaceRoot`：Wiki 工具仅对该目录下创建的 Session 可用。通过**设置 → 模型**配置模型 API Key；AgentWork 隐藏 DeepSeek 账号登录及自动凭据引导，这些设置作用于所选服务端。

<a id="package-the-mac-client"></a>
## 打包 Mac 客户端

在 macOS 上完成 DSH 和 Wiki 构建后，组装一个新的前端目录。目录包含浏览器包、中文资源及许可证声明，不包含服务端数据或后端程序；已有输出目录会被保留。

```sh
pnpm exec tsx apps/desktop/scripts/prepare-remote-frontend.ts \
  --out "$PWD/.artifacts/remote-frontend"
DSH_DESKTOP_UNSIGNED=1 \
DSH_REMOTE_FRONTEND_DIR="$PWD/.artifacts/remote-frontend" \
pnpm --dir apps/desktop exec electron-builder \
  --config scripts/electron-builder-remote.config.mjs \
  --mac --arm64 --publish never
```

此命令生成个人使用的临时签名构建；Apple 签名和公证发行遵循普通 [打包指南](../../apps/desktop/packaging/README.zh.md)。客户端立即打开本地界面，在启用后端功能前校验服务端 index、插件入口及 Wiki 构建摘要。修改这些资源或新增客户端插件后需重新打包 Mac 客户端；版本不匹配时会在**设置 → 服务连接**中显示错误，并阻止启用后端功能。Mac 优先使用保存的地址；未保存时使用构建配置中的回环默认地址（`http://127.0.0.1:18686`）。连接失败不会自动选择其他服务端。

<a id="install-the-ios-client"></a>
## 安装 iOS 客户端

[iOS 应用](../../apps/ios/README.zh.md) 在本地打包同一套前端，并访问所选 HTTPS 后端。按照其说明使用 Xcode 构建并通过 Personal Team 安装。手机需要可直接访问的局域网或 VPN HTTPS 地址；Mac 上的本地 SSH 隧道不足以支持手机访问。

<a id="verify-the-workflow"></a>
## 验证使用闭环

使用可丢弃的测试文档验收。

1. 打开**知识库**，选择 `doc` 或 `wiki` 中的文档，写入唯一标记。
2. 选择**页面操作 → 保存并关闭**，重新打开并确认标记。
3. 在配置的服务端工作区创建 Session，请 Agent 读取该标记，展开工具活动，确认 `wiki_read` 返回新版文本。
4. 点击**打开 Wiki 原文**，确认原文包含标记。
5. 在移动浏览器或窄屏触摸视口重复验证。关闭 Mac 客户端，确认 Web 仍可访问服务端。

录制的模型输出可在没有提供方密钥时验证真实 Agent 工具执行和持久化，但不能验证在线模型推理或提供方连通性。

<a id="operation-and-limits"></a>
## 运行和限制

该服务供一名可信 owner 从多个设备使用，不提供组织账号、多用户授权、离线编辑或同一块的多人实时协作。版本检查拒绝过期写入；维护知识库前先关闭编辑视图，连接丢失后会撤销其视图。

手机不能直接使用 Mac 的回环 SSH 隧道。手机访问需要在公司内网或 VPN 中配置可信、位于域名根路径的 HTTPS 入口，转发 WebSocket 并保留 DSH 认证。Mac 仅接受远程 HTTPS 地址或回环 HTTP。仅完成 SSH 部署不代表已配置 HTTPS 入口或通过实体手机验收。

使用私有证书机构时，设备所有者须在每台设备上安装并信任其根证书。Mac 客户端启动时读取操作系统信任的根证书，并保留证书链、有效期和主机名校验。修改信任设置后需重启客户端。macOS 将根证书导入“登录”或“系统”钥匙串，启用 SSL 信任并授权保存。iOS 安装证书描述文件后，需启用根证书完全信任。在一台设备上完成信任不会配置其他设备。

远程客户端没有本地 shell、Agent 本地文件系统或本地后端，目录选择均指向服务端。服务端负责生命周期及重启策略，停止服务会中断其任务；每份 portable 库应只由一个内核管理。目前原生 PDF 导出需要 macOS helper，Linux 返回转换器不可用；Word 导出需要可信的对应平台 Pandoc helper。

## 开发备注

无。
