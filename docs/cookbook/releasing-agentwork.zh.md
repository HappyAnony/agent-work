---
description: "构建配套的 AgentWork 服务端和原生客户端产物、汇集发布包，并从 develop 使用 GitHub Actions。"
---

# 构建与发布 AgentWork

[English](releasing-agentwork.md) | 中文

## 概述

共享前端只构建一次，各原生目标在对应平台构建，分发前校验完整发布集合。本地人工、[Agent](../../.agents/skills/agentwork-release/SKILL.md) 和 GitHub Actions 调用相同命令。本教程覆盖开发发行包与显式发布；Apple 正式凭据和私网部署 runner 需要单独配置。

## 目录

- [准备固定源码](#prepare)
- [构建共享资源](#frontend)
- [构建原生目标](#native)
- [汇集与发布](#release)
- [使用 GitHub Actions](#actions)
- [部署已停止的服务端](#deploy)
- [故障恢复](#recover)
- [开发备注](#dev-note)

-----

<a id="prepare"></a>
## 准备固定源码

使用 `develop`、[.node-version](../../.node-version) 指定的精确 Node 版本、根 `packageManager` 指定的 pnpm，以及 C/C++ 编译器。Linux 服务端构建还需要 `musl-gcc`（Ubuntu 的 `musl-tools`），用于静态 Landlock 启动器和完整原生包；仅编译 addon 的源码测试构建不够。原生 Go 构建选择 [go.mod](../../native/wikiman/wiki-kernel/go.mod) 指定的版本。Mac 和 iOS 构建需要 macOS 与完整 Xcode；若当前选择 Command Line Tools，将 `DEVELOPER_DIR` 指向 Xcode 的 `Contents/Developer`。CI 选择 Xcode 26.3。Mac 发行包要求 macOS 15 或更新版本，iOS 要求 18.4 或更新版本。

按仓库版本规则选定并提交产品版本。根目录、CLI、Mac、iOS、服务端依赖集合和 WikiMan 的清单版本必须一致。现有[版本更新器](../../scripts/release/bump.ts)管理仓库统一版本序列；AgentWork 发布使用独立的 `agentwork-v<version>` 标签，不发布 npm 包，也不上传 COS。

```sh
pnpm install --frozen-lockfile
pnpm --silent agentwork inspect
```

正式构建要求工作区干净。`--allow-dirty` 只允许显式标记的开发构建，发布汇集会拒绝它。保留已有工作，不要重置工作区。每次构建选择新输出目录。凭据、签名文件和全部用户数据必须放在输出目录之外。

<a id="frontend"></a>
## 构建共享资源

从仓库根目录运行以下命令。它们编译 DSH 和 Wiki 前端，包含语言、字体、插件分块及许可证，并校验每一个产物文件。归档包在 Actions 上传过程中保留可执行权限和指向产物内部的链接。

```sh
pnpm --silent agentwork build --target frontend --out .artifacts/agentwork/frontend
pnpm --silent agentwork verify --input .artifacts/agentwork/frontend
pnpm --silent agentwork pack --input .artifacts/agentwork/frontend --out .artifacts/agentwork/frontend.tar.gz
```

将该归档复制到其他构建机，并检出相同源码提交与锁文件。在新目录导入：`pnpm --silent agentwork unpack --input /absolute/frontend.tar.gz --out /absolute/frontend`。下列原生命令通过 `--frontend` 消费导入的产物，不重新构建其中的前端字节。

<a id="native"></a>
## 构建原生目标

在 Linux x64 构建机使用导入的前端与新输出目录：

```sh
pnpm --silent agentwork build --target server-linux-x64 --frontend /absolute/frontend --out .artifacts/agentwork/server-linux-x64
pnpm --silent agentwork smoke --input .artifacts/agentwork/server-linux-x64 --fixture packages/experimental/wikiman/tests/fixtures/portable
pnpm --silent agentwork pack --input .artifacts/agentwork/server-linux-x64 --out .artifacts/agentwork/server-linux-x64.tar.gz
```

CI 的 Linux 基线是 Ubuntu 22.04。运行包包含 Node 和平台原生依赖；打包后的启动器会检查 GNU libc 不低于构建基线。部署发行版也需要实测。`server-linux-arm64` 应在 ARM64 构建，不要复制其他机器的原生依赖。

在 Mac ARM64 构建机使用同一份共享前端：

```sh
pnpm --silent agentwork build --target server-darwin-arm64 --frontend .artifacts/agentwork/frontend --out .artifacts/agentwork/server-darwin-arm64
pnpm --silent agentwork build --target mac-arm64 --frontend .artifacts/agentwork/frontend --out .artifacts/agentwork/mac-arm64
pnpm --silent agentwork build --target ios-simulator --frontend .artifacts/agentwork/frontend --out .artifacts/agentwork/ios-simulator
```

Mac 目标生成采用 ad-hoc 开发签名的 `payload/mac-arm64/AgentWork.app`。可复制到 Applications 供个人使用，不具有 Developer ID/公证保证。正式 Mac 构建选择 `--signing developer-id`，并按[打包指南](../../apps/desktop/packaging/README.zh.md)配置证书、预期身份/Team ID 和 Apple 公证凭据。验证必须成功；缺凭据不会回退为未签名版本。Intel Mac 在 Intel 构建机上使用 `mac-x64` 和 `server-darwin-x64`。

模拟器产物是 `payload/AgentWork-simulator.app`，不是可安装到 iPhone 的 IPA。真机构建使用 `--target ios-device --signing development-device` 和 `AGENTWORK_APPLE_TEAM`；先在 Xcode 配置账号、证书和描述文件。Personal Team 安装及续期仍由[设备所有者操作](../../apps/ios/README.zh.md#build-and-install)。脚本不会安装手机，也不会导出公开分发的 Personal Team 版本。

逐个使用 `agentwork pack` 归档目标，归档名称使用目标名。将 frontend、Linux x64 服务端、Mac ARM64 服务端、Mac ARM64 客户端、iOS 模拟器这五份归档放入新目录，例如 `.artifacts/agentwork/cohort`。可选架构可随完整集合一同汇集。

<a id="release"></a>
## 汇集与发布

```sh
pnpm --silent agentwork collect --input .artifacts/agentwork/cohort --out .artifacts/agentwork/release
pnpm --silent agentwork verify --release --input .artifacts/agentwork/release
```

汇集会拒绝缺失目标、归档变更、混用源码/锁文件/协议/数据版本，以及内嵌前端与共享产物字节不一致的情况。发布目录包含目标归档、`release.json` 和 `SHA256SUMS`。保留已有输出与版本目录。Mac/iOS 签名状态在发布清单中明确记录。

发布是显式操作。先登录 `gh`，或私下设置 `GH_TOKEN`。从选定的成功构建 attempt 或固定本地工作区取得预期提交，再准备可审阅的草稿：

```sh
pnpm --silent agentwork publish --input .artifacts/agentwork/release --repository HappyAnony/agent-work --mode draft --expected-commit "$(git rev-parse HEAD)"
```

审阅草稿及资产后，在获得发布授权时用同一命令改为 `--mode publish`。稳定版 Mac 发布要求 Developer ID 签名和公证；当前未正式签名的 Mac 集合只能作为预发布版本。重试会复用摘要一致的资产，同版本下内容不同会被拒绝。远程变更期间网络失败会报告 `publication-unknown`：先检查保留的草稿，再决定重试。

<a id="actions"></a>
## 使用 GitHub Actions

打开 [Actions](https://github.com/HappyAnony/agent-work/actions)。`AgentWork CI` 检查 `develop` 推送及向它提交的 PR。`AgentWork build` 在 `develop` 推送（排除仅计划文件变更）或手动触发时运行：一个共享前端任务、托管原生矩阵、完整发布汇集。中间产物和报告保留七天。本仓库已为继承的 DSH 工作流增加隔离条件，不需要上游 runner、COS 或组织秘密。

手动构建选择 **AgentWork build → Run workflow → develop**。等待整次运行成功，并记录 run ID 和 attempt 次数。重试使用 **Re-run all jobs**，确保所有目标消费同一次 attempt 的共享前端。只重跑失败任务不能静默复用前一次的资源产物。

选择 **AgentWork release → Run workflow**，输入成功构建的精确 run ID 和 attempt，先选 `draft`。工作流验证构建源码、事件和 attempt，下载完整发布集合后调用同一发布器。只有授权公开发布时才选择 `publish`。仅该工作流获得 `contents: write`；普通构建保持只读，不获取签名秘密。

使用已登录的 GitHub CLI，Agent 或人工也可触发同一工作流：

```sh
gh workflow run agentwork-build.yml --repo HappyAnony/agent-work --ref develop
gh run list --repo HappyAnony/agent-work --workflow agentwork-build.yml --limit 5
```

已知 run 时，`gh run watch RUN_ID --repo HappyAnony/agent-work --exit-status` 等待其结果。选择发布产物前运行 `pnpm --silent agentwork run --repository HappyAnony/agent-work --id RUN_ID --attempt ATTEMPT`。不能从某个矩阵任务成功推断整次成功，也不能自动选择更早的成功 attempt。排队、取消、过期、无权限和失败结果按[恢复规则](#recover)处理。

<a id="deploy"></a>
## 部署已停止的服务端

将验证后的服务端产物解包到匹配的 Linux 或 Mac 主机。在程序目录外创建私有 JSON 配置，替换为实际绝对路径：

```json
{"home":"/absolute/agentwork-home","library":"/absolute/portable-library","workspace":"/absolute/agent-workspace","host":"127.0.0.1","port":18686}
```

portable 库必须已经存在并含有效 `store.json`，三个数据目录必须不同。现有源码模式 home 先单独备份和迁移；打包服务端拥有自己的 `agentwork-server` profile，拒绝接管未受其管理的 profile。可选的 `tlsCert`、`tlsKey`、`publicUrl` 和 `trustedHosts` 配置现有 DSH 监听器；证书和私钥保留在发布包外。TLS、认证及设备访问见[服务端连接](deploying-agentwork-server.zh.md)。

先运行 `pnpm --silent agentwork deploy --input /absolute/server-artifact --root /absolute/programs --config /absolute/server.json --dry-run`。预演不创建目录、锁或备份。受管进程仍在运行、Wiki 库被占用、待恢复事务或不兼容格式会阻止部署。先保存并关闭编辑器，再正常停止现有服务；脚本不会强行结束用户任务。

去掉 `--dry-run` 后重跑，会私下备份 home/library/workspace、复制已校验程序到不可覆盖的版本目录，并原子切换 `current`。结果明确报告 `service: stopped` 与 `start` 参数数组。通过服务管理器或终端启动该命令，再检查 Web/Mac/iOS 和[知识库验收闭环](deploying-agentwork-server.zh.md#verify-the-workflow)。程序选择与配置检查不能证明在线 HTTP 健康或编辑器验收通过。

程序回退使用之前验证过的服务端产物，运行 `agentwork rollback` 并沿用相同 root/config，先做预演。服务必须停止，数据格式版本必须一致。数据恢复是独立的人工操作。失败的备份没有完成记录，不能当作可恢复备份。旧版本目录及私有备份保留到恢复验证结束。

手动 **AgentWork stopped-service deployment** 工作流使用仓库变量 `AGENTWORK_DEPLOY_RUNNER`（如 `["self-hosted","linux","x64","agentwork-deploy"]`）、`AGENTWORK_DEPLOY_ROOT` 和 `AGENTWORK_DEPLOY_CONFIG`。在目标机或受控网络配置专用可信 runner；工作流不假设托管 runner 能连接私网 IP。缺配置会在托管预检阶段失败，不让私网任务一直排队。默认模式为只读 `preview`；`deploy`/`rollback` 只选择程序，仍保留服务停止状态。它不会在该 runner 执行 PR 代码。

<a id="recover"></a>
## 故障恢复

| 报告 | 安全后续操作 |
| --- | --- |
| 工作区脏、工具版本不符或输出已存在 | 保留工作，修复源码/工具链并选择新输出，不清理无关文件。 |
| 产物缺失、篡改或混用 | 重新下载选定 attempt 或重建整套集合，不修改摘要绕过错误。 |
| 构建排队、失败或取消 | 检查运行状态，等待、修复原因或重跑全部任务，不发布部分成功的矩阵。 |
| 产物过期或 Actions 权限不足 | 重建，或取得正确运行的访问权限，不静默回退。 |
| 发布结果未知 | 检查标签、草稿和资产摘要，再重试同一份验证过的发布集合。 |
| 服务/库忙、遗留 PID 或部署锁 | 保留编辑内容，检查所属进程和操作记录；确认无持有者后才人工移除遗留锁。 |
| 部署中断或重启后服务不健康 | 保持停止，检查 `current` 与备份记录，选择兼容的旧程序，验证健康后再使用。 |
| 数据格式变更 | 采用审阅过的迁移或已验证备份恢复，程序回退不会降级数据。 |

<a id="dev-note"></a>
## 开发备注

[命令参考与开发职责](../../scripts/agentwork/README.zh.md)管理详细字段和扩展规则。[TODO](../../.agents/plans/agentwork/TODO.md)记录实际验收与尚待处理的平台、签名和部署事项；本教程不能证明某次远程发布或真机安装已经执行。
