# WikiMan portable runtime

[English](README.md) | 中文

这份源码构建 [DSH WikiMan 插件](../../packages/experimental/wikiman/README.zh.md)使用的改造版 `wiki-kernel` 和 `wiki-app`。app 提供原生文档树、富文本编辑器、文档菜单、本地设置及作用域内的 app 插件。DSH 负责页面导航、标签、分屏、Agent 工具、旧库导入和 portable 双端同步。

## Build

将 `WIKIMAN_PREBUILT_APP_DIR` 设为输出目录之外、已经构建完成的 `wiki-app` 绝对路径，可让不同服务端平台复用相同前端资源。构建仍会为当前机器编译 Go 内核和 CLI。Linux 的 SQLite 需要 C 编译器；通过 `wiki-pdf` 的 PDF 导出仅支持 macOS。

使用 Node 24.9 或更新版本、pnpm，以及 `wiki-kernel/go.mod` 指定的 Go 工具链。在仓库根目录运行 `bash native/wikiman/build.sh`。app 使用独立锁文件和仅用于构建的 package，不启动 Electron 或 Node 应用。产物通过 DSH profile 启动。

产物为 `native/wikiman/out/bin/{wiki-kernel,wiki-cli}` 和 `native/wikiman/out/wiki-app`。macOS 上还会通过 Xcode 命令行工具构建 `bin/wiki-pdf`。设置 `WIKIMAN_OUT_DIR` 可指定其他绝对输出目录；设置 `WIKIMAN_PANDOC_ARCHIVE` 为当前平台可信的 Pandoc ZIP 可启用 Word 导出。HTML 和原生文档导出不依赖 Pandoc。产物不包含原始 `siyuan-kernel` 或 `siyuan-app`。

将插件的 `kernelPath` 和 `appDir` 指向这些产物，将 `sourceRoot` 指向包含 `store.json` 的 portable 知识库。DSH 通过私有编辑器通道传入当前界面语言；字典、主题、字体、表情、图标、Lute、PDF 和富文本编辑器资源均在本地打包。

portable 编辑器通过私有通道接受回环 Web 窗口、精确的 `tauri://localhost` 和 `dsh-app://app` 应用源，以及与编辑器文档同源的远程父页面。远程内容和事件请求经过 DSH 认证；内核仍位于私有回环地址。

## Verify

运行 `bash native/wikiman/test.sh` 验证内核的服务、UI、workspace、源码事务和存储测试。构建 DSH 和此 runtime 后，运行 `DSH_WIKIMAN_RESOURCES="$PWD/native/wikiman/out" DSH_SNAPSHOT=replay pnpm run test:web:built apps/web/tests/wikiman.e2e.ts -t 'persists a browser edit|moves and restores|keeps native tabs'` 验证 portable 集成。原始 SiYuan 兼容性测试需要另一个包含原始 runtime 配对产物的资源目录。

## Local feature policy

portable 入口不提供 WikiMan 账号初始化、登录、会员购买或官方云仓库。内核拒绝账号、云服务和官方同步 API 请求，已移除的云账号启动选项会校验失败。本地富文本编辑功能不要求会员。DSH 模型登录仍由 DSH 负责。portable 双端同步继续通过 DSH 插件提供。

本地 view ticket、回环地址凭据、页面授权及版本校验仍为必需。浏览器连接关闭时会撤销它的编辑器租约；知识库维护仍会等待所有活动编辑器关闭。

## Source maintenance

`source-snapshot.json` 记录导入时的 IronMan 源码版本及基线文件哈希，包含该工作区已有的 WikiMan 改造。`LOCAL_MODIFICATIONS.md` 记录 DSH 专用改动。内核测试使用 `fixtures/` 下复制的合成数据；构建产物和依赖目录已忽略。更新源码时应明确选择文件、保留本地改动和许可证，然后重新构建并执行原生及 DSH 集成检查。

导入的 SiYuan 衍生文件保留原版权及 AGPL-3.0-or-later 声明，见 [LICENSE-AGPL-3.0.txt](LICENSE-AGPL-3.0.txt)。内置资源保留各自的许可证文件。轮询后端声明会复制到 runtime 的 `notices/pollwatch` 目录。本源码子树不适用根目录的 MIT 许可证。
