---
kind: upgrade-guide
description: "WikiMan Web 编辑器响应使用相对于文档的 API 地址，不再使用私有内核地址。"
---

# 相对于文档解析 WikiMan 编辑器地址

[English](guide.md) | 中文

## 变更

Web 组合中的 `WikiEditorView.baseUrl` 现在为 `/api/wikiman`，不再是私有内核回环 URL。假定该字段为绝对 URL 的自定义 portable 编辑器调用方须相对于当前文档解析该值。非 Web 调用方仍使用私有回环 URL。

## 迁移

1. 使用 `new URL(editor.baseUrl, location.href)` 构造编辑器 API URL。同源请求保留 DSH 认证，并继续发送页面级编辑器 ticket。
2. 从提供页面的 DSH 源加载 portable 应用资源。本地打包的远程客户端在本机提供这些资源，并将内容请求转发到所选服务器。
3. 重新打开文档、保存一个唯一标记，在配置的工作区请 Agent 读取它，再打开读取结果的原文引用。远程客户端操作见 [部署教程](../../../cookbook/deploying-agentwork-server.zh.md)。
