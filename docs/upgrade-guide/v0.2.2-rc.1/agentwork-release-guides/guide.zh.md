---
kind: upgrade-guide
description: "AgentWork 发布清单增加安装指南摘要，新发布要求包含指南的格式。"
---

# 为 AgentWork 发布附带安装指南

[English](guide.md) | 中文

## 变更

新的 AgentWork 发布目录使用 release.json schemaVersion 2，包含有摘要校验的 README.md 与 README.en.md 文档。发布器要求这一完整格式，并在说明中直接列出目标下载。已有 schemaVersion 1 发布目录仍可校验和部署，但不能通过新版发布器重新发布。单目标产物清单和知识库数据格式保持不变。

## 迁移

1. 使用当前统一脚本重新构建或汇集完整集合，不手工修改旧 release.json 或校验清单。
2. 校验发布目录，确认 documents 清单与 SHA256SUMS 都包含两份安装指南。
3. 从校验通过的集合发布新的产品版本，保留已有公开版本及资产。见[安装教程](../../../cookbook/installing-agentwork-release.zh.md)与[发布流程](../../../cookbook/releasing-agentwork.zh.md)。
