---
description: "用于 Linux 和 macOS 独立 AgentWork 服务端发行包的构建期依赖集合。"
---

# AgentWork 服务端依赖集合

[English](README.md) | 中文

## 概述

构建包含 DSH 和 WikiMan 生产依赖的独立 AgentWork 服务端。这个私有 workspace 提供部署依赖图，没有应用 bin，也不启动服务。

## 目录

- [构建与运行](#build-and-run)
- [开发备注](#dev-note)

<a id="build-and-run"></a>
## 构建与运行

使用[统一构建教程](../../docs/cookbook/releasing-agentwork.zh.md)。在目标平台安装原生生产依赖，将它们实体化到运行包中，与共享前端共同校验。打包适配器启动普通的具名 `dsh` profile。用户 home、知识库、workspace 及私有配置保留在程序包外。

[命令参考](../../scripts/agentwork/README.zh.md)管理产物校验与开发职责。普通源码 CLI 发行包不会通过这个 workspace 默认依赖 WikiMan。

<a id="dev-note"></a>
## 开发备注

无。
