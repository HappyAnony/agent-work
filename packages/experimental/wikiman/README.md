---
description: "Use the original SiYuan desktop or a portable WikiMan workbench in DSH Web and let the Agent cite saved documents."
kind: "package-reference"
---

# @deepseek-ai/dsh-wikiman

English | [中文](README.zh.md)

## Summary

Open WikiMan from the DSH Web sidebar. The source-owned portable runtime combines the WikiMan page tree with the native document tree, tabs, splits and rich editor, and supports legacy import and verified peer sync. The configured workspace can search saved documents and reopen a cited original from the Session. Original SiYuan mode remains available for compatibility.

## Table of Contents

- [Use this package](#use-this-package)
- [Understand the implementation](#understand-the-implementation)
- [Further Exploration](#further-exploration)
- [Model Experience](#model-experience)
- [Known Limitations and Deferred Work](#known-limitations-and-deferred-work)
- [Dev Note](#dev-note)

-----

<a id="use-this-package"></a>
## Use this package

Build this checkout with Node 24.9 or newer. Close IronMan before making a working copy of a WikiMan library.

Keep the matching runtime resources in a persistent directory, such as `$DSH_HOME/runtimes/wikiman`, and keep library data under a separate directory, such as `$DSH_HOME/data`. The plugin uses the configured executables and application assets at runtime.

Create a new Web profile and link the built plugin from the repository root.

```sh
export DSH_HOME="$HOME/.agentwork-dsh"
pnpm dsh --profile agentwork --from-default-profile web --dump-config
pnpm dsh plugin --profile agentwork add --workspace-root link:./packages/experimental/wikiman
```

### Use the original SiYuan desktop

Use the matching `bin/siyuan-kernel` and `siyuan-app` artifacts from the IronMan version that ships SiYuan, such as the package resources built from commit `d5565e0`. Set `runtime: siyuan` and point `sourceRoot` to the copy containing `page.db` and the page workspaces under `pages/`.

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

Run `pnpm dsh --profile agentwork --patch /path/to/wiki.patch.yml --host 127.0.0.1 --port 0` and open **Knowledge base**. Select a WikiMan page on the left. SiYuan owns the document tree, tabs, editor and saves. The native interface uses the active DSH language when opening each page, including existing workspaces. Reopen the page after changing DSH's language. Languages absent from SiYuan's resources use English. The **Page actions** menu contains page management and **Save and close**. The Agent reads committed documents in the configured workspace. Select **Open Wiki source** from a read result to open the cited document in SiYuan.

### Use the portable workbench

Portable mode supports a Linux or macOS Host with remote Web and mobile browsers, or a Mac client that packages the matching frontend locally. Follow [deploying an AgentWork server](../../../docs/cookbook/deploying-agentwork-server.md) for profile preparation, SSH access and packaging.

At widths up to 800 CSS pixels, use **Pages** to switch between the page list and the full-height editor. Selecting a page closes the list. The editor remains mounted while browsing pages or resizing, preserving pending edits. The Wiki document tree opens separately from its dock button.

Build the modified source pair with `bash native/wikiman/build.sh`; see the [runtime build instructions](../../../native/wikiman/README.md) for prerequisites, export helpers and licenses. Use `native/wikiman/out/bin/wiki-kernel` and `native/wikiman/out/wiki-app`, plus a portable store whose root contains `store.json`. Mount the plugin with the following patch. Replace the four paths with absolute local paths. The workspace path is the directory to select when creating a Chat Session. The log upload setting prevents the official DeepSeek API from receiving the additional canonical Session log. The selected model still receives the excerpts used to answer the question.

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

Start with `pnpm dsh --profile agentwork --patch /path/to/wiki.patch.yml --port 0`. Open **Knowledge base**, select a page in the tree or choose **Page actions → New page**, and edit in the native work area. Native tabs support pinning, close actions, dragging and splits; their layout restores when reopening the page. Choose **Page actions → Save and close** to save every tab and finish editing. Ask the configured workspace about a distinctive term in that paragraph. Expand the turn's tool activity and choose **Open Wiki source** on the read result.

Select **Page actions → Manage page** to rename the selected page, change its parent and sort rank, or send it and its active descendants to the **Recycle bin**. Restoring a parent also restores the descendants recycled with it.

<a id="import-a-legacy-library"></a>
### Import a legacy library

Close legacy WikiMan and open **Knowledge base → Page actions → Library tools → Import legacy library**. Enter the folder containing `page.db`, an existing destination folder outside the legacy library, and a name. Preview the file inventory and native validation warnings. **Create portable copy** publishes a new directory without editing the legacy library. Set `sourceRoot` to the reported path and restart DSH to use that store.

Page IDs, hierarchy, extension metadata, document contents and attachments carry over. A legacy document page needs one identifiable entry document. The report counts legacy deletion records that contain no restorable document data. When retaining historical native citations, add that library’s old store ID to `legacyCitationStoreIds` in the plugin profile; aliases apply only to this configured imported library, and documents are revalidated before opening.

### Sync two portable copies

Start with two copies of the same portable store and point both plugins' `syncDirectory` to the same existing shared filesystem directory, outside both sources. The filesystem must support atomic directory creation and rename across both clients. Open **Library tools → Portable sync → Sync now** on each copy once to establish a common version. Repeat on both sides after editing. A received version becomes active in DSH and persists across restarts.

If both copies change since the common version, DSH preserves both in `<sourceRoot>/.local/dsh-wikiman-sync/conflicts/<digest>` and reports their digests. The shared folder also keeps previously published versions. Compare the copies before deciding how to reconcile the changes. The original source directory remains available when DSH opens a received working copy.

The plugin includes adapted AGPL portable-host files alongside MIT DSH code; its package license records both. The source-owned runtime and bundled resources retain their original licenses.

The [configuration catalog](../../../docs/config-catalog.md#deepseek-aidsh-wikiman) lists operation time, response, scan and source size limits.

-----

<a id="understand-the-implementation"></a>
## Understand the implementation

<details>
<summary>Maintainer details</summary>

In SiYuan mode the Host starts IronMan’s original kernel for the selected page workspace. A one-use loopback handoff sets the native HttpOnly session cookie. A same-origin frame waits for native editor transactions before switching pages. The browser connection holds the kernel lease and releases it when the tab closes. Agent reads consume committed document files. DSH records page recycle status in a separate SQLite table.

In portable mode the Host starts the source-built Wiki kernel on a private loopback port and owns its process and credential. WikiMan account, membership and official cloud services are removed; DSH model authentication remains available. The browser connects the supplied editor assets with a page-scoped view on a private message channel. Changing Wiki pages drains every tab before the Host revokes the old views. The browser connection owns these leases and releases them on disconnect. Switching to Chat parks the editor without reloading its iframe. Browser Remotes serve the local owner. The tool executor checks the Session's original workspace directory on every call.

The Web composition relays portable editor requests and events through authenticated `/api/wikiman` routes. Browser results carry a document-relative `baseUrl`; non-Web consumers retain the private loopback URL. The relay forwards page tickets and session identifiers, never the kernel management credential or the DSH credential. Assets under `/stage`, `/appearance` and `/emojis` remain public application code. `maxEditorBytes` bounds each editor transfer. A packaged remote client serves these assets locally and sends content requests to the selected Host.

The page types are labeled `doc`, `wiki` and `table` in both interface languages. The page tree, creation dialog and recycle bin use document, workspace and spreadsheet glyphs for these types. Page titles remain user-defined.

Wiki pages use `wikiDesktop`; document pages have the stored type `docx` and use `docxSingle`. Every page keeps its content under `pages/<pageId>/data` and its runtime state under `.local/workspaces/<pageId>`. Switching pages or editor modes retains the same kernel process. Selecting a different knowledge-base `sourceRoot` creates another kernel instance. Opening a single document for editing initializes an empty body with one paragraph through a revision-checked native write; search and Agent reads do not initialize documents.

Page mutations use native compare-and-set transactions. Sync checks byte inventories and runs the kernel’s offline content validation before accepting a generation. The current kernel holds its session lock during copying, with all editor views closed. A persisted pointer selects the verified working copy after reception.

Search and reads query the portable kernel directly. A read result carries the store identity, document coordinates and content hash. The Session persists the text seen by the model and the citation metadata. A reopened citation reports a newer revision when its hash differs.

</details>

-----

<a id="further-exploration"></a>
## Further Exploration

- [DSH profiles](../../../apps/cli/reference/README.md) for local plugin installation and overlays.
- [Tool authoring](../../../docs/cookbook/adding-a-tool.md) for model output and Web result cards.
- [Web composition](../../bundle/web-app/README.md) for local Host startup.

-----

<a id="model-experience"></a>
## Model Experience

### Workspace knowledge

#### What the model sees

The configured workspace receives the two tool schemas. `wiki_search` returns native document locations and a continuation. `wiki_read` returns a committed excerpt and its source reference.

#### Token effect

Search hit count and read length bound each result. Tool results remain in Session history until compaction.

#### KV Cache effect

The schemas are stable within the workspace, and tool results append to the existing history.

## Known Limitations and Deferred Work

<a id="known-limitations-and-deferred-work"></a>

- Original SiYuan mode requires the browser and Host on the same machine, a Web origin on `http://127.0.0.1`, and the original WikiMan library format. Remote deployment uses portable mode. SiYuan keeps its per-workspace settings and creates an access code when the workspace has none.
- The workbench opens and creates Wiki spaces and Docx pages. Standalone table pages can be created and managed as metadata.
- Portable sync runs on demand over a shared filesystem. Conflicting versions require manual reconciliation. The Agent tools are read-only.
- Search uses the kernel's literal matching and bounded page scans. The citation opens the document and indicates a version change when the original excerpt is historical.

<a id="dev-note"></a>
### Dev Note

<details>
<summary>Working context for maintainers</summary>

Run the real-kernel Web acceptance test with `DSH_WIKIMAN_RESOURCES=/path/to/wikiman-runtime pnpm run test:web:built apps/web/tests/wikiman.e2e.ts` after building the required assets. For the source-built portable runtime, filter with `-t 'persists a browser edit|moves and restores|keeps native tabs'`. Running the original SiYuan tests also requires `bin/siyuan-kernel` and `siyuan-app/` in the resource directory. The synthetic source fixture lives in this package. The tests exercise original SiYuan tabs and editing, portable sync, and the Agent read and citation path with recorded model output.

</details>
