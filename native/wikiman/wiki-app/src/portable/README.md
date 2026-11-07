# Scoped wiki-app sessions — S07 in progress

`session.ts` is a protocol/session library, **not a replacement editor UI**.
The second batch adds native BlockDOM transactions, actual Protyle hooks, a
dedicated factory and verified iframe-channel plumbing. Existing desktop/single
boots still use legacy `/api/*` and are not silently routed to the portable
Server. The third batch adds controlled boot/static serving and real-browser
editing coverage. The fourth batch connects production Rust/React mounting and
release controls; full UI/resource/tool acceptance is still pending. editorReady
now means controlled assets are available, not complete legacy feature parity.
The fifth batch adds scoped uploads, attachment downloads and a native `.docj`
export toolbar. Rich exports and complete tool/platform acceptance remain open.
The sixth batch adds an offline HTML export through the shared core, with page
assets captured in the same read admission. Word DOCX conversion is now wired;
macOS PDF export now uses a small system WebKit helper, not a browser bundle.
The seventh batch audits legacy dynamic render entry points and exercises
heading/table/list/code editing with actual keyboard input and undo/redo.
The eighth batch covers last-cell Tab row insertion and list Enter splitting,
including inverse/forward structural history and loss-prevention guards.
The ninth batch introduced exact native undo for opaque whole-block deletion.
R04 replaces the browser snapshots with server-owned journal handles.
The tenth batch adds Word DOCX reading-copy export using the existing trusted
Pandoc archive/executable, shared by CLI and Server. Native sources stay .docj.
The eleventh batch adds macOS PDF via the shared core and independent wiki-pdf
helper, including native pagination, offline policy and visible-image validation.
The twelfth batch extends native history to enclosing-block updates that remove
identified opaque descendants, without relaxing anonymous-node identity guards.
The thirteenth batch closes upload/history overlap and prevents retired view
dirty reports or their late failures from changing a replacement host view.
T01 adds a controlled gutter menu rather than enabling legacy API transport.
T02 connects local references and host-resolved document/page references,
including child documents, through drained owners and existing page activation.
T03 is now auditing the editing-tool matrix; full S07 acceptance is still open.

## Original UI providers (R06)

The controlled frame installs one private provider for original component
calls to the scoped fetch helpers. The server resolves an explicit allowlist
through the shared query, transaction, lifecycle and configuration services.
Each successful UI response must match the store, page, generation and view.
Unmapped calls are reported as component errors. They cannot fall through to
legacy networking. Browser callbacks are retired when the frame detaches.

The provider boots with a projection of the owning page's configuration and
bundled language and theme resources. The original Title can rename its mounted
document through the save queue. The legacy archive picker uses the same
queue and shared archive importer with an owned destination. Both operations
reload the committed document after completion.

Desktop and Single use the original Template picker. Single lists only
templates owned by its page and renders against the ticket's entry document.
Its SQL functions use a temporary projection built from that document.
Preview does not publish source. Insertion goes through the active editor's
save queue. Template AV cloning and page-wide reads remain restricted in Single.
Single block-reference candidates use the same entry-only projection and do
not offer new-document creation. The typed block ID only excludes the current
block from results, including when its save is still queued.
The slash menu uses the same scoped reference search, upload queue and
local renderers as direct editing. Uploads from a stale menu or a retired
view are rejected before changing the editor.

The complete desktop and single-view layouts remain in R07. R06 still tracks
original menu coverage, host preferences, resource URLs and event consumers
whose dependencies need verification.

## Host and browser boundary

The trusted Rust host uses the versioned bridge to open a view:

```json
{"sessionId":"CURRENT_SESSION","route":"editor","payload":{"protocolVersion":1,"storeId":"STORE_ID","action":"open","pageId":"PAGE_ID","mode":"docxSingle"}}
```

EditorView contains storeId/pageId/sessionId/viewId/mode, a private random ticket and
optional entryDocument. `wikiSharedCommands.openEditor/closeEditor` are typed
wrappers. Root Bearer stays in Rust; browsers receive single-page capabilities,
never source paths/root administrative credentials. Do not log/persist tickets
or put them in URL/query/fragment. `bridge.ts` verifies the exact parent origin,
window and fresh challenge before accepting one transferred MessagePort. All
replies bind channel/store/page/session; flush/dispose acknowledges only an
actually drained clean binding. The production PortableEditor owner mounts this
iframe and closes acknowledged clean views before source release. Native leases
make every open view block release even if an old page-level dirty report says
clean. React unmount hides a stable frame owner; it does not move/reload the
iframe or destroy unconfirmed input. Release/resume state survives UI remount.
Dirty-report queues remain lease-owned: normal disposal drains reports already
accepted before native close; an explicitly stopped/discarded owner drops queued
reports, ignores new callbacks and cannot paint a late failure on another view.
Reporting failure on the still-active owner remains visible, not silently clean.

## Controlled menus (T01)

The dedicated factory installs a lease-owned capture boundary before legacy
gutter/context handlers run. Ordinary content contextmenu is left uncancelled
for the browser's native menu, while the legacy context handler is bypassed.
Gutter click/right-click offers copy block ID, delete current top-level block,
native Undo and Redo. Nested/multi-block deletion, gutter folds/dragging,
properties and conversions remain explicitly unavailable. Copy failures are
reported without failing the save queue; clipboard availability still depends
on browser/WebKit permissions. Automated clipboard tests intercept the API and
do not certify OS clipboard integration.

Menu deletion commits through the existing nativeHistory/CAS/journal queue,
blocks upload/export/release during the command, then renders committed source.
Deleting the last top-level block inserts a new empty native paragraph in the
same transaction so editing remains possible. It deletes the selected block
only, not the subsequent heading range. Undo/Redo restore exact native bytes.
No optimistic DOM removal, inverse HTML history or document-file deletion is
used. A frozen/detached owner cannot execute a stale menu; detach removes the
menu and all listeners. Floating formatting is limited to verified bold,
italic and underline; other tool fidelity is tracked under T03.

## Reference navigation (T02 controlled scope)

The binding intercepts native block-ref spans and retained `wiki://blocks/ID`
links before legacy handlers. Stored `siyuan://blocks/ID` is recognized only as
existing-data compatibility; no source URL or AST is rewritten. Original URL
text must match the strict grammar (no credentials, path normalization, query
or fragment). Attachments and ordinary external links keep their own handlers.
Exactly one matching node in the current owning editor can be focused/scrolled;
this does not fetch, adopt another source hash, save or open a protocol handler.
Composition/unconfirmed input and busy history/upload/export prevent jumping.

Nonlocal references use the verified private MessagePort, bound to the channel,
store/page/session and source document. Navigation errors are nonfatal, not save
acknowledgements. The host-only [resolver](../../../../../src/features/wiki-runtime/services/referenceResolver.ts)
uses existing pages/list, document.list and block.get core reads, including every
cursor and child document. It never parses AST or sends root credentials to the
frame. Duplicate block IDs across pages fail explicitly rather than selecting
the first match; missing-block errors must still carry the expected document
hash. Source metadata/document versions are checked again after scanning.
This is optimistic validation across RPCs, not an atomic whole-store snapshot.
Work is bounded (4096 pages/documents, 10000 reads, 20 seconds); exceeding a bound
is an error, not a partial match. A changed owner/generation invalidates results.

For a same-page target the host disposes/drains the old browser owner, waits for
dirty reports, closes its native lease and mounts the target document. Its exact
resolved hash and block must match the rendered source before ready/focus. A
failed input/IME preflight preserves the old owner. Navigation and flush use
separate request lifecycles, so navigation cannot deadlock its own disposal.
Mount validates the target while hidden; after showing the frame, a distinct
focus request/acknowledgement establishes real editable keyboard focus. This
acknowledgement never acts as a save/clean receipt. A parked owner cannot accept
a late lookup result.

Cross-page navigation uses the production [route adapter](../../../../../src/features/wiki-runtime/services/referenceRouter.ts):
after draining and closing the old owner, it synchronously selects the sidebar
page and the existing runtime/tab activation state. WikiRuntimeSurface still
owns native activation. No deferred search message or second router is created.
A one-shot document/hash/block hint is consumed only by an exact base URL,
store/session/page/mode activation. An unrelated page, generation, explicit
document choice, release or discard drops it; old lookup results cannot override
a manual sidebar/search intent, tab/module change or parked view. Failure to
confirm input prevents selection changes. Failed target mounting reports an
error instead of dropping version validation or substituting another document.
Explicit navigation follows document-list cursors if the target is outside the
normal first page; the complete document-tree/selector UI remains T09.

Unhosted bindings report `reference_requires_scoped_resolution`; hosts without
a page router report `reference_navigation_unavailable`. Neither asserts a
missing target. This scope is renderable native blocks in Wiki/Docx views;
ambiguous IDs, unsupported page modes and unavailable block views fail clearly,
without opening legacy protocols. Physical macOS/IME and full-tool parity are
not claimed by T02.

## Editing tool matrix (T03 audit in progress)

This is an audit baseline from the current factory, portable UI preferences,
menu guard, core codec and tests—not a claim that every legacy command works.

| Tool or interaction | Current boundary | Evidence / remaining acceptance |
|---|---|---|
| Paragraph text and basic keyboard editing | Native transactions and CAS | Actual keyboard, source bytes and queued saves covered; heavy-history latency remains to measure |
| Bold / italic / underline | The only enabled floating-format tools and shortcuts, centralized in tools.ts | Each has real keyboard/native-save and byte-exact undo/redo coverage; mixed text formatting preserves adjacent code/link/reference and opaque block metadata |
| Heading, table, list and code text | Native editing for covered structures | Heading/table/list/code edits, last-cell Tab and list Enter have disk/history coverage; not all structural commands or anonymous metadata changes are supported |
| Gutter copy ID / top-level delete / undo / redo | Controlled T01 menu | Exact opaque subtree/last-block restoration covered; OS clipboard verification is T06 |
| Block references / child documents / cross-page navigation | Controlled T02 host routing | Private identity, ambiguity, versions, source drain, activation and Wiki↔Docx child round trip covered |
| Attachment upload / image and attachment reads | Scoped native assets | Real PNG/TXT bytes, offline images, downloads and A/B isolation covered |
| Image double-click gallery preview | Explicitly disabled before the legacy handler | No old getDocImageAssets request or source change; inline image display, attachments and exports remain available |
| Native / HTML / Word / PDF export | Read copies from native source, not Markdown | Source and attachment assertions covered; PDF engine is macOS WebKit, other platforms and physical acceptance remain limited |
| Query/script/database/chart nodes | Preserve native source, do not execute unsupported dynamic tools | Passive rendering / network blocking covered; expand mixed-node fidelity cases before claiming full preservation |
| Properties, conversion, batch tools and gutter drag/fold | UI entry guarded/disabled | Do not infer UI support from a core action; native source must remain unchanged on blocked gestures |
| Slash/reference/template/emoji insertion hints | Suppressed before rendering/filling, including late ambient callbacks | Plain `/` and `((` input still saves; explicit unavailable actions are nonfatal, never save acknowledgements |
| Formula / code language, more, refresh and render editor | Explicitly disabled; native formula/code source preserved | Root capture blocks legacy controls; showRender/showCodeLanguage also reject direct/keyboard callers before old DOM/API work. Plain code bodies remain editable |
| Annotation / memo / tag / table overlay tools | Special-control click/double-click disabled | Does not block ordinary table cells or controlled block references; shortcut/context variants still require audit |
| Legacy hover / explicit reference popovers | Disabled in permanent portable windows | init/show/getTarget/hide and delayed/in-flight responses recheck boundary before inspecting data or creating panels; no replacement preview is promised |
| Other shortcuts, plugins, legacy services and preferences | Unconfigured/blocked or awaiting per-entry audit | No old API/plugin fallback; remaining keyboard/context and clipboard entry points still need T03 review |

The first T03 implementation batch adds native formatting/mixed-inline fidelity
and unavailable-tool tests. Untouched mixed opaque inline, anonymous table-row
metadata and dynamic-query subtrees are asserted byte-exact; large integers are
checked lexically and never persisted through parsed JS numbers. The tested
mixed-inline edit changes ordinary text formatting while retaining adjacent
code, URL and block-reference semantics. It is not a guarantee for editing all
unknown inline fields, anonymous structure changes or every dynamic node.

Unsupported Hint.render/genHTML/fill paths are blocked before old API dispatch,
data rendering or insertion. The permanent portable-window boundary also blocks
ambient callbacks without a binding. Normal typing quietly suppresses hints;
an explicit unsupported action reports portable_tool_unavailable without
failing the session or clearing dirty input. Gutter drag and attribute gestures
likewise leave source unchanged; later ordinary typing must still save.

The second batch covers native-rendered formula and code-setting controls,
code-language Alt+Enter, and code-body editing followed by exact native undo.
The portable keymap no longer binds legacy focus/unfocus to Alt+Enter or
Shift+Alt+Enter: those workspace commands bypassed the code-language tool and
froze editing through the old-API rejection path. Controlled reference/child
navigation remains the host router's responsibility.
Formula and opaque code metadata survive unrelated edits byte-exact. Popover
tests execute the production listener and async entry point (only dependencies
are mocked), including a response arriving after permanent boundary activation.
Portable boot does not register legacy hover listeners; these tests protect
against accidental initialization and pre-existing late callbacks, not a new
human-facing preview feature.

Next T03 work is remaining keyboard/context/clipboard entry-point audit and
corresponding fidelity cases. Do not enable old APIs to make an uncovered
command appear functional. Keep read-only preservation distinct from editable
support; this matrix remains in progress.

The long second-batch editing/navigation regression exposed
reference_lookup_timeout. Native RPC timing confirmed repeated source-lock
admission/journal validation within a logical request. The approved core fix
passes a scoped ReadContext admission through workspace.WithReadContext; nested
reads reuse validation only while that same source lock lives. Cross-request
identity/recovery checks remain, late/cross-source contexts fail closed, and
concurrent borrowers drain before unlock. The same full regression now passes:
the later 14-read lookup's native RPC sum fell from about 20.7s to 4.5s on the
synthetic history fixture. This is a measured run, not a general performance
guarantee. The 20-second resolver budget and navigation/source-byte assertions
are unchanged; broader heavy-history performance and physical acceptance remain.

`new EditorSession(loopbackOrigin, view, options)` captures an immutable view.
HTTP uses X-Wiki-Editor/X-Wiki-Session, no cookies/redirects/CORS fallback.
Resource/event URLs contain explicit store/page but no credentials. WS protocols:
wiki.v1, wiki.editor.TICKET, wiki.session.GENERATION; only wiki.v1 is echoed.
Cross-origin requests/subscriptions fail. No stale-credential auto-reconnect.

Capabilities cannot control/recover/rebuild/list the store, mutate page metadata
or notebook configuration, or access another page. docxSingle content is limited
to its entry document/page assets; opening a single view requires existing data
and never creates/mounts/unmounts notebooks. Metadata operations use trusted host
services. Close revokes future requests and closes that view's sockets; already
admitted operations may finish, so drain browser queues before close. Store
release revokes all tickets; resume requires new tickets and a fresh source read.

## Save, history and lifecycle

- readDocument returns raw AST JSON text/hash. Preserve unknown members and large
  numbers lexically; never JSON.parse/JSON.stringify native AST. rawProperty
  extracts lexical fields; parsed metadata is used only for identity/hash/receipt.
- saveDocument requires a read, serializes its queue, captures target IDs, AST
  and operationId at creation, and advances CAS only after sourceCommitted. Hash
  comes from its own exact raw put bytes, not a later read that might adopt a
  different writer's hash and mask a conflict.
- Failure/uncertainty blocks subsequent saves and retains dirty state; no fake
  clean/release or automatic undo/retry of uncertain committed operations.
- EditorSessionGroup.prepareRelease freezes all sessions before awaiting every
  queue; any unsaved input prevents host release. Resume must invalidate old
  sessions/history, issue new capabilities and reread source; old caches cannot
  be re-enabled. Actual Protyle getDoc, transaction (including inverse/forward
  Undo operations), reload and destroy now branch through a per-instance binding
  before the legacy WS/global queue. This does not adapt every legacy tool.
- Delayed reads/assets are rejected after freeze/invalidation. Events carry
  immutable identity and stop after invalidation. Socket errors/resyncRequired
  mark stale, not automatic saved. Mounting UI must also check activation epochs.

Authenticated assets use owned Blob URLs revoked on freeze/invalidate/close.
Native `.docj` export drains the save queue before a scoped raw read. HTML uses
`document.export` (same CLI/Server service) for one publication snapshot of the
native document and referenced page assets, never live editable DOM or Markdown.
All toolbar exports freeze input while reading, reject IME/pending uploads and
block release; detach/stale callbacks never trigger a download or re-enable an
old view. Exports do not advance the save queue's CAS baseline. A failed read-only
export restores the current editor without falsely failing or saving its source.
Uploads are also rejected during native history restoration, before reading files
or creating assets; the in-progress undo/redo and active session remain usable.
Limits: 16 MiB input, 32 MiB response/asset,
30-second fetch/body timeout.

Word export calls document.export with format:docx and receives base64 data,
mediaType and the captured source hash. The core uses Lute's semantic DOCX HTML
projection, the same sanitization/scoped attachment snapshot as HTML, then fixed
Pandoc arguments outside the publication read barrier. No Markdown source or
second editable truth is introduced. Headings/tables/lists/code and supported
raster images are reading projections, not full fidelity for every native node.
Ordinary attachments/media/dynamic blocks are not promised as embedded Office
objects; use native/HTML exports for complete attachments.

PDF export uses document.export format:pdf with the same inert HTML/source/asset
snapshot, then macOS wiki-pdf outside the publication read barrier. CLI/Server
resolve the sibling helper, or accept a trusted --pdf-helper absolute executable
override (only for PDF export in CLI). Page JSON cannot select helper paths or
arguments. Native sources and editor CAS remain unchanged; absent helper/timeout
affects export only. Ordinary reads/editor startup do not initialize AppKit.
The toolbar exposes “导出 PDF” with the same input freeze, release preflight and
late-response checks as Word/HTML. Non-macOS reports converter_unavailable.

The helper links only system AppKit/WebKit/PDFKit; scripts/wiki-pdf.sh builds
arm64 or x86_64 with a macOS 11 deployment target. It takes bounded UTF-8 HTML
stdin in a private working directory, uses a nonpersistent website data store
and private rule store, disables content JavaScript, blocks all resources except
data/about:blank and denies navigations away from about:blank. It waits for
decoded data images/fonts, then uses asynchronous silent NSPrintOperation to
save paginated A4 PDF locally; no print/save dialog or foreground window is
requested. The core captures stdout, never accepts user source/output paths and
cleans its private temp directory. Helper deadline 18 seconds, parent conversion
deadline 20 seconds, 16 MiB input/output, at most 200 output pages and bounded
preflight document height. Non-image attachments/dynamic blocks are reading
projections, not full native backups. Exact page breaks/layout remain system-
renderer dependent; no PDF/A/accessibility/embedded-attachment guarantee.
Rendering needs available macOS graphical/WindowServer services but does not
require IronMan to be running. SSH/no-desktop execution is not accepted as tested;
failure/timeout leaves ordinary CLI reads and editor saves usable.
Quartz/PDFKit may extract CJK compatibility radicals; tests normalize text to
NFKC for comparison only, never rewrite native source or output PDF.
--inspect-stdin reads bounded PDF bytes and emits page/text metadata without
starting WebKit; it is a local verification command, never a page-facing API.

The host's --app-dir resolves pandoc/pandoc-GOOS-GOARCH.zip already present in the
package, or an operator supplies --pandoc ABSOLUTE_EXECUTABLE_OR_ZIP. CLI uses
the same --pandoc option only for document.export format:docx. The browser cannot
set a converter path, filters or command arguments. No PATH discovery, download
or old workspace configuration. Missing tools fail only the export; editing and
normal reads remain usable. Each conversion has a private temporary HOME outside
the source, fixed --sandbox/HTML/DOCX arguments, no user filters/templates, a
20-second deadline including the per-handler serial gate, 16 MiB binary output
limit and cleanup. Archives extract only one regular bin/pandoc (bin/pandoc.exe
on Windows), max 256 MiB, with no general ZIP extraction. Pandoc --sandbox is an
additional I/O restriction, not a claim of OS-level isolation for arbitrary
executables: only trusted application/operator resources may be configured.
Tool stderr is not returned to clients. Converter errors neither mutate native
source nor poison the save queue. Detach/stale/dirty checks discard late results.

HTML is self-contained with a static stylesheet, title and CSP. Fixed raster/
audio/video MIME types become data previews; attachments are opaque data-URL
downloads. Scripts, event handlers, user CSS, forms/frames/SVG/MathML and remote
embeds are removed; external HTTP(S) links require a user click. This is a safe
reading projection, not arbitrary interactive HTML/complex-widget fidelity or
an archival replacement for `.docj`. Missing/unsafe assets fail the whole export,
never produce a silently incomplete success. Limits: renderer body 4 MiB,
128 unique assets, 8 MiB unique decoded assets total, 12 MiB encoded references
(including repeats), final HTML 16 MiB/JSON string 24 MiB. Oversize returns
`export_size_limit`. The root/source paths and browser credentials are not embedded.
Uploads use create-only `asset.put`, fresh UUID filenames, 8 MiB per file and
at most 20 files per action. File reads themselves enter the owning save queue;
only a sourceCommitted receipt permits insertion through native Protyle
transactions/history. Selection and page ownership are captured before reading.
The binding disables editing during reads and refuses release until upload and
insertion finish. Failed/uncertain writes keep dirty state; a committed asset
whose document insertion fails may be orphaned, never automatically deleted or
reported as an inserted attachment. Detached callbacks cannot insert elsewhere.
Attachment links are intercepted before legacy navigation, fetched with the
page capability and downloaded as owned blobs. MIME types for preview are a
fixed raster/audio/video allowlist; HTML/SVG remain opaque downloads. Remote
embeds are not fetched. Blob URLs never become persisted AST references.
Independent page queues submit concurrently; source publication remains serialized
in the core without treating its temporary barrier as workspace closure.

## Native Protyle binding and remaining boundaries

### Audited dynamic tools (not feature parity)

`boundary.ts` marks a dedicated portable window permanently; it carries no
workspace/page selection and detaching a view cannot restore legacy behavior.
The legacy fetch helpers and render dispatchers use this same marker. In
portable windows:

- blockRender stops before evaluating stored `//!js` queries or passing a
  capability-bearing Protyle instance into document code; AV rendering stops
  before legacy database access, including direct refresh entry points.
- processRender and the nine individual ABC/chart/Graphviz/math/Mermaid/mindmap/
  flowchart/PlantUML/HTML renderers return before adding scripts, executing
  callbacks or creating remote previews. This also covers direct caller paths
  that bypass processRender. Dynamic tool parity is still unimplemented.
- looseJsonParse uses strict JSON.parse, never Function evaluation. Legacy
  windows keep their previous parsing behavior. This is a specific tool boundary,
  not a claim to sandbox all arbitrary JavaScript in a browser.
- Native heading, table cells, list paragraphs and fenced code keyboard edits
  and inverse/forward history are covered against the real kernel. Table opaque
  metadata, code language and other-page isolation are checked. This is not
  acceptance of every table structural command, list reparenting or complex node.

Last-cell Tab adds a table row, and Enter at a list paragraph end creates a list
item; both now have native disk/undo/redo coverage. A structural whole-block
update may remove ordinary projected children (necessary for undo of a newly
created list item), while hidden metadata on surviving stable IDs is retained.
Hidden anonymous row/inline metadata plus a child-count change is rejected,
rather than moving it to a shifted neighbour by positional matching.

Undoable portable transactions request `nativeHistory:true`. The editor
receives a history ID and committed source hash after the original index is
ready. It keeps only the operation ID, document ownership and adjacent hashes.
`document.history` resolves the exact journal image in the kernel and uses the
same queue, CAS and durable writer. It refuses foreign or out-of-order history.
Undo and Redo use the original 64-entry UI limit; new edits clear Redo and
non-history writes invalidate previous history. The transport has no document
snapshot or 4 MiB history limit; the source transaction limits still apply.
Restore disables input, sets aria-busy, blocks release, and refocuses a surviving
editable block. Late completion cannot remount a detached or stale view.
HTML-only requests still reject opaque deletes. With owned native history,
removing a stable-ID subtree is reversible; ambiguous anonymous row or inline
changes fail before publication. Failed requests keep the editor dirty.

The toolbar makes unavailable dynamic tools explicit. Their native source is
retained, not rewritten into a placeholder or silently deleted. Safe native
editing/exporting is not permission to re-enable old graph/database/query
services. Each future adapter must bind page/session and implement proper
late-response, failure and read-only/fidelity handling.

The normal editor loading icon has an exact static exception for
`/stage/loading-pure.svg`, already present in the trusted build output. Other
arbitrary `/stage/` files, traversal, symlinks and source SVG remain forbidden.
Both package verification gates require this nonempty file; the synthetic
resource-gate test is not a real signed bundle/DMG acceptance.

`protyle.ts` owns a fixed document and render epoch. Beforeinput/composition are
dirty until a recognized native transaction captures them; active IME or unknown
operations cannot be acknowledged as saved. A detached/frozen view cannot render
late reads. Foreign mutation events invalidate source/history rather than reload
over input; own queued operation events do not invalidate their owner.

The shared core's document.render/document.transactions actions convert native
AST ↔ Protyle BlockDOM with existing Lute/dataparser, without Markdown or a new
source representation. Supported operations use the same CAS/journal as CLI.
Opaque native metadata/big integers survive covered edits; ambiguous unsupported
inline changes fail closed. This is not a blanket fidelity guarantee for every
legacy node or tool. Receipt replay without an exact committed hash requires a
fresh read, never automatic CAS adoption.

`mount.ts` creates the actual Protyle, not a textarea. Its dedicated window must
already have source-independent UI configuration/languages/Lute/CSS initialized.
It forbids plugins, blocks legacy shared fetch helpers permanently in that
window, and sends supported file uploads through its fixed binding. No old getConf fallback
is allowed. `boot.ts` initializes source-independent window-local preferences
and bundled language/Lute/CSS, then accepts one verified parent handshake. Its
fragment contains only parentOrigin and a public random channelId (16–128 ASCII
letters/digits/hyphens); capabilities arrive on the port. The parent must wait
for wiki:editor:booted from that exact frame/origin/challenge before connecting.
The current defaults intentionally leave unsupported tool shortcuts disabled;
undo/redo and bold/italic/underline shortcuts are enabled. This is not the full
legacy preferences UI. Asset images become scoped owned Blob URLs and are restored before writes;
remote embeds are blocked, not automatically fetched. Scoped attachment links
and upload are connected; unadapted tools/direct URL consumers and physical IME/
macOS webview acceptance remain S07 work. Whole-host source switching and
full recovery UI still require integration/acceptance. The basic host release/
resume and main-window close protection are connected; explicit discard needs
a second confirmation and successful native stop first.

Start the portable Server with optional `--app-dir TRUSTED_APP_RESOURCES` (the
wiki-app build output, not its source folder). The app directory must be disjoint
from the knowledge source. Only portable build, Protyle runtime and bundled
appearance/emoji assets are served. Hidden paths, directory listings, symlinks,
source paths and legacy APIs are denied; the listener rejects nonmatching Host.
Static responses have CSP/nosniff/no-referrer/no-store. No source credentials or
getConf are needed to load the shell. Rust now locates the built app beside bin
and passes --app-dir when the controlled bundle is present; the ready record's
editorAssets flag gates editorReady/portable-editor. Missing assets keep the
backendReady/API-only guard. Rebuild both native and app assets for this path.

WebSocket subscriptions are live from their captured server tail, not a replay
of historical edits. `subscribe()` resolves only on workspaceReady, and the
factory awaits that before its first source read. A delayed foreign mutation
invalidates the view; it never adopts another writer's source hash silently.

## Build and verification

webpack.portable.js builds stage/build/portable/session.js (UMD session/binding/
channel library); webpack.portable-protyle.js builds the actual Protyle factory
as portable/protyle.js plus portable/boot.js, index.html and CSS.
scripts/wiki-app.sh includes both while restoring the pre-build source stage tree.
No new dependency was added. From the IronMan root:

```sh
npx vitest run src/__tests__/wikiEditorSession.test.ts src/__tests__/wikiProtyleBinding.test.ts src/__tests__/wikiSharedBridge.test.ts
npx vitest run src/__tests__/wikiRenderBoundary.test.ts
npx vitest run src/__tests__/wikiPortableMenu.test.ts src/__tests__/wikiPopoverBoundary.test.ts src/__tests__/wikiPortableTools.test.ts src/__tests__/wikiPortableUI.test.ts
npx vitest run src/__tests__/portableEditor.test.ts src/features/wiki-runtime/components/__tests__/WikiCloseNotice.test.tsx
node node_modules/typescript/bin/tsc --noEmit --strict --target ES2022 --module ESNext --moduleResolution bundler --lib ES2022,DOM --skipLibCheck public/wiki/wiki-app/src/portable/index.ts
GOPROXY=off WIKI_KERNEL_OUT_DIR="$PWD/src-tauri/target/package-resources/bin" bash scripts/wiki-kernel.sh
node --test scripts/__tests__/wiki-editor-session.test.mjs
# Uses the existing host-architecture archive, or WIKI_TEST_PANDOC override:
node --test scripts/__tests__/wiki-docx-export.test.mjs
# macOS, real system WebKit/PDFKit; optional WIKI_PDF_EVIDENCE_DIR for QA copies:
node --test scripts/__tests__/wiki-pdf-export.test.mjs
# With local Playwright Chromium already available; no automatic browser install:
WIKI_APP_SKIP_INSTALL=1 WIKI_APP_OUT_DIR=/tmp/wiki-app-test bash scripts/wiki-app.sh
WIKI_TEST_APP_DIR=/tmp/wiki-app-test node --test scripts/__tests__/wiki-editor-browser.test.mjs
WIKI_TEST_APP_DIR=/tmp/wiki-app-test node --test scripts/__tests__/wiki-editor-tools.test.mjs
# Optional focused diagnosis, NOT a replacement for the full regression above:
WIKI_TEST_REFERENCE_ONLY=1 WIKI_TEST_APP_DIR=/tmp/wiki-app-test node --test scripts/__tests__/wiki-editor-browser.test.mjs
# Requires an existing root dist from build:web; synthetic resources test only:
node --test scripts/__tests__/wiki-package-resources.test.mjs
```

The editor browser/tools, session, entrypoint and DOCX export tests also accept
WIKI_TEST_KERNEL_BIN / WIKI_TEST_CLI_BIN to exercise freshly compiled temporary
binaries without replacing package resources or an installed app. Their binary
directory must contain the existing wiki-pdf and wiki-compat sibling helpers
for PDF/compatibility checks (unchanged helpers may be linked there). This does
not rebuild those helpers or establish a signed/package/DMG acceptance result.

The native test compiles this actual library, starts actual kernel/CLI on a
synthetic source, runs A/B concurrent saves, A→B→A/mode views, validates exact AST
bytes/large numbers, native BlockDOM transactions and inverse/forward operations,
drains/releases, edits with CLI and resumes with new tickets
without PID/port restart. Unit tests cover delayed responses, failures/dirty,
freeze, resources/events, composition, asset-reference restoration, and a real
MessageChannel handshake with test DOM ports (not mounted Protyle UI). Go tests
cover native rich-text operations, HTTP/assets/WS scope/admin denial and
revocation; Rust tests exercise native capability creation through the bridge.
The browser test mounts actual Protyle in controlled iframes against a real
kernel/CLI, types via keyboard, checks disk commits, undo/redo/bold, two page modes
and isolation, clean dispose/release, CLI changes and a fresh source after resume
at the same address. It blocks nonlocal HTTP and checks no legacy requests/root
Bearer leakage. It also compiles the exact production PortableEditor owner,
replaces only native IPC with authenticated Node-side Server RPC, and exercises
stable hide/reattach, typing, failed composition preflight and successful retry.
It now also clicks a real nonlocal reference, drains/replaces the production
owner with a same-page child document, checks exact source bytes and target
selection, types without an extra manual click, verifies the full edit on disk
and releases normally. Native session tests separately exercise ambiguous
cross-page IDs, unique cross-page lookup and missing-block results. Long edit
history can leave acknowledged saving work queued beyond the former 12-second
test poll: the last two typing checks allow up to 60 seconds (240-second full
test cap), without relaxing source-content or release assertions. That is a
test waiting budget, not a latency guarantee; queue performance remains to be
measured/improved in integrated acceptance.
It additionally uploads PNG/TXT through the real editor, checks exact source
asset bytes, authenticated attachment downloads, exact native export bytes,
Word downloads with exact embedded PNG bytes, semantic heading/table/code text
and A/B-isolated DOCX content. DOCX errors/delayed exports are covered by unit
tests; the real CLI also executes the bundled Pandoc outside the repository.
The browser also checks same original filename isolation across pages and persisted images after a
fresh mount with nonlocal networking blocked. Scroll lazy-loaded images into
view before asserting decoded dimensions; disk commit alone does not mean the
upload/insertion callback has drained or the view is releasable.
The HTML export test opens the actual downloaded bytes in a separate network-
blocked page without kernel/editor resources and verifies decoded images and
exact attachment downloads. CLI integration exercises document.export outside
the repository with an isolated HOME; HTTP tests reject cross-page/entry export.
Native Rust lease/asset tests separately cover that part of the bridge; this is
not a real macOS webview IPC/physical-IME/close-button test.
Browser-native fetch is explicitly bound to globalThis; transient
<wbr> markers are stripped by the native codec without deleting literal carets.
These are not physical IME/macOS webview acceptance or completed S07.

### Device-scoped writing and previews

The desktop host looks for private `ai-provider.json`,
`preview-origins.json` and `cloud-account.json` files in its `wiki-runtime`
directory under app data.
Set the files to mode 0600 and keep them outside the knowledge source. Each
profile is tied to the `storeId` in the source manifest. A standalone
`wiki-kernel serve` can select the same files using `--ai-file` and
`--preview-file` and `--cloud-account-file`.

- The writing profile contains `storeId` and `openAI`, with `apiKey`,
  `apiBaseURL`, and `apiModel`. It accepts HTTPS and explicit numeric loopback
  HTTP for local providers. The desktop writing dialog sends the submitted
  prompt and recent conversation for its page. Conversation stays in the kernel
  process and survives saves. The client refuses redirects.
- The preview profile contains `storeId` and optional `origins` and
  `widgets`. List origins as scheme, host and optional port. Image, audio,
  video and iframe URL blocks keep their original URL on disk. The preview
  request happens on click, refuses redirects and sends no editor credentials.
  Iframe preview defaults to static text and structure. An origin already
  listed in `origins` can also appear in `interactiveFrames` to allow its inline
  JavaScript in an opaque-origin preview. The optional `frameScripts` map
  keys an exact preview URL to its same-origin JavaScript paths and SHA-256
  digests. The kernel fetches and verifies those files on preview, then serves
  an expiring snapshot. The optional `frameAssets` map uses the same exact
  preview URL and maps same-origin CSS, image, audio, or video paths to SHA-256
  digests. Only referenced assets are fetched, with 1 MiB per file and 2 MiB
  total. Approved bytes are embedded in the opaque frame. Plain stylesheet
  imports and approved relative image and font URLs are resolved from the
  same manifest, with bounded recursion and expansion. Conditional CSS
  imports and arbitrary frame network access remain blocked.
- Local form controls dispatch submit events inside the frame. For Desktop
  previews, an optional `frameForms` entry binds one exact preview URL to an
  absolute same-origin `action` and its allowed `fields`. A matching POST
  form can present those fields and the destination for review in the editor.
  The kernel sends them only after the separate host action. It sends no
  browser credentials, refuses redirects, and returns bounded plain text.
  List explicit CSRF inputs under `hiddenFields` and approved file input
  names and MIME types under `files`. The editor confirmation chooses the
  files and shows their destination. The kernel validates their type and
  size and encodes multipart data. Up to two files of 256 KiB each are
  accepted. Password controls stay local.
- The optional private `frameSessions` map binds an exact form preview URL
  to its device-provisioned cookie names and values. The kernel maintains a
  separate in-memory cookie jar per preview URL, including session rotation.
  The jar participates only in the exact preview GET and approved form POST.
  It is excluded from nested resources, renderer responses and source data.
- The `widgets` map binds a widget directory name to the SHA-256 of its
  `index.html`. The picker uses the installed widget catalogue for the current
  desktop page. Add `widgetScripts` with the widget name mapped to an object
  of relative `.js` or `.mjs` paths and SHA-256 digests. Preview checks the
  digests and serves an expiring byte snapshot to an opaque-origin frame.
  Classic scripts and modules can load approved package files, including
  relative module imports. Editor APIs and external network access remain
  unavailable to the frame.
- The optional `plugins` map binds a locally enabled Desktop plugin name to
  the SHA-256 of its `index.js`. The isolated CommonJS loader supports the
  original `Plugin.onload` and `protyleSlash` registration shape. An invoked
  slash callback can insert bounded plain text through the editor's native
  transaction. The bridge also runs `onLayoutReady`, maps `addCommand` to
  searchable editor commands, and renders `addTopBar` as editor toolbar
  buttons. `showMessage` displays bounded plain text after invocation.
  The event bus receives the initial document load and clicks in the active
  document as identifiers. It never receives the host editor or DOM.
  `Setting`, `addDock` and `addTab` with the custom form of `openTab`
  display plugin-owned elements inside the same isolated frame. Only one
  panel per plugin is shown, and it opens through an editor command or button.
  An optional `pluginHotkeys` map binds approved plugin names and their
  command `langKey` values to original shortcut chords, for example
  `⌥⇧⌘J`. A binding applies only in the current editor, ignores IME and
  repeated keys, and does not take priority over existing editor handlers.
  A separate `pluginSystemHotkeys` map binds plugin `globalCallback`
  commands to approved macOS chords of the form `⌥⇧⌘K`. The host registers
  shortcuts only for the current active Wiki view. Parking or closing the
  view removes the registration. Dock entries can choose the Wiki layout's
  left, right or bottom region, and `addStatusBar` adds a scoped control to
  the Wiki status region. The global shell and other legacy modules retain
  separate interfaces.
- List hash-approved plugin names in `pluginStorage` to enable `loadData`,
  `saveData` and `removeData`. They use the original
  `storage/petal/<plugin>/<file>` directory in the current Wiki page. Each
  call accepts one filename and at most 64 KiB, with a 64-entry directory
  limit for new files. Source writes share the editor save queue and the
  normal source transaction, including operation replay and conflict checks.
  Plugin enablement and the approved code hash are checked again on every
  request. Binary files and nested storage paths are outside this profile.
- The cloud profile uses `storeId`, `region`, `origin` and an optional
  `bootstrapToken` provisioned on the device. Region 0 uses
  `https://siyuan-sync.b3logfile.com` and region 1 uses
  `https://siyuan-cloud.liuyun.io`. Numeric loopback HTTP is accepted for
  isolated testing. The Desktop account dialog can connect, refresh or clear
  the saved session. It receives only availability and connection flags.
  Clearing the saved session leaves device provisioning in place. The account
  profile enables account access only. Remote repository operations and
  synchronization require separate runtime bindings.
- A profile can set `allowPasswordLogin` to enable the original account
  sign-in protocol for its fixed region. The original password digest is
  sent through the local authenticated adapter. The device fetches the
  CAPTCHA image, keeps CAPTCHA and 2FA challenges scoped to the originating
  editor view, and retains the resulting session in the private state file.
