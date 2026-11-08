---
name: agentwork-release
description: Build, verify, package or release AgentWork with the shared scripts and GitHub Actions; use for new versions, cross-platform artifacts, publication recovery and operator handoff.
disable-model-invocation: false
user-invocable: true
---

# AgentWork 构建与发布

人工与 Agent 调用同一套脚本。先读[操作手册](../../../docs/cookbook/releasing-agentwork.zh.md)、[命令参考](../../../scripts/agentwork/README.zh.md)、[PLAN](../../plans/agentwork/PLAN.md) 和 [TODO](../../plans/agentwork/TODO.md)。后两者是目标和实际进度，不能作为验收证据。

## 确定任务与源码

1. 核对分支、工作区、`agentwork` remote 和目标仓库。保留用户修改，不重置，不向 `origin` 上游推送 AgentWork 改造。
2. 沿用会话已有授权。构建、汇集、草稿、正式发布和部署是不同操作；任务只要求实现流程时，不据此触发正式发布或切换现有服务。
3. 使用 `.node-version`、根 `packageManager` 和原生模块 Go 版本。核对产品清单版本一致，先运行 `pnpm --silent agentwork inspect`。正式构建要求干净源码；开发时显式使用 `--allow-dirty`，这种产物不能发布。

## 执行与解读结果

1. 先 `build --target frontend --out NEW_PATH`，再 `verify`、`pack`。其他平台导入同一归档；原生目标都传 `--frontend`，不各自重建共享资源。
2. 按平台调用 `build`。默认完整集合是 frontend、server-linux-x64、server-darwin-arm64、mac-arm64、ios-simulator。可选架构在原生构建机执行并单独记录证据。iPhone 安装按用户安排；模拟器包不能当作 IPA。
3. 使用 `collect` 汇集或 `release` 指定产物目录，再 `verify --release`。检查全部目标、源码/锁文件/接口/数据版本、内嵌资源、原生架构和签名状态。不要改摘要、去掉失败目标或混用其他 run 让校验变绿。
4. stdout 是一条 JSON；使用 `pnpm --silent` 避免包管理器横幅。子进程日志在 stderr。只以 `ok`、`operation`、`result` 和 `error.code` 处理结果。退出状态：0 成功、2 参数/输入无效、130 取消、1 其他失败。保留完整结果路径供下一轮检查。

## GitHub Actions

- 人工可选 Actions UI，Agent 可用已授权的 `gh` 调用相同工作流。缺 CLI、未认证、权限不足或缺签名配置须明确报告，不从浏览器/系统读取密码、私钥或令牌。
- `agentwork-build.yml` 在 develop 推送或手动触发；记录精确 run ID 和 attempt。用 `gh run watch RUN_ID --repo HappyAnony/agent-work --exit-status` 等待；某个 job 成功不代表整套成功。
- 发布前使用 `agentwork run --repository HappyAnony/agent-work --id RUN_ID --attempt ATTEMPT`。失败、排队、取消、非 develop、PR 来源或错误工作流都不能用于发布；相同源码还必须通过 AgentWork CI。过期资产需要重建；不自动改选其他 attempt。重试构建使用 Re-run all jobs。
- `agentwork-release.yml` 先准备 `draft`，供审阅版本、源码及资产。用户明确要求正式发布后才选 `publish`；发布完成还要核验远程标签与资产。源码 commit 不同或同版本摘要不同必须停止。
- Apple 账号、证书/描述文件授权、钥匙串、设备开发者模式由用户在系统界面处理。未配置 Developer ID/公证不能报告正式 Mac 包已交付。

## 部署与恢复

- 先执行 `deploy --dry-run`。程序、home、library、workspace 和私有配置路径必须分离。现有服务需保存编辑并正常停止；有活动 PID、Wiki 锁、待恢复事务或格式不兼容时，停止依赖操作。
- 真正的 `deploy`/`rollback` 备份私有数据并选择程序，结果仍是 `service: stopped`。按返回的 `start` 数组交给服务所有者启动；验证在线健康、知识库保存、Agent 读取新版与原文引用后，才能报告服务已恢复。
- 私网部署使用受控的 `agentwork-deploy` self-hosted runner，先检查仓库变量。不要给不可信 PR 分配该 runner 或签名/部署秘密；缺私网配置不能猜地址或一直等待排队。
- `publication-unknown` 或 `deployment-incomplete` 代表结果需核查。检查草稿/资产摘要、current 和备份操作记录后再重试，不声称已自动回退。数据回退是独立的备份恢复，不做盲目程序降级。

## 交接

更新 TODO 的执行记录和有证据的完成项：代码/报告路径、实际命令、run/attempt、产物及摘要、真实运行平台、签名状态、待人工事项和下一步。区分单元夹具、模型回放、真实模型、模拟器与真机结果；未执行的发布、服务切换和设备安装保持未完成。不记录凭据或用户知识库内容。
