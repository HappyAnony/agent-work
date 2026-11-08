---
kind: upgrade-guide
description: "AgentWork 原生客户端需要打包本地启动信息，并在设置中配置服务连接。"
---

# 使用本地启动信息重新构建 AgentWork 客户端

[English](guide.md) | 中文

## 变更

AgentWork Mac 和 iOS 客户端在服务端认证前打开本地界面。服务端地址和访问令牌从启动表单移至**设置 → 服务连接**。DeepSeek 登录和自动凭据引导已隐藏，模型 API Key 仍在**设置 → 模型**中配置。Mac 凭据迁移须按[文件存储说明重新认证](../agentwork-client-credentials/guide.zh.md)，iOS 保留原有已保存凭据。

准备好的前端目录须在 `manifest.json` 旁包含 `bootstrap.json`。新承载层无法使用旧打包工具生成的目录。普通 DSH Desktop 发行包及 Web 认证保持不变。

## 迁移

1. 按[服务端部署](../../../cookbook/deploying-agentwork-server.zh.md#package-the-mac-client)构建匹配的 DSH 和 Wiki 资源。
2. 使用 `apps/desktop/scripts/prepare-remote-frontend.ts` 准备新的前端目录并重新构建 Mac 客户端。iOS 运行 `pnpm --filter @deepseek-ai/dsh-ios prepare:frontend` 和 `pnpm --filter @deepseek-ai/dsh-ios sync` 后在 Xcode 中重新构建。原生客户端和服务端资源须来自同一次构建。
3. 停止服务端后打开应用，确认主界面和设置仍可访问，服务连接显示故障。启动所选服务端并重试，确认知识库编辑器可打开；若认证被拒绝，在设置中填写当前服务端启动链接中的令牌。
