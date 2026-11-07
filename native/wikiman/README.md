# WikiMan portable runtime

English | [中文](README.zh.md)

This source copy builds the modified `wiki-kernel` and `wiki-app` used by the [DSH WikiMan plugin](../../packages/experimental/wikiman/README.md). The app supplies the native document tree, rich editor, document menus, local settings and scoped app plugins. DSH owns page navigation, tabs, splits, Agent tools, legacy import and portable peer sync.

## Build

Set `WIKIMAN_PREBUILT_APP_DIR` to an absolute, already-built `wiki-app` directory outside the output to reuse identical frontend resources across server platforms. The build still compiles the Go kernel and CLI for the current machine. Linux needs a C compiler for SQLite; PDF export through `wiki-pdf` is available only on macOS.

Use Node 24.9 or newer, pnpm and the Go toolchain selected by `wiki-kernel/go.mod`. Run `bash native/wikiman/build.sh` from the repository root. The app has an isolated lockfile and build-only package; it does not launch an Electron or Node application. Start the resulting runtime through a DSH profile.

The output is `native/wikiman/out/bin/{wiki-kernel,wiki-cli}` and `native/wikiman/out/wiki-app`. On macOS, Xcode command line tools also build `bin/wiki-pdf`. Set `WIKIMAN_OUT_DIR` to select another absolute output directory. Set `WIKIMAN_PANDOC_ARCHIVE` to a trusted Pandoc ZIP for the current platform to enable Word export. HTML and native document export do not need Pandoc. The output does not include the original `siyuan-kernel` or `siyuan-app`.

Point the plugin's `kernelPath` and `appDir` at these outputs and `sourceRoot` at a portable store containing `store.json`. DSH supplies the active interface language through the private editor channel; dictionaries, themes, fonts, emoji, icons, Lute, PDF and rich-editor resources are packaged locally.

The portable editor accepts loopback Web windows, the exact `tauri://localhost` and `dsh-app://app` application origins, and a remote parent with the same origin as the editor document. Remote content and event requests pass through DSH admission; the kernel stays on private loopback.

## Verify

Run `bash native/wikiman/test.sh` for the imported kernel's server, UI, workspace, source transaction and store suites. After building DSH and this runtime, run `DSH_WIKIMAN_RESOURCES="$PWD/native/wikiman/out" DSH_SNAPSHOT=replay pnpm run test:web:built apps/web/tests/wikiman.e2e.ts -t 'persists a browser edit|moves and restores|keeps native tabs'` for portable integration. Original SiYuan compatibility tests require a separate resource directory containing that original runtime pair.

## Local feature policy

The portable entry has no WikiMan account bootstrap, login, membership purchase or official cloud repository. Account, cloud and official sync API requests are rejected by the kernel, and the removed cloud-account launch option fails validation. Local rich-editor features have no membership requirement. DSH model login remains owned by DSH. Portable peer sync remains available through the DSH plugin.

Local view tickets, loopback credentials, page authorization and revision checks remain required. Closing a browser connection revokes its editor leases; source maintenance still waits for every active editor to close.

## Source maintenance

`source-snapshot.json` records the imported IronMan source revision and baseline file hashes, including that checkout's WikiMan modifications. `LOCAL_MODIFICATIONS.md` records the DSH-specific changes. Kernel tests use the copied synthetic fixtures under `fixtures/`; build outputs and dependency directories are ignored. Refresh source files deliberately, preserve local changes and licensing, then rebuild and run both native and DSH integration checks.

The imported SiYuan-derived files retain their copyrights and AGPL-3.0-or-later notices; see [LICENSE-AGPL-3.0.txt](LICENSE-AGPL-3.0.txt). Bundled resources retain their own license files. The polling backend notice is copied into the runtime's `notices/pollwatch` directory. This source subtree is not covered by the root MIT license.
