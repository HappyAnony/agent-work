---
description: "AgentWork 构建与发布命令字段、产物保证、故障处理和后续扩展职责。"
---

# AgentWork 构建与发布参考

[English](README.md) | 中文

## 概述

在工作区使用 `pnpm --silent agentwork <command>`，构建并验证配套的服务端和客户端发行包。命令独占新输出、保留既有版本，在 stdout 返回一条 JSON 结果；子进程日志使用 stderr。[人工教程](../../docs/cookbook/releasing-agentwork.zh.md)管理操作顺序，[Agent 技能](../../.agents/skills/agentwork-release/SKILL.md)管理续接和授权处理。

## 目录

- [命令与报告](#commands)
- [产物保证](#artifacts)
- [开发职责](#responsibilities)
- [验证](#validation)
- [开发备注](#dev-note)

-----

<a id="commands"></a>
## 命令与报告

| 命令 | 输入与效果 |
| --- | --- |
| `help` / `inspect` | 列出操作 / 读取源码、锁文件、工具及持久化版本，不执行构建。 |
| `build` | `--target`、`--out`，可选 `--frontend`、`--release-id`、`--allow-dirty`、`--signing`、`--timeout-ms`；生成新的完整产物。 |
| `verify` | `--input`，可选 `--release`；校验单个产物或完整发布集合，不激活。 |
| `pack` / `unpack` | `--input`、`--out`；传递一个产物，保留权限及产物内部链接。 |
| `release` | 多个 `--artifact`、`--out`，可选逗号分隔的 `--require` 和 `--timeout-ms`；汇集本地归档。公开发布仍要求完整产品集合。 |
| `collect` | `--input` 指向目标 `.tar.gz` 归档目录、`--out`，可选 `--timeout-ms`；导入并汇集完整集合。 |
| `publish` | `--input`、`--repository`、`--expected-commit`，可选 `--mode draft\|publish`（默认 draft）；校验资产并显式修改 GitHub Release。 |
| `run` | `--repository`、`--id`、`--attempt`；验证 develop 上成功的 AgentWork 构建及其精确 attempt。 |
| `select` | `--input` 发布目录、`--target`、`--out`、`--expected-commit`；导入绑定源码的单个目标。 |
| `runner` | `--labels` JSON、`--root`、`--config`；验证受控部署 runner 配置，不创建排队任务。 |
| `smoke` | `--input` 服务端产物、`--fixture` portable 夹具，可选 `--timeout-ms`；不继承模型/GitHub 凭据，启动隔离的具名 profile，认证后比较实际提供的 Web 字节。 |
| `deploy` / `rollback` | `--input` 服务端产物、`--root`、`--config`，可选 `--dry-run`；为已停止的服务选择兼容程序并保留私有备份。 |

`--timeout-ms` 默认为每个构建/归档子进程 30 分钟。取消会终止所属进程树，等待结束后才清理输出。移除新建的不完整构建输出，不移除既有输出。发布时网络/服务端错误或响应不可读，以及部署结果未知时，保留草稿、程序版本或私有操作记录供检查，不声称已经回退。

成功报告为 `{ok:true,schemaVersion:1,operation,result}`；失败为 `{ok:false,schemaVersion:1,operation,error:{code,message}}`。退出 `0` 表示该操作完成，`2` 表示参数/输入无效，`130` 表示取消，`1` 表示其他失败。构建成功不等于已发布；部署成功报告仍明确保留 `service:stopped` 并给出 profile 启动命令。Agent 根据结果字段作决定，不解析诊断措辞。

<a id="artifacts"></a>
## 产物保证

产物根目录仅含 `artifact.json` 和 `payload/`。清单记录产品版本/发布标识、提交、工作区状态、源码指纹、锁文件摘要、工具、目标 OS/CPU、协议/知识库/持久化版本、共享资源身份、签名状态及完整文件清单。文件包含 SHA-256、大小和可执行权限；链接必须指向 payload 内部。缺失、额外、修改或越界的内容均拒绝验证。归档先检查再提取；原生程序按 ELF 或 Mach-O 校验声明的 CPU。

前端包含 Web、客户端插件/分块/资产、Wiki stage/appearance/语言及许可证。原生构建消费该产物，发布验证逐项比较内嵌共享文件。服务端将 Web 及已安装插件字节导入实际运行包位置，Wiki 内核使用导入的 Wiki 资源。Mac 读取内置前端，iOS 将原始 Web index 与原生壳分开保留。不从静态服务端快照复制运行时生成的认证注入。

[packaging/agentwork-server](../../packaging/agentwork-server/README.zh.md) 的服务端依赖集合在目标平台编译 Host 库和 GUI 的 Node 适配代码、安装生产依赖、实体化包链接，并包含 Node 和 Wiki 原生程序。`node-adapters` 构建阶段不生成浏览器包，浏览器资源从共享产物导入。生成后的 CLI 解析清单包含 WikiMan，源码中的 DSH 默认发行包保持独立。私有启动适配器走普通的 `dsh --profile agentwork-server` 路径。配置、知识库、home、workspace 和备份不属于发布资产。

`release.json` 列出必需目标、归档、摘要、产物清单摘要及签名状态；`SHA256SUMS` 也覆盖发布清单。默认必需集合由 [release.ts](release.ts) 的 `requiredTargets` 定义，并与 Actions 矩阵校验。可选 Linux ARM64 与 Intel Mac 需要对应原生构建机；默认托管矩阵不代表这些额外目标已通过验收。 云端汇集逐个校验归档并保留原生构建机生成的归档字节，不保留整套解包副本。提取过程在清理前完成文件写入，解析失败时同样遵循这一顺序。

<a id="responsibilities"></a>
## 开发职责

| 变更 | 归属代码 | 必要发布影响 |
| --- | --- | --- |
| 通用 Agent 工具或外部集成 | `packages/` 下独立能力包 | 测试真实组合；重建服务端，包含客户端代码时也重建共享前端/原生客户端。 |
| 知识库业务、搜索、导入、同步和引用 | [WikiMan 包](../../packages/experimental/wikiman/README.zh.md) | 验证保存 → Agent 读取已提交新版 → 引用打开原文；显式处理源格式迁移。 |
| 原生文档事务和格式 | [wiki-kernel](../../native/wikiman/README.zh.md) | 重建两种服务端平台；更新兼容元数据并审阅数据迁移/回退。 |
| 富文本、块交互、Wiki 资源 | `native/wikiman/wiki-app/` | 重建共享前端与全部消费者；验证语言/资产及内嵌资源比较。 |
| 导航、设置、响应式客户端页面 | 所属 `packages/client/` 插件或 WikiMan 客户端 | 遵循 UI/国际化规则；重建共享资源和原生客户端，验证受影响的真实页面。 |
| 原生网络、钥匙串和系统集成 | `apps/desktop/` 或 `apps/ios/` | 重建对应原生壳；区分模拟器/真机及开发/正式签名证据。 |
| 产物格式、目标适配和发布 | 本目录 | 一同更新 schema/读写方、负例、Actions 矩阵和人工/Agent 文档。 |
| runner/事件/任务编排 | `.github/workflows/agentwork-*.yml` | 调用同一 CLI；保留仓库/源码/attempt 检查和最小权限。 |

编排不进入 Agent loop，Wiki 源码不放进部署脚本，凭据不进入清单。配置由所属组件解析。新增格式字段必须更新所有读写方；不兼容持久化变更遵循仓库迁移和升级指南规则。新增平台还需更新原生构建环境、必需/可选目标策略、原生程序校验、测试和验收矩阵。WikiMan 正式迁包仍在 [TODO](../../.agents/plans/agentwork/TODO.md) 中独立跟踪；脚本通过包名发现它，不把实验目录固化进公共 CLI。

<a id="validation"></a>
## 验证

```sh
pnpm exec vitest run scripts/agentwork --maxWorkers=2 --testTimeout=60000 --hookTimeout=60000
pnpm run build
pnpm run test:docs
```

单元夹具覆盖有效/无效清单、原生程序头、内嵌字节、归档、失败/排队/取消构建选择、可恢复发布、已停止服务检查和数据版本拒绝。它们不能证明云端运行、正式签名发行、手机安装或在线部署健康通过。实际执行、命令和剩余阻塞记录在 TODO 中。

<a id="dev-note"></a>
## 开发备注

无。
