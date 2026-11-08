---
description: "从下载的 AgentWork 发行包安装服务端、Mac 客户端和 iOS 模拟器，并连接 Web 或手机。"
---

# 安装已下载的 AgentWork 发行包

[English](installing-agentwork-release.md) | 中文

## 概述

先启动一台 Linux 或 Mac 服务端，再连接 Web、Mac 或 iOS。服务端包包含 Node、Wiki 内核、Web 和 Wiki 前端，不要求目标机安装 Node、Go 或 pnpm。Mac/iOS 应用仅提供客户端；同一发布版本的各端共用后端。

## 目录

- [选择与校验下载](#downloads)
- [安装服务端](#server)
- [连接其他设备](#network)
- [安装 Mac 应用](#mac)
- [安装 iOS 应用](#ios)
- [验收与升级](#verification)
- [开发备注](#dev-note)

<a id="downloads"></a>
## 选择与校验下载

Linux x64 服务端需要 x86_64 CPU、GNU libc 2.35 或更新版本；受限 shell 执行还需要内核支持并启用 Landlock。Mac ARM64 服务端及应用需要 Apple Silicon 和 macOS 15 或更新版本。iOS 模拟器包需要 Mac、Xcode 和已启动的兼容模拟器；当前最低 iOS 版本是 18.4。不要将不同版本或架构的程序混用。

下载 README 顶部所列的匹配包。一般用户只需一份服务端包和所需客户端；原生包已包含前端，不必另外部署 frontend 包。GitHub 自动生成的 Source code 是源码，不是可运行程序。将全部自有资产与 SHA256SUMS 放在同一目录后，Mac 使用 `shasum -a 256 -c SHA256SUMS`，Linux 使用 `sha256sum -c SHA256SUMS`。只下载部分资产时，完整清单会提示其余文件缺失；至少核对所选文件的摘要，不把缺文件的检查报告当作整套校验成功。

<a id="server"></a>
## 安装服务端

1. 选择服务端归档并解压到一个新的程序目录，进入该目录；不要覆盖正在运行的旧目录。下列命令假设当前目录包含 artifact.json 和 payload。配置及知识库必须位于程序目录之外。

```sh
PROGRAM="$(pwd)/payload/server"
mkdir -p "$HOME/AgentWork/data"
cd "$HOME/AgentWork/data"
umask 077
```

2. 首次部署可创建空知识库及配置。下列命令拒绝覆盖已有 library 或 server.json；已有库请先备份，跳过初始化，另建配置并将 library 指向原 portable 库。源码模式的旧 home 不直接复用，打包服务端需要自己的 home/profile。

```sh
"$PROGRAM/bin/node" --input-type=module <<'NODE'
import { mkdir, writeFile } from 'node:fs/promises'
import { resolve, join } from 'node:path'
import { randomUUID } from 'node:crypto'
const library = resolve('library')
await mkdir(library, { mode: 0o700 })
await mkdir(join(library, 'pages'), { mode: 0o700 })
await writeFile(join(library, 'store.json'), JSON.stringify({
  schemaVersion: 1, layoutVersion: 1, documentFormat: 'docj',
  storeId: randomUUID(), name: 'AgentWork',
}) + '\n', { flag: 'wx', mode: 0o600 })
await writeFile('server.json', JSON.stringify({
  home: resolve('home'), library, workspace: resolve('workspace'),
  host: '127.0.0.1', port: 18686,
}, null, 2) + '\n', { flag: 'wx', mode: 0o600 })
NODE
```

3. 检查配置并在终端前台启动。保留该终端；正常退出使用 Ctrl+C。服务使用具名 dsh profile，不能同时启动多个实例争用同一 home 或知识库。

```sh
"$PROGRAM/bin/node" "$PROGRAM/start.mjs" --config "$HOME/AgentWork/data/server.json" --check
"$PROGRAM/bin/node" "$PROGRAM/start.mjs" --config "$HOME/AgentWork/data/server.json"
```

4. 打开终端输出的完整 Web 链接（包括首次认证的 token）。不要只输入裸地址；否则会提示需要 Web 认证。Mac 客户端在设置中填写根服务地址，并填写该链接 token 参数的值，成功后会保存会话凭据。令牌和私有日志不要分享或纳入 Git。

需要长期运行时，由服务管理器（Linux 的 systemd 或 macOS 的 launchd）运行相同启动命令，并使用绝对程序/配置路径。工作目录、配置、日志和三个数据目录应归同一服务账号。不要将终端退出或单次 --check 成功当作后台服务已部署。

<a id="network"></a>
## 连接其他设备

默认仅监听服务端本机。远程电脑可先运行 `ssh -N -L 18686:127.0.0.1:18686 用户@服务端`，通过 `http://127.0.0.1:18686` 访问；首次仍需使用服务端打印的 token。此隧道只供建立隧道的电脑使用，手机的 127.0.0.1 指向手机自己。

手机或其他内网/VPN 设备需要可达的 HTTPS 地址。可配置公司 HTTPS 反代，或在 server.json 设置实际监听 IP、tlsCert 和 tlsKey 的绝对路径；同时正确设置 publicUrl 与 trustedHosts。所有设备必须信任该证书，主机名/有效期必须匹配。不要关闭证书校验，不要把服务端 token 嵌入公开链接。示例字段如下，替换为真实地址和证书路径，保留原 home/library/workspace。

```json
{
  "host": "192.168.1.6",
  "port": 18686,
  "publicUrl": "https://agentwork.example.internal:18686",
  "trustedHosts": ["agentwork.example.internal:18686"],
  "tlsCert": "/absolute/certs/fullchain.pem",
  "tlsKey": "/absolute/certs/private-key.pem"
}
```

<a id="mac"></a>
## 安装 Mac 应用

解压 mac-arm64 包，正常退出旧版 AgentWork，再将 payload/mac-arm64/AgentWork.app 复制到“应用程序”。当前预发布采用 ad-hoc 开发签名，未提供 Developer ID/公证。核对下载来源与摘要后，如 macOS 拦截启动，可由用户在系统设置 → 隐私与安全性中检查并选择“仍要打开”；不要全局关闭 Gatekeeper。

应用打开后直接展示工作台。在设置 → 服务连接填写根地址和首次访问令牌；Mac 默认尝试本机服务。Mac 将地址和带有效期的会话凭据保存到 ~/.agent-work/client/，升级旧钥匙串存储时首次需重新输入令牌，之后自动连接。同一系统用户下的其他程序可能读取该文件，凭据文件不要放入共享备份。模型 API Key 在连接后的设置 → 模型中配置。

<a id="ios"></a>
## 安装 iOS 应用

ios-simulator 是模拟器包，不能直接安装到 iPhone。启动 Xcode 的 iOS 模拟器后，解压对应归档，执行以下命令安装；在模拟器桌面打开 AgentWork。

```sh
xcrun simctl install booted /absolute/unpacked/payload/AgentWork-simulator.app
```

使用普通 Apple 账号安装到 iPhone，需要从本版本配套源码在自己的 Mac 上构建并由 Xcode 签名。检出本发布标签，按根 .node-version 与 packageManager 安装 Node/pnpm，执行 pnpm install --frozen-lockfile；通过统一 agentwork build 生成 frontend，使用 --frontend 将同一资源交给 ios-device 目标，选择 --signing development-device 并设置 AGENTWORK_APPLE_TEAM。也可使用 apps/ios 下的 prepare:frontend 与 sync 后，在 Xcode 打开 apps/ios/ios/App/App.xcodeproj，选择 Personal Team 和已连接的 iPhone，点击 Run。Apple 账号授权、开发者模式及开发者 App 信任需由设备所有者在系统界面完成。Personal Team 描述文件通常七天到期，需重新签名安装；不是任何手机都能安装的公开 IPA。

iPhone 首次打开允许本地网络访问，在设置 → 服务连接填写手机可达、证书可信的 HTTPS 根地址及 token。iOS 保存凭据的实现保持原有设备钥匙串方式。通用公开真机分发需要另行配置 Apple Developer Program 与 TestFlight 或 App Store，目前发布集合不承诺这种安装方式。

<a id="verification"></a>
## 验收与升级

打开知识库创建或编辑文档，等待保存完成，换一端打开确认新版。让 Agent 读取这份文档，核对引用能打开原文；重开客户端检查自动连接。无可用模型时先在设置 → 模型配置 API Key。知识库目前需要在线编辑，不提供离线写入或同一块的实时协同。

升级前保存编辑、停止旧服务并备份 home、library、workspace 及私有配置；将新版本解压到新目录，以同一私有配置启动兼容的新程序，同时更新客户端到匹配版本。启动失败时保留日志和数据，不删除库、不强行覆盖文件或让两个服务争用同一库。程序回退不能代替数据恢复；格式不兼容时必须使用已验证备份。

<a id="dev-note"></a>
## 开发备注

本教程随完整发布集合生成可下载的 README.md（中文）与 README.en.md；文件摘要纳入 SHA256SUMS。下载表由经过校验的目标清单生成，不能把模拟器归档标作真机安装包。
