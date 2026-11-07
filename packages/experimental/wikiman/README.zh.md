---
description: "在 DSH Web 中使用原版思源桌面工作台或 WikiMan portable 工作台，让 Agent 引用已保存的文档。"
kind: "package-reference"
---

# @deepseek-ai/dsh-wikiman

[English](README.md) | 中文

## 概述

从 DSH Web 侧栏打开 WikiMan。原版思源模式将 WikiMan 页面树与思源的文档树、多标签和编辑器并排展示。Portable 模式支持旧库导入和双向同步。指定工作区可以搜索已保存的文档，并从会话中的引用打开原文。

## 目录

- [使用此包](#use-this-package)
- [了解实现](#understand-the-implementation)
- [延伸阅读](#further-exploration)
- [模型体验](#model-experience)
- [已知限制和待办工作](#known-limitations-and-deferred-work)
- [开发备注](#dev-note)

-----

<a id="use-this-package"></a>
## 使用此包

用 Node 24.9 或更新版本构建此仓库。关闭 IronMan 后，再复制要使用的 WikiMan 知识库。

将相互匹配的运行资源保存在固定目录，例如 `$DSH_HOME/runtimes/wikiman`，知识库数据另存到 `$DSH_HOME/data` 等目录。插件运行时会使用配置的可执行文件和应用资源。

在仓库根目录创建新的 Web profile，并链接已构建的插件。

```sh
export DSH_HOME="$HOME/.agentwork-dsh"
pnpm dsh --profile agentwork --from-default-profile web --dump-config
pnpm dsh plugin --profile agentwork add --workspace-root link:./packages/experimental/wikiman
```

### 使用原版思源桌面工作台

使用同一 IronMan 版本构建出的 `bin/siyuan-kernel` 和 `siyuan-app`，例如 IronMan 提交 `d5565e0` 的打包产物。设置 `runtime: siyuan`，将 `sourceRoot` 指向包含 `page.db` 和 `pages/` 页面工作区的副本。

```yaml
- id: session-log-deepseek
  disabled: true
- insert:
    - id: wikiman
      name: '@deepseek-ai/dsh-wikiman'
      config:
        runtime: siyuan
        kernelPath: /path/to/package-resources/bin/siyuan-kernel
        appDir: /path/to/package-resources/siyuan-app
        sourceRoot: /path/to/wikiman
        workspaceRoot: /path/to/project
```

运行 `pnpm dsh --profile agentwork --patch /path/to/wiki.patch.yml --host 127.0.0.1 --port 0` 并打开 **知识库**。在左侧选择 WikiMan 页面。思源负责文档树、标签页、编辑和保存。每次打开页面时，原版界面采用 DSH 当前选择的语言，已有工作区也适用。修改 DSH 语言后需重新打开页面，思源资源未包含的语言使用英语。**页面操作** 菜单提供页面管理和 **保存并关闭**。在已配置的工作区中，Agent 可以读取已保存的文档。点击读取结果上的 **打开 Wiki 原文**，会在思源中打开对应文档。

### 使用 portable 工作台

准备相互匹配的 portable `wiki-kernel` 和 `wiki-app`，以及一份根目录包含 `store.json` 的 portable 库副本。用下面的配置挂载插件，将四个路径换成本机绝对路径。创建 Chat 会话时选择配置的工作区目录。关闭日志上传后，官方 DeepSeek API 不再收到额外附带的完整会话日志。用于回答问题的摘录仍会发送给所选模型。

```yaml
- id: session-log-deepseek
  disabled: true
- insert:
    - id: wikiman
      name: '@deepseek-ai/dsh-wikiman'
      config:
        kernelPath: /path/to/package-resources/bin/wiki-kernel
        appDir: /path/to/package-resources/wiki-app
        sourceRoot: /path/to/portable-wiki
        workspaceRoot: /path/to/project
```

运行 `pnpm dsh --profile agentwork --patch /path/to/wiki.patch.yml --port 0`。打开 **知识库**，从页面树选择页面或点击 **新建页面**，然后在原生工作区编辑。编辑完成后点击 **保存并关闭**。在已配置的工作区里询问该段落中的一个具体词语。展开这轮对话的工具过程，在读取结果里点击 **打开 Wiki 原文**。

选择 **管理页面**，可以重命名所选页面、更改父页面和同级顺序，或将它和未删除的子页移入 **回收站**。恢复父页面时，同时恢复与它一起回收的子页。

### 导入旧知识库

关闭旧版 WikiMan，打开 **知识库 → 知识库工具 → 导入旧库**。输入包含 `page.db` 的目录、旧库之外的一个现有目标目录和名称。预览文件清单和内核校验警告，再选择 **创建 portable 副本**。导入会生成新目录，旧库原样保留。将 `sourceRoot` 改为界面显示的新路径，重启 DSH 即可打开新库。

导入保留页面 ID、层级、扩展元数据、文档正文和附件。旧版单文档页需要有一个可识别的入口文档。报告会计入只剩删除标记、已经没有可恢复文档数据的旧库记录。

### 同步两份 portable 知识库

从同一个 portable 知识库复制两份副本，两个插件的 `syncDirectory` 指向同一个现有共享目录，且目录在两个知识库之外。两端访问的文件系统须支持原子创建目录和重命名。先分别执行一次 **知识库工具 → Portable 同步 → 立即同步** 建立共同版本，编辑后再分别同步。接收的版本会在 DSH 中打开，重启后继续使用。

共同版本之后两端都有改动时，DSH 将两份版本保留在 `<sourceRoot>/.local/dsh-wikiman-sync/conflicts/<digest>` 并显示哈希。共享目录也保留之前发布的版本，可以比对后再决定如何处理冲突。DSH 打开接收的工作副本后，原始知识库目录仍可访问。

操作时间、响应字节数、扫描和知识库大小的限制见[配置目录](../../../docs/config-catalog.zh.md#deepseek-aidsh-wikiman)。

-----

<a id="understand-the-implementation"></a>
## 了解实现

<details>
<summary>维护者细节</summary>

在思源模式下，Host 为选中的 WikiMan 页面工作区启动 IronMan 原版内核。浏览器通过一次性的本机登录入口取得思源的 HttpOnly 会话 Cookie。切换页面前，同源 iframe 等待原版编辑器的保存事务。浏览器连接持有内核租约，标签页关闭时释放。Agent 读取已落盘的文档。DSH 使用独立的 SQLite 表记录页面回收状态。

在 portable 模式下，Host 在独立的回环端口启动一个 portable 内核，持有其进程和凭据。浏览器通过私有消息通道将页面级视图交给编辑器。切换 Wiki 页面时，先结清保存队列，再由 Host 撤销旧视图。切到 Chat 时保留编辑器实例，返回后无需重新加载。浏览器 Remote 面向本地所有者。每次执行工具时都会检查 Session 的初始工作区目录。

页面修改由内核比较当前版本后事务提交。同步比对文件内容清单，并在接受版本前通过内核的离线内容校验。复制期间关闭所有编辑器视图，当前内核继续持有会话锁。接收后通过持久化指针选择已校验的工作副本。

搜索和读取直接查询 portable 内核。读取结果带有知识源标识、文档位置和内容哈希。Session 保存模型看到的正文及引用元数据。重新打开引用时，如果哈希有变化，界面会标明当前版本已有更新。

</details>

-----

<a id="further-exploration"></a>
## 延伸阅读

- [DSH profiles](../../../apps/cli/reference/README.zh.md)介绍本地插件安装和配置覆盖。
- [工具开发](../../../docs/cookbook/adding-a-tool.zh.md)介绍模型输出和 Web 结果卡片。
- [Web 组合](../../bundle/web-app/README.zh.md)介绍本地 Host 启动。

-----

<a id="model-experience"></a>
## 模型体验

### 工作区知识

#### 模型看到什么

配置的工作区会获得两个工具定义。`wiki_search` 返回原生文档位置和续查信息。`wiki_read` 返回已保存的摘录及其来源引用。

#### Token 影响

搜索命中数和读取长度限制每个结果的大小。工具结果保留在 Session 历史中，直到压缩处理。

#### KV Cache 影响

工具定义在工作区内保持稳定，工具结果追加到已有历史。

## 已知限制和待办工作

<a id="known-limitations-and-deferred-work"></a>

- 浏览器和 Host 位于同一台机器。思源模式使用 `http://127.0.0.1` 的 Web 地址和原版 WikiMan 库格式。思源保留各页面工作区的设置，未配置访问授权码的工作区会生成授权码。
- 工作台支持打开和创建 Wiki 空间及单文档页。独立表格页支持创建和管理页面元数据。
- Portable 同步通过共享目录手动触发，分叉版本需要人工处理。Agent 工具支持读取。
- 搜索使用内核的字面匹配和有界页面扫描。引用打开对应文档，并在摘录来自历史版本时提示版本变化。

<a id="dev-note"></a>
### 开发备注

<details>
<summary>维护者工作上下文</summary>

构建所需资源后，使用 `DSH_WIKIMAN_RESOURCES=/path/to/wikiman-runtime pnpm run test:web:built apps/web/tests/wikiman.e2e.ts` 运行真实内核的 Web 验收。资源目录的 `bin/`、`siyuan-app/` 和 `wiki-app/` 包含两套运行时，合成知识库样例随本插件保存。测试覆盖原版思源标签页和编辑、portable 同步，以及使用模型回放的 Agent 读取和引用。

</details>
