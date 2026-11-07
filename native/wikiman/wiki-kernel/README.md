# wiki-kernel

Native document lifecycle, archive import, retained deletion and historical
versions share the CLI and authenticated HTTP service. See the
[document lifecycle contract](internal/wikiops/README.md#document-lifecycle-r05).

Original edits use one `wikiops.Edit` service for the native CLI, authenticated
HTTP and scoped Protyle. The shared source transaction records exact undo/redo
images in the kernel and returns an operation handle. See the
[original edit contract](internal/wikiops/README.md#original-edit-transactions-r04).

Current R03 acceptance (2026-11-05, done; 11/11 work packages, zero remaining TODO): the native CLI `query` command
and host-authenticated `POST /v1/query` share the 68-action original read registry,
checked source admission and disposable original indexes. See the
[read/query contract](internal/wikiops/README.md#original-readquery-services-r03).
The current execution-plan/evidence in AnonyPower controls stage acceptance;
older dated “R03 pending” notes below are historical checkpoints. The original UI adapter has a separate R06 gate.


The portable kernel uses explicit workspace contexts and shared application
services. The original server remains in `internal/legacy`, isolated in the
temporary `wiki-compat` executable for the unconverted GUI (S06–S08). Portable
wiki-kernel/wiki-cli do not statically import legacy or its initialization workers.
Original third-party copyrights and licenses remain with their sources.

## R02 original runtime lifecycle (historical implementation notes)

The original model no longer starts its transaction worker or registers its five
event-subscription groups at package import. `model.StartLegacyRuntime()` is
called explicitly by the compat, mobile and Harmony startup paths. Its original
`performTx` callback now runs through an independently owned
`internal/txqueue.Queue`; admission, serial execution, drain and close are
synchronized. Closing rejects new submissions, waits for accepted work and joins
the worker before databases close. Early sync/update decisions do not close the
queue. Transaction API adapters propagate admission errors rather than returning
success or waiting for an unadmitted transaction.

The current R02 business batch shares AppConf notebook/language dependencies,
four original reference aggregates and the FIFO history-index queue with Context
services. Context history flush tasks do actual original index work. performTx's
load/stage/finish/parser dependencies are explicit, but only create/update-time/attrs/fold-heading/unfold-heading
are audited in its private scoped seam; remaining operations are denied there.
The compat adapter remains process-scoped. It admits HTTP only after databases
and single-mode bootstrap are ready; getConf is read-only and never bootstraps a
notebook. Original settings controllers, other queries,
transaction receivers and original background/event businesses remain pending.

Classified persistence now includes shared store source preferences at
.wiki/preferences.json, page source preferences and local device CAS. Main queue
content updates and nine delete/rename families share original SQL algorithms;
index/upsert now join the same Context FIFO from actual detached source trees; original update_refs/index_node
now read explicit page/document source and enqueue owned projections. Typed
asset/embed queries and the complete asset-content queue are also shared. Event
callbacks retain the publishing lease, run outside the DB gate and can be removed
and drained. The original index-changed/flushed state business is shared with a
Context-owned callback. Five original SQL progress formatters are also shared;
Context uses its own copied language catalog and explicit sink for real history,
asset, delete and rename progress. The original block-insert Context producer now publishes committed tree batches
with owner/generation, original count/hash and language policy. Compat still registers 49 callbacks; the other 42
business bodies and WS routing remain pending. Unfold-heading
uses explicit parser/reference/AV-name dependencies and post-commit owner reload,
not a detached waiter. Its staging seam is NOT authoritative source commit (R04).
Content/refs use the original extraction rules (OCR/AV require explicit
owner-bound NativeContent readers, never silent fallback). Ref replacement is one
transaction with 512-row batches and copied records. This is derived index work,
not a source-save handler. No new public command or endpoint is exposed.

Resource factories now also share the four **original** database schemas and
connection setup (`internal/indexdb`) with sql/treenode. A Context lazily owns
separate main/history/asset/blocktree in-memory indexes through
`Lease.WithNativeIndexes`; these are not the existing small byte-span projection.
Each DB has its own keeper connection, setup is rollback-safe, failed setup
closes earlier handles, and a live lease prevents Context teardown. The callback
is a private service port, not a new SQL endpoint; original query algorithms and
population/rebuild services have not yet been wired here (R03).

`internal/ialcache` extracts original document/block IAL caching into explicit
Open/Close owners. `Lease.WithNativeIAL` bounds each page's optional derived cache;
the compat cache facade no longer launches Ristretto workers at import. Original
SQL caches are explicitly opened/closed too; reference expiry is swept during
use, without an unowned janitor. Cache inputs/outputs are detached. Parser
factories in `internal/nativeparser` take original syntax/editor options by value;
util/model use those same factories, and `Lease.WithNativeParser` confines a fresh
mutable parser to the request. Lute's shared default emoji dictionaries are
detached in both directions before custom entries can be applied; a fresh Lute
pointer alone is not isolation. Default editor instances no longer share one
Markdown-options pointer. No factory saves defaults or registers GUI/providers.

Original `AppConf.SaveError` now saves its receiver using a private absolute-path
binding and read-only authority. Detached import/masking configurations cannot
write; JSON cannot redirect that binding. This preserves the complete original
configuration shape. The full 31-field contract now lives in `conf.AppData`,
embedded by the original AppConf and classified by `conf.ScopeFor`. `conf` no
longer imports util/model/sql/treenode or implicitly reads device/environment
preferences. Original boot/reset callers explicitly supply those inputs.
Context acquisition snapshots a registered identity-based Configuration provider;
`Lease.Configuration` returns a deep copy, and `WithConfiguredParser` uses the
same AppData.ParserOptions as original model.NewLute. Neither call saves defaults
or opens a user config file. Classification does not move old files or migrate
device secrets into the shared source. Full original configuration methods and
save/read services still need contextual dependencies. The complete field policy
and explicit-input contract are in [conf/README.md](conf/README.md).
The original tree index/upsert block writer now shares explicit-transaction
InsertBlocks, SelectUpsertBlocks and DeleteIndexedBlocksByIDs with indexdb.
Old hashes are read through the writing transaction, not a global database.
All 21 columns, 512-row batches, physical ROWID deletion, case-sensitive FTS
policy and original batch hashes remain; compat retains its cache/event hooks.
Context QueueNativeTree now uses the original complete block/span/asset/attribute
walk plus reference/annotation projection. The actual compat index/upsert paths
use the same ProjectTree; old private helpers remain for unmigrated callers and
differential tests. Context reads with ParseJSONWithoutFix: no empty paragraph or
Spec migration is synthesized. All 14 audited main queue families share one FIFO.

Attachments are resolved read-only within the page data root, preserving original
doc/box/page precedence and Qiniu ETag, rejecting traversal/symlinks. Context reads
page data/.wiki/indexignore without creating defaults. The original post-deletion
ignore semantics of upsert are deliberately retained. NativeContent supplies OCR
text/AV readers; NativeOCR supplies synchronous owner-bound preparation, or an
explicit no-op for disabled OCR. Missing/failing services or renderer panic reject
projection before enqueue. ConfigureNativeOCR now explicitly binds the shared
Tesseract engine and owner-local text/preparation services; the owned background
task recognizes private image snapshots outside the source/DB gate, rechecks
hash/engine identity, refreshes FTS and atomically saves reconstructible text under
.local/workspaces/<pageId>/temp/ocr-texts.json. Legacy source OCR text is a read-only
seed. Recognition is opt-in, bounded to eight images per run, cancellable and joined;
pending refreshes survive that batch limit. No executable is installed or started
by Context acquisition. StartNativeOCRLifecycle explicitly configures an owner,
starts its nonrecursive asset watcher and OCR/flush loop, and joins both on
cancellation. Original InitConf no longer launches OCR; compatibility bootstrap
uses a registered cancellable boot task, and cron uses a cancellable cycle.
The original watcher dispatch order and escaped status message are shared;
Context notifications use an explicit owner sink, not a global WS broadcast.
Rich attachment extraction, cloud-sync scheduling, AV lifecycle and the complete
original multi-Context UI adapter remain separate migration work.

Main commits invalidate bounded copied native block/location caches and tree IAL
caches under the owning gate; failed work never publishes cache changes or block
success progress. Original blocktree query/index/upsert/remove/path algorithms
and legacy facades now share indexdb.BlockTrees. An explicit confined source
snapshot builds the owner registry atomically; reference locations use physical
AST parents even when FTS uses heading parents. Registry refresh is independent
of main FTS transactions. Owner asset-list/hash caches share the original naming
rules, with copied snapshots, bounded scans and ETag revalidation. CacheRef now
shares the original single-node projection and owner-local copied TTL cache;
committed main work invalidates that cache. Original upper reference queries,
matching/keywords/recent/graph and reference/container text use explicit DB/parser
services through both actual SQL facades and Context admission. Other upper
fulltext/AV/tag queries and dynamic-reference source mutation callers remain.
These internal services expose no new endpoint.

The compat PDF page workers serialize calls into their pool's shared WASM runtime
(the cached wazero version races on lazy function-type keys). Workers are joined,
including instance cleanup, before the pool closes; failed document opens no
longer dereference a nil response. This may reduce parallel PDF throughput;
performance is not benchmarked. PDF jobs are not yet fully Context-owned.

Original SQL tree projection methods now own an operation-scoped parser supplied
by value; the import-time `sql.luteEngine` singleton has been removed.
The original query/queryRow and main/history/asset Begin/Commit facades now use
the explicit DB/context operations in `internal/indexdb`. The same operations
back `Lease.WithNativeTransaction`, whose commit/rollback/cancellation are pinned
by the page lease and gate. These are derived-index transactions, NOT document
source transactions or durable save receipts. Original higher-level query and
performTx dependency migration is still pending.

Compat subscriptions are grouped by `internal/eventscope`: closing removes model
callback references and joins active callbacks. The upstream event bus has no
unsubscribe API, so one inert ingress relay per topic remains; it does not retain
closed workspace callbacks. This bridge is **not** a contextual application bus.
`internal/taskgroup` owns cron, original task handlers and the history producer.
Admission closes before joining, and UI task timeouts do not release live work.
The filesystem-check loop and version-check retry now accept cancellation;
macOS asset/emoji polling callbacks capture their own watcher and are joined
before DB teardown, not asynchronously nulled. A private BSD-3-Clause backend
fork joins its poll helper and interrupts polling sleep/event delivery on Stop,
while keeping the original polling interval and event semantics. Repeated
100-cycle close/stack checks and create/write/rename/remove tests cover this
specific leak; see [source notices](THIRD_PARTY_NOTICES.md). Pending tasks are cancelled; main/history/asset
SQL queues are flushed before setting the exit flag. Other platforms' watchers,
theme watcher startup/joins, detached jobs inside original business callbacks,
and full original business per-context ownership are still pending audit/migration.
Context StartTask now registers its cancellation owner before launch;
Drain/DrainStore cancel only the affected tasks, and uncooperative callbacks keep
their leases until actual return. StartPeriodicTask uses that same lifetime,
not the compat scheduler. This does not wire global original jobs into Contexts.

### R02 dynamic anchors, upper search and append operations (2026-10-22)

Original `getNodeRefText`, dynamic-reference setters/expansion/seven-round
propagation now share `internal/anchor` with Context candidate planning. The
original compatibility path retains its save/index/notification callbacks.
`PlanNativeDynamicAnchors` reads owner source without fixes, returns detached
ASTs, matching source hashes and pending changes, and does not publish candidate
hints/index rows or save source. AV source effects without an audited binding
fail explicitly. R04 must still provide serialization, the unique source
transaction, history and durable receipts.

Original full-text dispatch and SQL selectors share explicit DB/config algorithms
for FTS, syntax, SQL and regex, counts/paging and multi-keyword document mode.
Native search reads `.wiki/searchignore` without creating defaults; SQL validation
rejects writes/multiple statements, including injected ignore clauses. Backtick
identifier normalization is validation-only, not a rewrite of executed SQL.
Original FullTextSearchBlock now also shares `blockview` presentation/grouping:
the unchanged Block/RiffCard wire types, highlight/escape/truncation, IAL/path,
document/content-order sorting and reference counts. SearchNativeBlockView runs
query plus these reads under one owner admission/gate; this is an internal
service, not the complete original HTTP/WS adapter or a new public endpoint.
Original AV search ranking/cap/views and tag labels/highlighting share cores;
native AV reads existing JSON/msgpack without migration/save and resolves owner
notebooks/blocks. Four original AV key selectors share order/duplicate-ID and
type/name rules; NativeAttributeViewKeys returns detached full key JSON, preserving
unknown fields without upgrading the source. Original primary filtering, bound
block existence, updated-desc sorting, default size and view fallback now share
explicit `avquery` algorithms with Context. Mirror lookup retains batched owner
registry reads; primary rows are still sorted before paging. Out-of-range pages
are empty instead of the old slice panic; nonpositive pages are explicit errors.
ReadNativeAttributeView returns a detached full source snapshot, while
NativeAttributeViewPrimaryValues/NativeAttributeViewView preserve full unknown
JSON in their projections and never fix/save source. View selection reads block
override/current/first and returns raw filters/sorts, not computed rendered rows.
Invalid primary values/rules and known-but-broken source fail without fallback.
Original AV domain types and filter/sort/calc algorithms now live in `internal/avmodel`;
the legacy `av` package aliases the same types and binds its own reader, duration
formatter and error sink. `internal/avrender` shares original table assembly,
basic auto-values, query filtering and manual ordering with actual SQL facades.
`NativeAttributeViewTable` renders detached Spec 4 source under the owner gate,
using original filter/sort/calc before paging and pinned AST attributes for bound
blocks. It never repairs/saves source and exposes no new public handler.
`NativeAttributeViewRender` additionally shares original gallery/kanban assembly,
four cover modes, group generation/order/state and per-group filter/sort/calc/
paging. Bound trees/IAL are request-owned and batch-loaded; cover AST ID/IAL/token
changes are restored. Group generation only changes candidates, never source.
Owner language is required for special group labels. Limits bound fields/cells,
total rendering work and group count; malformed/non-progressing ranges fail.
Dynamic template dependencies and ambiguous/future source formats fail explicitly;
the latest update below covers detached Spec 0–4 projections. A group field must
be present in the rendered layout (hidden fields are allowed).
These limits do not remove those features from the legacy UI. AV writes remain pending.
BuildTags now shares hierarchy/counts, original natural/pinyin comparisons and
root-level limit behavior. Native tags flush only their owner's main queue and
return the limit flag for the future handler, without global UI notifications.

The private scoped performTx audit now admits seventy-two operation families (including three AV configuration, three view-settings, the cell-editing, four option-management, three field-structure, twelve field-settings, four ordering/copy, three row-operation and twenty-six view/layout/group-management and the database-name family): the
previous five plus append/appendInsert/prependInsert/move/delete/insert/update. Actual original receivers
use explicit lookup/IAL callbacks and preserve operation backfill. Unreviewed
AV receivers remain blocked. Update requires reference cache/dynamic anchor reads,
reference tasks, removed-node AV effects, AV upsert and unfold/insert notification
bindings before begin; scoped bindings must stage candidates, not publish or save
early. Original legacy callbacks keep existing behavior. Same-ID source-backed
tests cover plain, removed-child and two folded-heading branches plus failures;
this is not a native source-writing update handler. Insert requires explicit assets, reference
tasks, AV registry and AV binding services before begin. Compatibility binds
original implementations; scoped tests cover parent/previous/next insertion and
dependency failures against real isolated source, not a completed native AV/asset
write adapter. Large scoped insert batches remain closed. Delete requires explicit registry,
reference-task and AV effect bindings before begin; these must stage owned effects,
not fall back to globals or silently skip AV work. Compatibility retains original
callbacks. Missing-after-move and list-placeholder/IAL rules are preserved;
scoped stage failures propagate. No portable AV-delete adapter/commit is implied.
Move requires explicit history
and reference-task services; compatibility retains original history/async tasks,
while scoped callers must stage owned effects and supply the later commit binding.
Cross-document candidates, source/target failures, folded headings and cycles are
tested. The private model seam is not yet a portable move handler. Caller-owned candidate
finish is not authoritative source persistence; no new public handler is enabled.
Tests cover same-ID A/B isolation, unchanged source, failure/cancellation, original
SQL facades and detached transaction candidates, not all original UI branches.

**R02 is still in progress, not completed multi-workspace migration.** Full
AppConf/path ownership, original SQL/treenode query and transaction dependencies,
asset caches, contextual event routing, jobs and remaining watchers still need
contextual ownership and teardown. Do not import model into
the new Server/CLI or invoke
multiple legacy runtimes with different global paths. `StartLegacyRuntime` is
idempotent during its process lifetime, propagates cache-open errors, and refuses
restart after `CloseLegacyRuntime` joins its worker and closes its caches.
Source atomicity and synchronous transaction receipts remain the service layer's
responsibility, not the queue's admission acknowledgement.

Targeted checks from this module:

```sh
go test -race -tags fts5 ./internal/indexdb ./internal/ialcache ./internal/nativeparser ./internal/txqueue ./internal/workspace
go vet -tags fts5 ./internal/indexdb ./internal/ialcache ./internal/nativeparser ./internal/txqueue ./internal/workspace
go test -race -count=20 ./internal/eventscope ./internal/taskgroup ./internal/pollwatch/...
go test -race -tags fts5 ./conf ./internal/workspace ./internal/indexdb
go test -race -tags fts5 -vet=off ./cache ./sql ./conf ./model ./task ./job ./util ./api -run 'Test(AppConf|Original|Configuration|BackgroundLoops|Legacy|DefaultEditors|Single|CheckSingleNotebookMode|EnsureSingleNotebookAndDoc|SortBoxesForElection|SortDocsForElection|InjectSingleModeFields|ResolveSingleModeIDs)' -count=1
```

The last command disables vet only because the original model/legacy code has
pre-existing Go 1.25 nonconstant logging-format diagnostics; it is not a clean
whole-legacy vet result. Queue tests cover independent owners, FIFO/backpressure,
cancellation, drain/close retry, panic reporting and concurrent close/submit.
The model lifecycle test runs the real original executor on empty synthetic
transactions, without a source or DB; it does not prove original rich-text
business isolation. The workspace tests retain the existing synthetic A/B source
and lease/resource coverage.

The original-schema tests need FTS5; the build script now uses `-tags fts5` for
both Server and CLI. A build without FTS5 can still use existing byte-span reads,
but requesting the native schema port fails explicitly, not with a reduced
replacement. Tests cover original file-index first initialization/reopen (capture
file existence **before** eager connection validation), native schema rollback,
same-ID A/B isolation, cache close and parser-option isolation. All runtime checks
use synthetic sources and isolated HOME, never a personal workspace.

## Build locally

From this module (Go 1.25.4, CGO required for both server and CLI SQLite):

```sh
go build -tags fts5 -o ./out/wiki-kernel ./cmd/wiki-kernel
go build -tags fts5 -o ./out/wiki-cli ./cmd/wiki-cli
go build -tags fts5 -o ./out/wiki-compat ./cmd/wiki-compat
go test -race ./internal/workspace ./internal/wikistore ./internal/sourcetx ./internal/docast ./internal/wikiops ./internal/handlers/... ./util
```

Use a temporary workspace for server checks. Do not point this development
version at personal legacy workspaces before the migration stage is verified.

```sh
./out/wiki-kernel --version
./out/wiki-kernel --help
./out/wiki-cli --version --json
./out/wiki-cli --help --json
```

The CLI supports a **portable-source document/block read preview**:

```sh
./out/wiki-cli read --root /path/to/inactive-wiki --store STORE_ID \
  --page PAGE_ID --box BOX_ID --doc DOC_ID --json
# Add --block BLOCK_ID to return one block's original AST.
```

All identities are explicit strings; `--key=value` is supported. Success exits 0,
invalid arguments/read failure exit 2, output failure exits 1. JSON output is one
envelope on stdout; ordinary output is the original AST, not Markdown. Unsupported
commands fail, never proxy to a server. Help/version do not open any source. No CLI
dependency imports legacy/model/util or their initialization workers.

S04 source/session locking and journal admission are implemented: a logical read
holds a shared source lock, rejects unfinished recovery journals, and does not
open/create persistent page.db. First access provisions `.local` runtime-only
locks/directories; help/version remain zero-write. An unprovisioned read-only
source fails closed instead of reading without a lock. All writers must obey the
same protocol; it cannot make arbitrary Git/rsync/direct file edits atomic.

Use `wiki-cli --help` for the native command list. The
[shared operation contract](internal/wikiops/README.md) defines JSON schemas,
dry-run, receipts and limits. `page`, `query`, `content` and `edit` use the
same business services as the authenticated server. `status` inspects local
locks, pending recovery and index freshness. `recover` supports a read-only
preview with `dryRun:true` and explicit application before normal page-tree
admission. These commands use `--root PATH --request -` with stdin JSON.
Local reads can run while the editor holds the writer. Save and release the
active WikiMan session before CLI writes, then resume to read the new source.

## Portable Server

```sh
./out/wiki-kernel serve --root '/path/to/inactive wiki' --store STORE_ID \
  --token-file '/private/path/to/mode-0600-token' --listen 127.0.0.1:0
```

Provision a private token file (32–256 bytes, no whitespace/comma) outside source
data. Never put a real token in argv, a URL, Git or documentation. The listener
accepts loopback IPs only, defaults to an ephemeral port, and prints one ready
JSON address. Authenticated routes are POST `/v1/pages`, `/v1/content`,
`/v1/status`, `/v1/recover`, GET `/v1/resource` and `/v1/events` WebSocket. All source operations
carry store/page identity (store-wide page listing/index/recovery use storeId).
There is no implicit current workspace, HTTP root-path registration, static GUI
bundle serving, cloud/daemon startup or silent recovery. A pending startup source
requires explicit wiki-cli recover first. SIGINT/SIGTERM closes/joins WebSockets,
drains HTTP requests and contexts, and releases the persistent writer session.

The ready record also includes `sessionId`. Every data HTTP request must carry
`X-Wiki-Session: <sessionId>` in addition to Bearer authentication. Browser event
connections use protocols `wiki.v1`, `wiki.auth.<token>`, and
`wiki.session.<sessionId>` (only `wiki.v1` is echoed). Never log these values.
POST `/v1/session` takes `{protocolVersion:1,storeId,action,sessionId}` where action
is `status`, `release`, or `resume`; status does not require a sessionId. Release
blocks new requests, closes source sockets, drains activity and releases the
writer without exiting the Server. A failed drain remains `releasing`, not
released. Resume requires completed release, revalidates/claims the bound source
and rotates sessionId; old requests then fail with `stale_session`. While another
CLI owns the writer, resume fails rather than stealing it. Only the process host
can bind a source; the control endpoint cannot register a filesystem root.
The [S06 native host notes](../../../src-tauri/src/wiki/shared/README.md) describe
the opt-in Rust bridge and the still-pending existing GUI conversion.

S07 first batch adds host-only authenticated POST `/v1/editor` open/close requests
issuing/revoking single-page browser capabilities, not root Bearer credentials.
Scope/HTTP/WS authorization, queues and remaining editor limitations are in the
[wiki-app session contract](../wiki-app/src/portable/README.md). This does not
serve/enable the legacy rich-text editor on the portable Server yet.

S07 second batch adds native `document.render` / `document.transactions` to the
same CLI and HTTP content handler, with Protyle BlockDOM conversion and atomic
CAS/journal writes. The session contract documents supported operations, actual
Protyle hooks/factory, verified iframe channels and still-pending controlled boot
and UI acceptance. Browser capabilities remain page-scoped; unsupported editor
tools never fall back to a legacy workspace.

Optional `--app-dir TRUSTED_APP_RESOURCES` serves the controlled portable editor
entry at `/stage/build/portable/` and bundled runtime/appearance assets. It must be
disjoint from the source root; symlinks/hidden files/directory listings/legacy
routes and nonmatching Host are rejected. The trusted editor loading icon has
an exact `/stage/loading-pure.svg` exception, not general stage access. No root token is included in the
browser URL. Build the wiki-app first and follow its session contract for the
verified parent handshake. The ready record includes editorAssets, and the
Rust/React owner now mounts this controlled entry when assets are available.
Basic Protyle/host lifecycle is tested in Chromium; complete feature parity and
physical macOS webview/IME acceptance remain pending.
The portable editor now uses existing asset.put/resource handlers for bounded,
create-only uploads and capability-authenticated attachment downloads; native
.docj export is a drained raw read, not a new conversion endpoint. Preview media
CSP allows owned blobs only. `document.export` now returns bounded, self-contained
HTML under one read admission for native body plus page assets; it uses the same
CLI/Server service and no network/conversion daemon. See the session contract
for sanitization/limits and reading-projection limitations. Word DOCX export now
uses the same captured source/assets with Lute's semantic DOCX projection and a
trusted optional Pandoc converter. macOS PDF now uses the independent wiki-pdf
helper and the same scoped captured HTML/assets, without a bundled browser.

document.export format:docx returns base64 data/mediaType/hash (max 16 MiB DOCX).
CLI: add --pandoc ABSOLUTE_EXECUTABLE_OR_ZIP to the content invocation. Server:
use the same flag or the host architecture archive under --app-dir/pandoc. The
converter is lazy; absent tools never block source reads/editor startup. No
downloads or legacy configuration are used. Fixed --sandbox/HTML/DOCX arguments,
private temporary HOME outside source, bounded archive extraction/output and
20-second conversion deadline; --sandbox is not an OS security boundary for an
arbitrary executable. Only trusted operators configure paths, never page requests.
Exports do not write source or advance editor CAS. Office files are reading
copies, not complete native/attachment backups; see the portable session contract.

document.export format:pdf returns base64 data/mediaType/hash. On macOS the
CLI/Server lazily resolve sibling wiki-pdf, or trusted --pdf-helper override;
non-macOS/missing tools fail only that export. The core invokes fixed
--html-stdin, no source/output path, using its private temporary directory and
the same deadline/output protections. System WebKit uses nonpersistent data,
private blocking rules, disabled scripts and no file/network loads; asynchronous
silent printing provides A4 pagination, with an 18-second helper timeout and
200-page/16 MiB output bounds. No external engine, new runtime library or daemon.
scripts/wiki-kernel.sh builds the fourth helper on macOS; package resources,
presence checks and architecture checks include it. Intel cross-compilation is
not Intel runtime acceptance; full desktop/WebKit/IME acceptance remains pending.

R04 original editor operations run through the shared edit service and one
durable source transaction. Use `wiki-cli edit --root PATH --request -` or host
`POST /v1/edit`. Scoped Protyle `document.transactions` and `document.history`
use the same service; undo and redo resolve journal images in the kernel.
See the [edit contract](internal/wikiops/README.md#original-edit-transactions-r04).

Old `--workspace/--wd/--port/--fixedPort` invocations execute the sibling
`wiki-compat` helper on macOS/Unix, preserving the process PID and old GUI kill
semantics. This helper alone loads legacy. Build scripts/package verification
include all three Go executables plus macOS wiki-pdf; help/version on the new kernel are zero-write.
Portable mode does not yet make the old wiki-app use scoped requests; that work
and native host integration are intentionally S06–S08. No DMG/Intel validation is
implied by the local S05 acceptance.

The portable [source-layout and page-tree reader](internal/wikistore/README.md)
is implemented independently of the legacy runtime. It has schemas, stable
scanning, tree validation, immutable snapshots and explicit watcher lifetime;
it is not yet connected to the live WikiMan. The first document/block read
service is shared by CLI and a tested HTTP adapter. Page operations additionally
have a Bearer-authenticated `/v1/pages` adapter using the exact same service and
error/receipt schema; the new server mounts these adapters, not the legacy server.
The older `New(manager)` read adapter remains an unmounted compatibility test
prototype; it is not an unauthenticated route of the portable listener.
S03 core isolation is complete: each workspace opens real configuration readers,
a bounded AST cache/parser and a private in-memory SQLite block/reference index.
These are not wrappers around legacy global databases. JSON reads also return
the scoped notebook configuration; ordinary output remains only the original AST.
The CLI exits after draining and closing its contexts; it never keeps a daemon.

Context lifecycle and test coverage are described in the
[workspace implementation notes](internal/workspace/README.md).
The [S04 source transaction contract](internal/sourcetx/README.md) documents CAS,
staging/journal/receipt retention, recovery, index publication and test limits.

R02 computed-field update: original template evaluation/order and relation/rollup
assembly now share avrender with actual legacy SQL facades. Native Spec 4 rendering
supports these fields across table/gallery/kanban and their groups, using confined
owner reads, request-local AV caches and explicit template SQL/stat/parser/clock
services. Template SQL reads the current derived main index (no implicit rebuild),
allows a single SELECT and bounds returned rows. Output is capped at 1 MiB, not a
general untrusted-template sandbox. Field dependencies now use the bounded resolver described below instead of whole
view reentry. Duration localization, Spec 0–4 read projections and runtime dynamic
field access are covered by the subsequent updates below. Ambiguous/future formats
still fail; opaque whole-map functions remain conservatively materialized.

The scoped original transaction seam now admits seventy-two families; the original configuration batch added
updateAttrViewColTemplate/updateAttrViewColRelation/updateAttrViewColRollup.
Subsequent audited settings/cell/options/field families are detailed below.
All AV read/stage/group/relation-index/reload/clock/ID dependencies are explicit;
scoped callbacks must stage owned effects, not write source or publish midway.
Candidate/failure tests are NOT an authoritative source adapter or R04
commit/history/journal/receipt completion. R02 remains in progress.

R02 duration/compatibility/dependency update: the original relative-time thresholds
are shared through conf.RelativeMagnitudes. Business.Duration explicitly snapshots
owner language catalogs; AV field/group/rollup durations never use device globals.
Spec 0–4 read adaptation shares the original UpgradeInMemory rules on detached
candidates only. Raw source reads retain the old version/unknown extensions; no
source save or authoritative ID generation occurs, and ambiguous/future data fails.

FieldResolver follows AV/field/row dependencies, reusing original value/template/
rollup algorithms without rendering an entire destination view. Valid self-links
and 12-view chains are covered; cycles include an identity path. Runtime hooks in
the original Go template evaluator resolve field/ID/raw selectors, dynamic index/get,
map aliases, lexical scopes and named templates only when accessed. Native execution
no longer uses the conservative FieldDependencies inspection utility. Untaken if
branches and short-circuited and/or operands do not trigger dependency reads.
Whole-map output/iteration materializes all members, including the root ID maps;
a real self-cycle fails rather than exposing unresolved formula source. Opaque
functions receiving a map are conservatively materialized unless the pinned
original reader is explicitly adapted (see below); exact member selection inside
other functions remains pending, not a permanent language reduction.
The eager legacy evaluator and original typed model/functions remain unchanged.
Map aliases retain identity across refresh, duplicate-name precedence follows
original assignment order, and full model refresh is charged to the operation
work budget. Memoization and depth/work limits are operation-owned. Shared Calc clears stale results before each
calculation so empty groups cannot inherit parent duration results. Formal source
commit and other R02 gaps remain pending; this is not a new public handler.

R02 map-reader update: pinned original Sprig dig/pluck/pick/omit/keys/values
now prepare only the members they read, then delegate to the original function.
Get/hasKey/default/empty also verify function identity; same-name/same-signature
caller overrides remain opaque and keep their original handlers. Keys enumerate
names without reading template values. Optional relation/rollup display-name
membership is established explicitly; omit does not compute excluded names.
The original unsorted/duplicate key policy, pluck order, defaults, nil behavior,
error reasons and return types remain intact. Legacy eager rendering is unchanged.

Read results may contain nested registered maps. Output/opaque consumers walk
transparent maps/slices/arrays with identity-cycle checks, cancellation, cumulative
work charging and a 256-level container-depth limit before exposing values.
Selection can still pass aliases through these containers without reading them.
This is not a general untrusted-template sandbox. Canonical mutation functions are covered by the next update; other unadapted/
custom function internals remain follow-ups, not approved language removals. Context two-owner layout/group and no-partial-result cycle tests cover
this read-only seam; source commit/history/journal/receipt remain R04.

R02 local-map mutation update (2026-10-23): pinned original set/unset and
merge/mustMerge/mergeOverwrite/mustMergeOverwrite are explicitly adapted.
Per-evaluation map edits (including deletion tombstones and equal-value writes)
survive model refresh. Namespace maps retain identity even when the root entry is
replaced or removed; captured old aliases still work. Display-name, ID and raw
map entries remain independent, as in the original model.

Merges prepare participating members rather than every destination field. The
original Sprig/Mergo code still owns source order, empty/nil/slice behavior, deep
merging, pointer rebinding and error policy. Nonempty scalar destinations can
ignore unused source values, and literal overwrites need not evaluate the old
formula. Same-map merges remain no-ops for dependency scheduling. Shape cycles,
depth/work overflow and cancellation fail explicitly before unbounded merging;
adapter failures are not swallowed by non-must variants. Original function
identity is verified; caller overrides remain opaque, not silently replaced.

Raw values are private per-evaluation deep copies, preserving Go flags and
nil/empty distinctions rather than JSON-cloning them. Name/raw-ID aliases share
these copies until an original merge explicitly rebinds an entry. Local raw edits
do not poison another field's resolver cache or source. Legacy eager rendering
keeps its original raw identity; pointers themselves are not stable output data.
This is still not a generic untrusted-function sandbox. Unknown function
dependency/mutation tracking, other R02 callers and R04 authoritative source
commit/history/journal/receipt remain pending. No new public write handler exists.

R02 original view-settings update (2026-10-23): setAttrViewFilters,
setAttrViewSorts and setAttrViewPageSize now share the original decode/conversion
algorithm between legacy wrappers and scoped receivers. That batch brought the audit to eighteen
families. Scoped AV read/stage and transaction lookup/load select the owner's
block IAL view, including a tree already staged in this transaction. Missing
blocks retain original current/default/first-view fallbacks; known source load
failures, invalid trees and cancellation fail instead of editing a fallback view.
Consecutive operations use the owner's operation-local AV candidate. Unbound
read/stage, malformed outer inputs and unaudited batch operations fail before
begin. Rule decoding/stage/finish failures never produce a successful finish.
Original nil/empty rules and legal float-to-int truncation remain intact;
nonfinite/out-of-range sizes fail explicitly. Same-ID owner source/fallback,
mixed setAttrs/settings, failure/cancel/lock tests and ten race rounds pass with
unchanged authoritative AV/docj bytes. Original UI/API regression and thirteen
editor/package/export integrations also pass. These are candidate seams, not a
native source commit handler; R04 commit/history/journal/receipt and other R02
callers remain pending. No frontend, CLI help/schema or packaging paths changed.

R02 original cell-editing update (2026-10-23): that batch brought the audit to nineteen
families, adding updateAttrViewCell. Original single/batch entry points and
receivers share the value-edit and reverse-relation core. Explicit services own
AV read/stage/groups, block lookup/text/names/attributes, related refresh, time,
IDs and option colors. Missing dependencies fail before begin, and scoped code
cannot opt into legacy best-effort error handling. Field normalization, empty
option early return, original timestamps, relation ordering/change modes and
self-relation behavior are retained. Binding/unbinding/rebinding and static
anchor attributes use the original attribute/name algorithms. The old/new
updateAttrs notification payload is shared; scoped sinks stage owner effects,
not global broadcasts. Related sources are filtered and flushed in the original
read/group/stage/reload order, with errors and cancellation propagated.

Two same-ID owners use actual AV/docj/relations fixtures; candidate grouping uses
the original GenerateGroups/Table. Field/binding/relation/notification rules,
reverse creation, batch itemID/rowID compatibility, consecutive edits, admission,
malformed source/input and failure/cancellation cases pass ten race rounds with
unchanged authoritative source bytes. The original GUI/API saved edited cells,
reverse relations and batches and rendered their new results; thirteen editor,
export and packaging integrations pass. These migration seams/test adapters do
not implement R04 native source commit/history/journal/receipt, all Context WS
routes or the remaining R02 business graph. No frontend, CLI help/schema or
packaging paths changed. R02 remains in progress.

R02 option-management update (2026-10-23): that batch brought the scoped audit to twenty-three
families. updateAttrViewColOptions/removeAttrViewColOption/updateAttrViewColOption/
setAttrViewColOptionDesc reuse the original algorithms through explicit read/stage
and (except description-only edits) groups services. Original wrappers preserve
calls without an Action field. Keep stable sorting, new/duplicate/empty input
rules, existing-target merge colors/descriptions, row/filter propagation and raw
description whitespace. Do not add unrelated AV refresh or block/broadcast effects.

Deleting a filter while iterating its mutable slice used to skip its neighbor.
The shared algorithm now visits a slice snapshot, retaining original matching and
empty/nonempty-operator rules while processing each original condition once.
Owned candidates keep consecutive edits but reset RenderedViewables per read,
as original ParseAttributeView did; stale projections cannot mask the next edit.
Input JSON conversion runs once; malformed inputs/source, missing services and
cancellation fail explicitly. Description edits need no groups service.

Two same-ID owners exercise select/multiselect, actual GenerateGroups/Table,
metadata/row/filter isolation, mixed cell edits and failure/rollback cases with
unchanged authoritative source bytes. Six new tests pass ten race rounds.
Original API saving and rendering across three layouts/two field types, the
legacy GUI regression and thirteen editor/export/package integrations pass.
These remain candidate seams, not R04 authoritative commit/history/journal/receipt
or the complete R02 graph. Frontend, public CLI/schema and packaging paths did
not change; R02 remains in progress.

R02 field-structure update (2026-10-23): that batch brought the scoped audit to twenty-six
families. Original addAttrViewCol/updateAttrViewCol/removeAttrViewCol receivers,
exported add/remove APIs and the internal update wrapper share the original
field algorithms. Explicit owner services provide read/stage, related sources,
groups, relation-index removal and reload effects; addition only needs read/stage.
Missing bindings and malformed add icons fail before begin. Scoped execution
cannot select compatibility's ignored-error policy.

Keep original supported types/no-ops, name/icon/default rules, current-layout
insertion order, previous-field fallback and wrapping. Type changes keep old value
payloads while updating types and invalidating groups/related rollups. Changing
type resets rollup Calc to None; deleting only clears rollup Contents, as before.
Two-way/self-linked deletion preserves or removes the reverse field, using the
same edited candidate for recursive reads. Original rendering memos reset per
operation while owner read/stage retains consecutive edits.

Fix deleted-field group cleanup by matching Group.Field directly, including a
deleted reverse field. Iterative reverse-key removal must not skip an adjacent
live relation. A retained back field is still a live one-way edge. Removing the
last one-way field also removes its registry edge; other references preserve it.
These are explicit bug fixes, not a new UI or value-conversion policy.

Eight new tests exercise two same-ID owners, three layouts, all original field
types, ordering/group/rollup/relation rules and mixed options/cell transactions.
Ten race rounds pass with unchanged source AV/docj/relation bytes and explicit
failure, cancellation, malformed-source and admission checks. The shared test
renderer now uses the actual Table/Gallery/Kanban cores with bound cover/tree
services. Final build, legacy GUI/API and integration evidence lives in the shared
execution plan. Candidate effects are not R04 authoritative source commit,
history/journal/receipt, full Context WS routing or all remaining R02 work.
Frontend, public CLI/schema, resource paths and packaging do not change.

R02 field-settings update (2026-10-23): that batch brought the scoped audit to thirty-eight
families, adding twelve original field-property operations: width, wrap, hidden,
pin, icon, description, number format, column calculation, two date-fill settings
and created/updated time display. All twelve original function bodies match HEAD
after normalizing read/stage/view dependency binding. Original wrappers and
receivers share those algorithms; no UI, extra group/relation refresh or global
notifications are added.

Use the owner's AV candidates and transaction-local source tree to select views,
including staged setAttrs and original fallback. Known source-load errors do not
silently fall back. Metadata settings do not require a view service. Width, pin
and calculations retain table-only behavior; wrapping still aggregates every
field. Number formatting keeps its original operation/key type checks. Icons and
descriptions retain whitespace. Date fill updates only empty values' time flags,
not nonempty dates/ranges; IncludeTime keeps original type/missing-key behavior.

Calculation JSON converts once, retaining typed/null values and encode-before-
no-op behavior on card layouts. Cancellation after encoding or successful callbacks
prevents staging. Nil calculation interfaces now fail admission instead of panic;
malformed primitive inputs, source structures and missing services fail explicitly.
Per-operation rendering memos reset without losing consecutive owner edits.

Six new tests pass ten race rounds: 288 two-owner/layout/property rule combinations,
80 source-view/fallback combinations, mixed candidate edits, date/wrap rules and
JSON/failure/cancellation/admission checks. Source AV/docj/relation bytes remain
unchanged. Final build, legacy GUI/API and editor/export/package evidence is in
the execution plan. These remain candidate seams, not R04 authoritative source
commit/history/journal/receipt or the complete R02 graph. Public CLI/schema,
frontend and resource/distribution paths do not change.

R02 ordering/copy update (2026-10-24): scoped performTx now admits forty-two
families, adding original row/column/key sorting and field duplication. Both
exported sorting wrappers and receivers share the core. Column/key ordering
matches HEAD after normalizing effect bindings. Keep self-drop and missing-key
no-ops, front fallback, missing-row insertion and original KeyIDs repair policy;
unspecified map append order is not silently replaced with a new sort policy.

Cross-group moves use owner-selected source trees, actual rendered/filtered/sorted
neighbor items, original filter/template default rules, reverse relations and
group regeneration. Templates/created/updated groups still prohibit cross-moves.
Default/near-value/filter effects now bind owner clock/IDs and relative-date/ID
parsing timezone, while legacy wrappers retain time.Now/ast.NewNodeID. Existing
cell overwrites preserve IDs/timestamps; only their value payload changes.

Copy metadata, not cells; block/relation fields remain non-copyable. Keep numeric
name suffixes, per-layout adjacency, wrap/hidden/description and table pin/width,
but not column calculations. Deep-copy nested metadata so subsequent candidate
edits cannot change the original options/date configuration. Fix empty-target
single-select moves that previously removed the old option without adopting the
known target option. Both fixes have isolated and original API regressions.

Seven model tests pass ten race rounds with unchanged owner AV/docj/relation
source bytes. Two domain tests cover relative dates, ID timestamps/timezones and
original default rules. Owned callbacks fail explicitly on missing dependencies,
malformed candidates, clone failures and cancellation. Actual three-layout
rendering/filter/sort/template and reverse-relation cores are used in tests, not
no-op placeholders. Final builds, legacy GUI/API and thirteen integrations are
recorded in the execution plan. Candidate adapters are not authoritative R04
source commit/history/journal/receipt or the complete R02 graph. Frontend, public
CLI/schema and resource/distribution paths remain unchanged.

R02 row insertion/removal/rebinding update (2026-10-24): the scoped audit now
admits forty-five families, including 33 of 60 original AV receivers. The three
row receivers and standalone/batch wrappers share the original algorithms with
owned candidate AVs, source trees/attributes, labels, names/existence, duplicate
messages, related refresh and history effects. Reverse iteration preserves batch
order without reversing caller input. Defaults, date filling, grouping, binding,
duplicate rebinding and row-versus-source-block identity retain their rules.
Removal does not delete the source block; detached conversion retains the old
row-only binding change.

The owned history builder snapshots raw mirror documents (unknown extensions
included) and the post-removal AV, deduplicates document paths, copies bytes and
records hashes. Scoped errors/cancellation cannot finish successfully. Legacy
compat still writes/indexes native history. R04 must provide authoritative
transaction publication, unknown-field-preserving AV serialization and the
source/history/journal/receipt adapter; this candidate seam is not that adapter.
Two owners, three layouts, defaults/reverse relations, batch/mixed operations,
failure/admission/cancellation and unchanged source/registry bytes pass ten race
rounds. Original GUI/API checks exercise all three layouts and actual mirror/AV
history files. See the execution plan for final evidence. R02 remains doing,
with 27 AV receivers and other configuration/query/background/event callers
still pending. Public CLI/schema, frontend and resource paths do not change.

R02 view/layout/group management update (2026-10-24): twenty-six original
receivers and their standalone wrappers now share the original algorithms with
owned AV candidates, source trees/mirror registry, attributes/cache/notifications,
tree batches, group rendering/counts/sorting, constructors, language, IDs/time and
reload effects. The scoped audit admits 71 families, including 59 of the original
60 AV receivers; database-wide setAttrViewName and its bound-name refresh remain.
This count is not a completion percentage.

Keep last-view protection, adjacent removal fallback/RetData, per-block selection,
layout configuration reuse, field inheritance, card-setting applicability, kanban
preferred grouping and original group-state/hide-empty behavior. Kanban group
regeneration still clears folding. Copy the original subset of view settings,
with detached nested filters, calculation/group settings and item-order slices.
When separate mirror loads return different objects for one document, resolve
all nodes against its first tree before flushing once, so one mirror cannot
silently lose another mirror's change. Owned source/registry bytes stay unchanged.

Eight model tests pass ten race rounds, covering both owners, three layouts,
all operations, mixed candidates, missing services, failures, cancellation and
independent mirror copies. Final legacy GUI/API checks use actual native AV
mirror blocks, original group/layout APIs, persistence and rendering. See the
execution plan for builds and thirteen integration checks. R02 remains doing;
native write adapters and R04 authoritative source/history/journal/receipt are
still pending, as are other configuration/query/background/event callers.
Public CLI/schema, frontend, resource and distribution paths are unchanged.

R02 database-name update (2026-10-24): scoped performTx admits 72 families,
including all 60 original AV receivers, each requiring its audited owner services.
setAttrViewName still selects the AV through operation.ID, trims whitespace,
replaces newlines and limits names to 512 Unicode characters. Its original entry
shares the owned read/stage, binding lookup/source nodes, other-AV names/untitled
language, tree candidates, IAL cache and updateAttrs notification algorithm.
The current name comes from the candidate, with other names resolved in the same
owner, preserving memberships, original span formatting, body and other attrs.
One tree per document retains all bound-node edits, including independent loads.

Detached/missing bindings retain their skip behavior; known source failures and
cancellation cannot finish successfully. Compat still saves/notifies and refreshes
its buffered IAL name cache. av-names remains derived presentation metadata: the
original JSON renderer removes it from docj. Do not persist it as authority.
Six model tests pass ten race rounds, with unchanged owner source/registry bytes,
original serialization and notification checks, plus final original GUI/API and
thirteen integrations recorded in the execution plan. All-receiver admission is
not completion of other standalone AV writers, the native write adapter, R02 or
R04 authoritative source/history/journal/receipt. Remaining configuration/query/
background/event callers and legacy binding helpers still need audit. Public
CLI/schema, frontend and resource paths remain unchanged.

R02 standalone detached batch append (2026-10-24): the original
AppendAttributeViewDetachedBlocksWithValues entry and private Context entry share
one algorithm with explicit owner read/stage, groups, reload, clock, ID and value
clone services. Missing services/cancellation fail without scoped global fallback.
The exported signature/API and legacy parse/save/group/reload behavior remain.
Preserve source field types, detached flags, primary binding removal with content/
icon retained, one batch timestamp, all-view row order and old/new option colors.
Empty batches, empty rows and sparse rows keep their original semantics.

Clone every input cell separately so shared caller values cannot overwrite other
rows. Preflight all input keys/payloads and reject empty or colliding generated IDs.
Five tests pass ten race rounds, including two owners with identical IDs, all
three layouts, real groups, sequential/mixed candidates, invalid last rows,
missing services, failures/cancellation and unchanged source/registry bytes.
Final original GUI/API and thirteen integration checks are recorded in the plan.
This independent API adds no performTx action: totals remain 72 families and
60/60 AV receivers. Staging/reload errors do not imply R04 atomic rollback.
Other standalone writers/helpers, native write adapters, configuration/query/
background/event callers and R04 authoritative source/history/journal/receipt
remain pending. R02 stays doing; public CLI/schema, UI and resource paths unchanged.

R02 standalone database duplication (2026-10-24): DuplicateDatabaseBlock and its
original API now share an owned algorithm with the private Context entry.
Explicit services supply the source object/raw JSON, occupied-ID checks, new IDs,
clock, mirror registration, raw candidate staging and bound-block refresh.
Scoped calls reject missing services/cancellation without global fallback.
No performTx action is added: totals remain 72 families and 60/60 AV receivers.

Preserve the original whole-JSON ID replacement (including self references and
text), nonempty-name timestamp suffix, empty-name behavior, row/field/view/group
identities and order, two-way relation disconnection and future mirror-block ID.
Existing one-way metadata stays intact; duplication does not insert a body block.
Bound rows still use BlockID, skip detached/vanished records, retain/deduplicate
custom-avs and use original name/attrs/cache/notification rules. Own one tree per
document, flush once and include initially missing memberships in the save set.
Remove transient av-names before tree staging. Other import callers of the old
updateBoundBlockAvsAttribute helper remain pending.

Six model tests pass ten race rounds, covering both owners, three layouts,
independent document loads, sequential candidates/memberships, failures,
cancellation, dependency/ID/source gates and unchanged source/registry bytes.
See the execution plan for final original GUI/API, builds and integration results.
A staged registry/AV/binding failure is not an atomic source rollback. Legacy
mirror helpers keep their existing logged-error contract. Native write adapters,
unknown-extension preservation, history/journal/receipt remain R04 work; other
R02 configuration/query/background/event callers are still pending.
Public CLI/schema, frontend and resource paths are unchanged.

R02 adding-default preview (2026-10-24): GetAttrViewAddingBlockDefaultValues and
its original API share the existing attributeViewDefaultValues calculator with
the private Context entry. Explicit services supply owned AV reads, an isolated
copy, actual filtered/sorted nearby items, template dependencies, clock and IDs.
There are no save/reverse-relation/registry/notification capabilities. Scoped
errors/cancellation return no partial values and never use global fallback;
the legacy wrapper retains logged errors and an empty map/API null result.

Preserve requested/current/first view fallback, early return before group lookup
when no rules exist, creation mode, group options and existing-row fallback,
sorted nearby items, absent previous IDs, date precision/autofill, template/
relation rules and asset early return. Clear primary Content before returning.
Clone without regenerating source identities or Spec and reset runtime render
memo so rendering or existing-option defaults cannot modify the owner's AV.
Outputs also have independent payloads; do not use AttributeView.Clone here,
because it regenerates identities.

Five tests pass ten race rounds across both owners and three layouts, covering
actual groups/sorts/template dependencies, primary suppression, dates, relation
preview without reverse writes, asset behavior, sequential candidates, returned
value isolation, malformed inputs, dependency gates, failures/cancellation and
unchanged sources. Final original GUI/API and integrations are recorded in the
execution plan. No performTx action is added: still 72 families and 60/60 AV
receivers. R02 remains doing; legacy binding/import helpers and other callers
remain pending, with authoritative writes/history/journal/receipt still R04.
Public CLI/schema, frontend and resource paths are unchanged.

R02 bound-node transaction chains (2026-10-24): getAttrViewBoundNodes now
returns errors and shares an explicit owner collector with ordinary insert,
database deletion/update-removal and the large-paste insertion path. Services
supply AV reads/staging/groups, block existence/tree loads, mirror decisions/
removal, names, attrs/cache/notifications, tree staging and refresh. Scoped
calls never fall back to globals; failures/cancellation cannot finish.

Preserve primary Block.ID rather than row BlockID, detached/vanished skips,
membership order, current/explicit view rules, the pre-removal mirror decision
and original primary-only pruning of deleted source blocks. Own one tree per
document across independent loads. Snapshot old attrs before unlinking members,
refresh the owned IAL cache and stage the final inserted mirror layout even
when it already has a selected view.

The >128-operation paste path now uses the existing transaction ports for
assets/references/registration/cache/rebinding and propagates TxErr to performTx.
Previously scoped batches were blocked. Only the audited same-previous inserts
plus final delete shape is admitted with complete owner dependencies; other
insert-led oversized shapes/missing adapters stay blocked. No action is added: 72 families, AV60/60.

Seven tests pass ten race rounds across both owners, actual insert/delete/update,
three layouts, same/cross-document and independent loads, 129-operation paste,
dependency/source gates, failures/cancellation and unchanged sources/registries.
Final original GUI/API, builds and thirteen integrations are recorded in the plan.
R02 remains doing. Other import/binding helpers, configuration/query/background/
event callers, native write adapters and legacy finish remain pending. The
file-delete controller is still R05; authoritative source/unknown extensions/
history/journal/receipt are R04. Staged rollback is not an atomic source rollback.
Public CLI/schema, UI and resource paths are unchanged.

R02 imported AV binding post-processing (2026-10-26): ImportSY keeps its
original signature/API and uses an explicit legacy adapter for the shared
mirror-registration, batch binding refresh and relation-projection phase.
The private Context entry supplies owner AV reads, node/existence/name/language,
attrs/cache/notifications, tree staging, mirror and relation services.
Missing dependencies/cancellation cannot fall back to global state or advance
to later phases after failure.

updateBoundBlockAvsAttribute returns errors and shares the extracted
refreshAttributeViewBoundAttributes batch algorithm with database duplication.
Preserve row BlockID lookup, detached/vanished skips, ordered/deduplicated members,
old names/options/language formatting and the original attrs/cache/notification
rules. Use one tree per document across all AVs, flush once and save initially
missing memberships. Remove transient av-names from trees; source AV payloads,
layouts and identities remain untouched and this phase generates no IDs.
The original relation projection retains the last relation destination per AV,
including one registration for duplicate source IDs. Archive trees are not yet
mounted, so their Box may be empty at this mirror-registration stage.

Five new tests and six existing duplicate tests pass ten race rounds, including
both owners, three layouts, shared/independent trees, existing/new members,
sequential candidates, dependency/error/cancellation gates and unchanged sources.
Final original archive imports and thirteen integrations are in the plan.
This is not a native whole-archive importer or a performTx action: still 72
families and AV60/60; R02 remains doing. R04 owns authoritative commit/unknown
extensions/history/journal/receipt and R05 owns archive/controller migration.
Other binding/update/config/query/background/event callers remain pending.
Staged failures do not promise atomic rollback; legacy registry/relation helpers
keep their logged-error contract. Public CLI/schema, UI and resource paths unchanged.

### R02 update AV transaction effects (2026-10-26, still doing)

legacyUpdateAV and the compatibility upsertAvBlockRel wrapper now use the shared
updateAttributeViewWithEnvironment/registerUpdatedAttributeViews core. Native
transactions bind private avUpdate services for mirror registration, AV reads,
one-hop source lookup, groups, staging, reload, original attrs effects and trees.
Missing services, invalid identity/source or cancellation cannot use globals.
Native refresh is synchronous candidate work: errors reach performTx before finish;
only the legacy handler retains the original 100ms best-effort background refresh.

Keep subtree mirror registration, ancestor-subtree memberships including siblings,
first-seen deduplication, and exactly one source-relation hop. A first mirror is not
an additional-mirror change; legacy reload still runs when an AV is absent. Keep
exact GetView(ViewID), preserve an explicit custom-sy-av-view and fill only missing
view attributes. This update path does not rebind every database record. Re-stage
the final tree after layout/attrs correction, including an existing explicit view,
so snapshot adapters cannot retain the earlier layout.

Six new tests plus seven existing bound-chain tests pass ten race rounds; actual
update operations cover both owners with identical IDs, all three layouts, ordered
refresh, failures/cancellation, exact-view lookup and unchanged source bytes.
Original desktop/single GUI/API and integration evidence is recorded in the plan.
Still 72 action families and AV60/60 receivers: this adds no public action or CLI.
R02 remains doing; R04 owns authoritative commit, unknown extensions, history,
journal and receipt. Other bindBlockAv0/source-event/config/query/background
callers and the native production write adapter remain pending. Legacy logged IO
errors and delayed work are not promoted to owned, atomic native guarantees.

### R02 shared block AV binding (2026-10-26, still doing)

bindBlockAv0 previously had only the unused bindBlockAv wrapper as a caller;
active cell edits and insert rebinding had already inlined the binding algorithm.
They now share bindBlockAv0WithEnvironment, including bindBlockAv/unbindBlockAv,
the cell binding method and transactionInsertAVBind (also large-paste inserts).
The binding environment projects existing cell/bound owner services: node lookup,
names and original attrs effects. No new transaction action, service slot, source
writer, mirror registration, AV saving or background task is introduced.

Private helpers return errors. Native calls reject missing dependencies, mismatched
node/tree identity and cancellation before/after services, without global fallback.
Already-loaded nodes need no lookup service. Keep ordered/deduplicated binding
members, unbinding's removal rule, original empty-name/last-member behavior,
normalization, static attrs, cache and old/new notifications. Legacy vanished-block
skips and cell-only best-effort attrs remain; direct helpers return attrs errors.

Seven new tests plus seven bound and two original cell tests pass ten race rounds
(16 tests); full original Go, owned-core race tests and pure-package vet pass.
Both owners with the same IDs, sequential candidate helpers, actual mixed insert
and cross-document cell rebind, service failures and post-service cancellation
preserve source bytes. Final GUI/API and integration evidence is in the plan.
The corrective unbind in GetBlockAttributeViewKeys uses the shared legacy adapter;
its SQL/controller chain is not thereby migrated. R02 remains doing (72 action
families, AV60/60). R04 still owns authoritative commit, unknown-field fidelity,
history/journal/receipt; other controllers, source events and legacy finish remain.
Public UI/CLI/schema/resource layout is unchanged.

### R02 block attribute-panel query and repair (2026-10-26, still doing)

GetBlockAttributeViewKeys now delegates to blockAttributeViewKeysWithEnvironment;
the private avBlockKeys Context projection binds synchronization, block attrs,
AV read/clone/render, node resolution, view creation/staging, mirror lookup/scan/
registration, corrective unbind, orphan membership removal and owner language.
The original general getRenderAttributeViewView shares the extracted view-selection
algorithm too. Native errors/cancellation return no partial success or global fallback.

Preserve binding lookup by Block.ID, projection by the distinct row BlockID,
line-number exclusion, original key-ID repair/order and name formatting. Keep
block-selected/current/first-view fallback, empty table creation and changed-view
persistence. Duplicate memberships still produce duplicate results with one cached
source read. Invalid binding correction returns prior results immediately. Only an
empty mirror registry triggers the old full scan, first-seen dedup/registration or
orphan cleanup (including document custom-hidden); existing registry results stay
unchanged. These are write-capable query effects, not a read-only endpoint.

Render detached AV copies to avoid mutating an owner read cache. Compatibility
copies explicitly restore original duration/read/error services; native renderers
bind owner runtime services instead. Eight new tests plus fourteen bound/binding
regressions pass ten race rounds (22 tests, 82.202s), including both owners, three
layouts, correction ordering, source bytes, malformed/dependency failures and late
cancellation. Final original GUI/API and thirteen integrations are in the plan.
No public CLI/schema/UI/resource changes or new performTx actions: still 72 and
AV60/60. R02 remains doing. Query repairs are native candidates until the R04
commit/history/journal/receipt adapter and R06 full UI Context wiring land; no
atomic rollback or production multi-Context endpoint is claimed.

### R02 current AV image query (2026-10-26, still doing)

GetCurrentAttributeViewImages now shares currentAttributeViewImagesWithEnvironment
with the private avImages Context entry. Read/clone, further rollup collections,
raw table rendering, filtering and sorting use explicit owner services. Copies
isolate source view/value mutation; missing dependencies, malformed payloads,
service errors and post-service cancellation return errors without partial images.

The image query and original getAttrViewTable exporter share the extracted pure
projectAttributeViewTable conversion. Keep table visibility, ID-only gallery/
kanban field projection (which originally drops card Hidden flags), explicit-view
fallback and exact current-view lookup, full unpaginated row order, asset order,
duplicate image paths, non-image exclusion and nil for an empty result.

Original sql.GetFurtherCollections also delegates to the new avrender shared
FurtherCollections algorithm. Its owned Read/Render services retain cache reuse,
first-view rendering and the original template/cross-AV auto-field conditions;
legacy missing-source skips stay explicit, while native errors/cancellation surface.
Five new model tests plus eight block-key tests pass ten race rounds (13 tests,
48.891s); two new pure-domain tests also pass ten rounds (2.526s). Full original
Go/core race, pure vet, original GUI/API and integration evidence is in the plan.
No public CLI/schema/UI/resource or action-count changes (72 families, AV60/60).
R02 remains doing; R04 owns authoritative writes/repair commit and R06 owns full
production multi-Context UI wiring. Other source events/controllers remain pending.

### R02 AV identity conversion queries (2026-10-26, still doing)

GetAttributeViewItemIDs/GetAttributeViewBoundBlockIDs now share the pure
avquery.IdentityMap through the typed AttributeView.IdentityMap bridge. Context
model callers project existing avCell read/ctx dependencies only; no new slot or
source writer is introduced. Workspace Lease.NativeAttributeViewItemIDs and
NativeAttributeViewBoundBlockIDs use the same algorithm against their owned source.

Keep Block.ID distinct from row BlockID, prefill every requested key with an empty
string, collapse duplicate request keys, preserve last-source-match wins, and keep
empty/detached/dangling identities without registry existence filtering. Empty
requests still read/validate the source and return an object. Legacy read errors
retain the prefilled object; native failures/cancellation return nil plus error.
No rendering, view validation, language dependency, ID generation or source save
is added. Native identity reads project the stored values without upgrading old
sources; the legacy parser retains its existing compatibility policy.

Six tests (three model, two pure, one real Lease) pass ten race rounds across both
owners with identical IDs, repeated candidates, malformed/dependency failures,
late cancellation and unchanged source bytes. Native reads also reject invalid
paths, mismatched source identity, broken JSON and missing files. Full Go/core
race, pure vet, original GUI/API and integration evidence is in the plan.
Still 72 action families and AV60/60; R02 remains doing. R04 source migration/
authoritative commit and R06 full UI Context wiring remain pending. No public
CLI/schema/UI/resource layout changes.

### R02 original full/primary/rules read callers (2026-10-26, still doing)

GetAttributeView, GetAttributeViewPrimaryKeyValues and GetAttributeViewFilterSort
now share explicit attributeViewReadEnvironment services with private Context
entries. Project existing avBlockKeys read/clone/wait/mirror/node/language services
and the owner block registry; no new slot, action or write handler is required.
The original getAttrViewViewByBlockID also shares the extracted selection helper.

Keep the full typed object, primary name/empty-name language, mirror IDs, original
Value.String(true), case-insensitive matching, detached retention, missing-bound
filtering, updated order and original page/default/empty behavior. Keep block-selected
view/current/first fallback, missing-block fallback and empty rules as arrays.
Known native source failures cannot silently choose another view. Legacy primary
errors retain original partial metadata; native errors clear the complete result.

Copy source objects before returning full data, filtering primary values or exposing
rule pointers, so callers cannot mutate owner caches. Reads do not select-and-save
views, repair source or publish notifications. Full-object reads need only the
minimal read services and preserve unrelated typed metadata. Existing raw Lease
read projections keep their unknown-field fidelity and are not replaced here.
Six new model tests plus eight block-key tests pass ten race rounds (14 tests,
49.408s); complete original/core race and pure vet pass. Original three-layout
GUI/API and thirteen integrations are recorded in the plan. R02 stays doing;
72 action families/AV60/60 remain unchanged. R04 authoritative writing and R06
production multi-Context UI routing, plus other controllers/events, remain pending.

### R02 tag mutations and AV block text propagation (2026-10-26, still doing)

RemoveTag/RenameTag now share tagMutationEnvironment with private Context entries.
The original SQL span query, owner trees, staged writes, flush/reload, progress,
cleanup and AV propagation are explicit dependencies. Empty removal and normalized
same-name renames remain no-ops; rename validation/trim rules stay unchanged.
Removal matches exact document/inline labels; renaming also covers child labels.
Native missing dependencies, malformed sources, service failures and late cancellation
return errors and clear owner progress without falling back to global services.

The extracted attributeViewTextEnvironment retains original non-document ancestor
propagation, first bound match, name/static/reference/untitled/icon and HTML rules.
Read the latest staged owner candidate and clone for each block: a related-field
refresh between blocks must survive subsequent text updates. Only changed primary
text/icon stages, reloads and refreshes related AVs. Legacy adapters preserve their
SQL/related-refresh logging contracts and best-effort AV saving. Other legacy
search-replace/anchor producers sharing this helper are not fully Context migrated;
native anchor writes with AV bindings still fail closed until their plan can carry
AV candidates. Native effects remain candidates pending R04 authoritative commit.

Seven new model tests pass ten race rounds (36.156s), including real owner tag SQL
across two workspaces, cross-document edits, failure/cleanup, cached source isolation
and related-candidate retention. Complete original/core race, pure vet and no-fts5
checks pass. Final original desktop/single, three-layout tag APIs and integrations
are recorded in the execution plan. No public schema, CLI, UI or resource layout
changes. Still 72 action families and AV60/60; R02 remains doing. Other settings,
queries/controllers/events/background work, R04 commit and R06 UI routing remain.

### R02 tag navigation and source refresh events (2026-10-26, still doing)

The retained labelBlocksByKeyword helper now shares tagquery.Navigate with private
Context services and Lease.NativeTagBlocks. One native admission owns both span SQL
and block projection. Preserve keyword/limit, descending deduplicated reader IDs,
span order, duplicate hits, nil entries for vanished blocks and original block UI
projection; failures/cancellation return no partial map. This helper currently has
no live caller. The original Tag.ts UI opens full-text search with #label#, whose
query/projection was already migrated; no replacement UI or public route was added.

ReloadProtyle and refreshRefCount now share sourceEventEnvironment. Owner services
provide trees, child definitions, references/blocktree, queue flush, anchor callbacks,
scheduling and notification candidates. Keep original per-definition seven-round
anchor propagation, embedded-root deduplication followed by the requested root,
200ms delay and distinct document/sub-block counts. Preserve the document ID as the
legacy task deduplication argument: different roots must not collapse into one task.
Guard every anchor callback and delayed execution with the owning ctx, including
callbacks returning nil after cancellation. Native AV refresh is rebound to the
same transaction; it cannot call the supplied legacy/global UpdateAVs callback.

Ten new tests (seven model, two tagquery, one real Lease) pass ten race rounds,
including real owner reference/tag SQL in two workspaces, tag mutation composed
with reload/anchor/AV candidates, expired queued callbacks and the actual legacy
task deduplication behavior. Complete original/core race and pure vet/no-fts5 checks
pass. GUI/API evidence and thirteen integrations are recorded in the execution plan.
Still 72 action families/AV60/60; R02 remains doing. Search-replace, attribute-save
background producers and other events remain pending; R04 owns authoritative commit
and R06 owns full multi-Context UI routing. The existing Lease anchor plan still
rejects AV-bound writes until it can carry their candidates. No CLI/schema/UI/resource
layout changes, and no promise of legacy concurrent source-write atomicity.
The legacy multi-block parent/child/reference binding fixture also exposed SQL/source
attribute mismatch before source-event assertions, reproduced with the pre-change
binary. This remains a separate TODO. Accepted source-event probes seed coherent
source attributes with the original batch API and do not certify that binding
wrapper or unowned attribute-save background work.

### R02 block attributes and owned reference refresh (2026-10-26, still doing)

SetBlockAttrs, BatchSetBlockAttrs, setNodeAttrs and setNodeAttrsWithTx now use the
same blockAttributeEnvironment and original attribute normalization/event payload.
Private native entries require explicit owner load/stage/cache/sync/notify/tag and
language services. Native inputs are copied; keep lowercase keys, trim/trailing
comma/tag normalization, empty removal and legacy caller-map behavior. Batch
entries apply in request order and stage each root once; duplicate entries retain
per-entry events, and inconsistent native candidates for one root are rejected.
Batch and transaction-local changes still do not start dynamic anchor refreshes.

The former unowned attribute goroutine now uses the owner's admitted/joined task
service (the explicit legacy task group for compatibility). Queue block/root IDs,
flush and reload the latest owner tree on execution, skip disappeared blocks, then
reuse original anchor/AV algorithms. A per-owner cancellable gate serializes these
refreshes with attribute API load/mutate/stage; native callers cannot fall back to
the legacy gate. Waiting for the old transaction queue happens before the gate.
Internal callers retain their supplied AST semantics; the gate does not rebase or
certify every other transaction/source producer. Job cancellation also reaches
nested native AV text guards, and shutdown joins callbacks until actual return.

Eight new model tests plus six source-event regressions pass ten race rounds
(14 tests, 63.243s), including two real owner reference indexes, request/payload
isolation, batch root/order behavior, fresh-tree reads, cancelled waiters and real
legacy task close/admission. Complete original/core race and pure vet/no-fts5 pass;
accepted API/startup and integration evidence is in the plan. Native effects remain
candidates until R04; full multi-Context UI wiring remains R06. R02 still doing,
72 actions/AV60/60 unchanged. Search-replace, other producers and the legacy nested
binding source/index mismatch are separate remaining tasks, not certified here.

### R02 original find/replace and replacement-all queries (2026-10-26, still doing)

FindReplace now shares its original traversal with explicit findReplaceEnvironment
services for source lookup/load/stage, preimage history/indexing, parser/case policy,
replace-all query, title candidates, progress/cleanup, tag/refcount and AV refresh.
The plain-text case flag and tag-to-reference helper no longer read global settings
or schedule global tasks on the native path. Copy admitted requests and service
query arguments; missing services, bad identities and late cancellation fail closed.
Preserve literal/query-syntax/regex behavior, field flags, original escaped text,
tag conversion, document-title slash handling and replacement carry between nodes.

Native operations use the supplied owner gate. Legacy execution retains its old
RenameDoc queue-drain behavior outside that gate, avoiding a transaction-helper
lock cycle; this does not certify global legacy write isolation. Keep legacy
best-effort rename/AV error contracts, while native owner errors propagate. History
captures each original source once before staging. After reload/anchor propagation,
read current owner nodes again before final AV projection so stale reference ASTs
cannot overwrite the AV text just refreshed by the reload callback.

Add explicit internal SearchAllBlocks/SearchAllNativeBlockView entry points: the
old MaxInt call was rejected by the shared 10,000-row page gate, breaking Replace
All. Ordinary search still enforces that gate; replace-all ignores display paging,
uses the same owner ignore/filter/read-only SQL rules and avoids page-count overflow.
No public all flag, CLI command or route was added. Tests cover two real owner
indexes, preimages, rich text, title/tag rules, callback faults, read-through after
reload and 10,005 results. Final race/API/integration evidence is in the plan.
R02 remains doing (72 actions/AV60/60). Native rename/history/source commit adapters,
other lifecycle/controllers and legacy nested binding consistency still remain;
R04 authoritative commit and R06 full UI routing are not completed here.

### R02 original bookmark rename/remove (2026-10-26, still doing)

The original RenameBookmark/RemoveBookmark APIs now share an explicit owner
bookmarkMutationEnvironment: Context, index query, current tree, attribute cache,
stage, language errors, progress/cleanup, delay and UI reload. The transaction seam
requires native services without falling back to global state. Owner mutations
share the attribute gate; cancellation while waiting or after a callback prevents
the next side effect. A failed request clears its owner progress even when canceled.

Keep the original broad SQL candidate query (all rows whose IAL contains bookmark=),
then compare the current AST bookmark exactly. Preserve marker validation before
trimming, empty-name rejection, slashes, same-name no-op, missing-node tolerance,
per-root cache/save/delay and final ReloadUI; unrelated and prefix labels remain.
The common indexdb.BookmarkBlocks algorithm retains all 21 SQL fields and is used
by legacy SQL and Lease.NativeBookmarkBlocks; the latter borrows only its own index
with cancellation/admission and never flushes a global queue or writes source.

Six model tests and two index/Lease tests cover owners sharing IDs, broad/stale
candidates, exact names, original ordering, failures, late cancellation, dependency
validation, read-only rejection and gate serialization. Final verification is
recorded in the execution plan. R02 remains doing (72 actions, AV60/60); this does
not contextualize BuildBookmark/BookmarkLabels presentation, supply R04 source
commit/receipt, or complete R06 multi-Context UI routing. No public API/CLI/schema,
UI layout, resource or packaging changes.

### R02 original bookmark panel and labels (2026-10-26, still doing)

BuildBookmark and BookmarkLabels now use explicit bookmarkReadEnvironment services.
The shared blockview.BookmarksView retains original FromRow projection, name before
AV title before outline, SQL order within lexical label groups, counts and depth.
Original outline rendering is shared with the existing model wrapper. Native
OutlinePreview restores temporary style/image-title edits, including on renderer
failure; native failures/cancellation return no partial successful panel. Legacy
keeps missing-tree SQL fallback and best-effort AV names. Compact AV markup is
parsed safely, avoiding the original substring helper's backwards-slice panic.

indexdb.BookmarkLabels retains the original raw IAL substring/dedup/sort semantics,
including custom-bookmark candidates; panel grouping still uses parsed bookmark
keys. Malformed label bounds fail explicitly instead of panicking. Labels keep the
original no-flush rule. Lease.NativeBookmarks flushes only its owner main queue,
then reads index, trees, parser configuration and AV files under one admission/gate;
NativeBookmarkLabels uses the same owner SQL algorithm. The private transaction
read seam forces native mode and its outer Context, with no global fallback.

Tests include two real source-to-index workspace projections with shared IDs and
different bodies/AV names, read-only digests, missing AVs, projection rules, input
copies, renderer restoration, dependencies, faults and late cancellation. Explicit
AV index content readers are still required when projecting AV nodes; no hidden
provider was added. Final verification/evidence is in the execution plan. R02
remains doing; other configuration/document lifecycle/background/query callers,
legacy nested binding consistency, R04 commit and R06 full UI routing remain.
Public API/CLI schemas, original UI and resource/packaging layout are unchanged.

### R02 original document navigation (2026-10-26, still doing)

SearchDocsByKeyword now shares internal/docquery.Search with the explicit private
transaction seam and Lease.NativeSearchDocuments. Notebook/configuration, root SQL
and optional deck/count services belong to the caller; no global fallback, queue
flush, source write or public route/CLI parameter is added. Keep the original
notebook exact-any-token match, document keyword AND with hpath/name/alias/memo OR,
opened-notebook filtering after SQL limit, root exclusions, path/icon fields,
lexical hPath order, empty keyword listing and missing-deck/zero-card behavior.
Legacy captures its builtin deck/IDs and uses the original count helpers. Native
flashcard mode requires explicit owned services; deck storage/task ownership is
still pending and is not silently replaced by a successful empty deck.

indexdb.SearchDocumentRoots retains original box-desc/depth-asc SQL selection,
complete rows, NAM flags, LIMIT and LIKE wildcards, but binds keywords/exclusions
as values. Apostrophes no longer break the query and strings cannot become SQL.
Both legacy SQL and native services use this algorithm. Native query/projection
borrow one source/index gate and configuration snapshot; no nested lease calls.
The new pure domain is included in the existing dependency/global-import audit.

Nine new tests cover original rules, faults/late cancellation, parameter safety,
real owner source projections and explicit card counts; actual old API checks use
real builtin cards. Final evidence is recorded in the plan. R02 remains doing;
ListDocTree, original flashcard resources, other controllers/lifecycle/background
callers, legacy nested binding consistency and R04/R06 boundaries remain open.

### R02 original document tree listing (2026-10-26, still doing)

ListDocTree now shares internal/doctree.List and its original File JSON type with
an explicit private transaction environment and Lease.NativeListDocumentTree.
Owner services supply notebook/config, directory/stat/IAL reads, metadata,
references, custom sort and optional cards. Preserve parent-document/directory
merging, hidden-child exclusion even with showHidden, references, all 15 sort
modes, notebook-over-global fallback, timestamp-prefix ties and totals before
truncation. No implicit index flush or source save is added. Metadata and the
file-tree alphabet comparator are shared with their original model/util wrappers;
relative times include original before/after language labels.

Native reads stay within the lease, reject traversal/symlinks, use actual .docj
properties and bounded reads, and read notebook .wiki/sort.json without creating
it. They never perform legacy Box.Ls tmp cleanup or docIAL corruption moves.
Explicit legacy adapters retain those original policies and child-read fallback.
Counts/sort/child projections copy File values before editing them. Native errors,
missing services and late cancellation return no successful partial list; cards
remain explicitly supplied owner services, not an implicit global deck reader.

Tests cover sort/merge/hidden/count/limit rules, metadata/language/comparison,
faults/cancellation and two real workspace filesystems with shared IDs, nested
files, source references, custom order, stale tmp retention, corruption and late
symlinks. Original API validation and final evidence are recorded in the plan.
R02 remains doing; other path/navigation/lifecycle and flashcard resources,
legacy nested binding consistency, R04 source commit and R06 UI routing remain.
No public CLI/API schema, original UI or resource/packaging change.

### R02 original document path/title helpers (2026-10-27, still doing)

The six GetHPathByPath/GetHPathsByPaths/GetHPathByID/GetPathByID/
GetFullHPathByID/GetIDsByHPath helpers share docquery.Paths with explicit model
and Lease environments. Keep the root shortcut, current-tree single lookups,
notebook prefix/error, indexed batch HTML decoding and actual-path dedup with
unspecified map order, missing batch entries skipped, nil empty batch and []
empty IDs. IDs retain first-seen dedup order. Callback failures/late cancellation
return no partial result; native private reads never fall back to legacy globals.

Lease.NativeDocumentPaths borrows one owner source/index gate, builds only its
blocktree registry when needed and performs no implicit main flush/source write.
By-path reads use the exact supplied path, not an ID relocation shortcut. Native
parent paths now match original parent HTML decoding and empty-title Untitled;
missing parents remain errors without original legacy auto-repair. Existing
parser behavior for the current title is retained. Original API and six new test
families verify these rules; final evidence is in the execution plan.

R02 remains doing, with GetDoc/dynamic loading/rendering, lifecycle controllers,
flashcard resources and the remaining R04/R06 boundaries still pending. No public
CLI/API schema, UI layout, packaging or resource changes.

### R02 GetDoc and dynamic loading (2026-10-27, still doing)

The original GetDoc algorithm now delegates to an explicit getDocEnvironment.
Every required service receives the outer request context: queue wait, tree and
parser, backlinks/refcounts, virtual references, search keywords/case, AV names,
DOM and recent-document metadata. Missing services, failures and late cancellation
return no partial result. The legacy API keeps an explicit original adapter and
its existing mode/index/range/fold/EOF/scroll/render/error and null-keyword rules.
Native candidate rendering restores the original AST, including links, mutable
metadata and list identity, on success/error/cancellation/panic; parser/render
options are copied. Callers must hold owner admission and exclusive tree access.
Recent metadata is detached; legacy scheduling uses its cancellable/joinable task
owner instead of a bare goroutine. Native recent persistence is an explicit service.

This is a private model migration seam, not a new Lease.NativeGetDoc or portable
server/CLI handler. Production Workspace source/FTS/regexp/virtual-reference cache
and anchor/AV-name/recent-storage adapters remain to be assembled and validated;
test callbacks do not complete them. R02 remains doing, R03–R10 todo; original UI,
public schema, resource packaging and action/event counts are unchanged.

### R02 actual GetDoc Workspace services (2026-10-27, still doing)

The previous model-only limitation is superseded by Lease.NativeGetDoc. Both the
original model and native Workspace use internal/docview for original GetDoc,
dynamic loading, backlink and marking/virtual-reference algorithms. NativeGetDoc
flushes only its accepted main queue, then uses one source admission/index gate
for actual trees/parser/settings, root-local FTS/regexp, reference counts, real AV
names and recent metadata. No legacy/global dependency, default provider, nested
Lease read or new worker is introduced; the dependency audit includes docview.

Highlighting retains seven full fields, 256 rows, original space-to-OR, types,
case and regex behavior; native errors/cancellation reject partial output.
Virtual references read this owner's SQL and both ignore files, preserve the
original include/exclude/current-document/self/anchor-text rules and use only a
request-local dictionary cache. Each next read sees current SQL/config/source;
this read does not submit dynamic-anchor source changes. Empty keywords after
self-exclusion no longer insert empty regexp spans in either shared path.

NativeRecentDocs reads version-1 local browsing state. Successful GetDoc writes
`.local/workspaces/<pageID>/temp/recent-docs.json` synchronously by atomic rename,
with original front-of-list dedup, 32 entries and timestamp fields. State survives
Context eviction/reacquire, rejects corrupt/oversized/symlink paths and never
enters source/journal/receipts. Cancellation before rename rejects the write;
already committed local state is not rolled back, and cross-process merge is not
claimed. The common local writer now tolerates concurrent parent-directory
creation only after revalidating the resulting directory. Original UI/public
schemas/resources stay unchanged; full multi-Context routing/UI and public CLI
integration remain later phases. R02 is still doing.

### R02 document metadata and statistics (2026-10-27, still doing)

GetDocInfo/GetDocsInfo and their private model environment share docquery.Infos;
Lease.NativeDocInfo/NativeDocsInfo bind actual owner trees/registry, original
reference-container promotion, AV names/localization and child-directory reads.
Only metadata reads flush their own accepted main queue before a single source
admission/index gate. Input order/duplicates, missing batch IDs, flags and null
versus empty reference shapes remain unchanged. Invalid scroll state is removed
only from the response copy, with zoomInId taking precedence over start/end;
non-string corrupt scroll values no longer panic. No source repair is performed.
ChildDefinitions now restores the original reference-ID -> definition-IDs map,
fixing the prior extraction's reversed direction and incorrect document counts.

Child counts retain suffix-only .docj counting, including directories. Legacy
single reads tolerate directory errors; batches fail as a whole on any directory
read error. Native single reads tolerate an absent child directory but reject
other unsafe/read errors; native batches also reject missing directories and do
not mkdir them. Missing registry IDs may be skipped; known bad source is an error.

ContentStat/StatBlock/StatTree/BlocksWordCount delegate to the same docquery
statistics core. Lease.NativeDocumentStatistics uses actual source/parser/full
AV read adaptation under one gate, without an implicit main flush, source save or
recent write. Original Lute counts, tree-vs-block behavior, input-count semantics
for missing/duplicate/overlapping batch IDs, and full AV field counting remain.
Number formatting uses a copy rather than mutating cached AV values. Actual
owner isolation, failures and API regressions are recorded in the execution plan.
Public schemas/UI/resources remain unchanged; R02 remains doing, with editor
position/breadcrumb helpers and lifecycle controllers still to be connected.

### R02 block positions and breadcrumbs (2026-10-27, still doing)

The original index, batch-index, breadcrumb and internal embed/backlink helper
now share docquery.Position/BreadcrumbFromNode. GetDOMText/renderBlockText share
the original text extraction, image fallback and HTML rules. NativeBlockIndex,
NativeBlocksIndexes, NativeBlockBreadcrumb and NativeBlockPosition bind actual
owner trees, notebook business, registry HPath, AV/OCR/text and parser under one
source admission/index gate. They add no main flush, nested Lease call, implicit
global provider, source repair or worker; private model reads use the outer ctx.

Original differences remain: single index counts direct document blocks from 1
and reports their total for a root target, while the batch root index is 0. Batch
reads use only the first ID's tree, preserve unknown/cross-tree zeros, collapse
duplicate map keys and return {} when empty or the first tree is absent. Reads
no longer annotate AST Box/Path; known bad source/cancellation rejects partial
success. Breadcrumbs preserve container/heading/task-list rules, registry path,
subtypes, name priority, 1024-character text truncation, exclusions/Caret/HTML,
current-name clearing and embed headingMode=2 behavior. children remains null.
GetDOMText still reads only the first DOM block. Missing notebook records return
an empty path; unbound business and required native AV failures are explicit.

The original API keeps explicit legacy compatibility, including old load/AV error
policies. Actual two-Workspace source/registry/AV tests and original API regressions
are recorded in the execution plan; this does not complete native UI/transport
routing. Node metadata/sibling/relevant-ID and folded-parent/state readers are
connected below; lifecycle core preparation is connected in the next section. Public schemas/resources stay
unchanged.

### R02 node metadata and adjacent relationships (2026-10-27, still doing)

GetBlockTreeInfos/GetBlockSiblingID/GetBlockRelevantIDs now share docquery.Relations.
Private transactions use the outer context; Lease.NativeBlockTreeInfos,
NativeBlockSiblingID, NativeBlockRelevantIDs and NativeBlockRelations load the
owner's real registry and source trees through one source admission/index gate.
There is no main flush, global fallback, AST annotation, repair or new worker.

The original three algorithms remain distinct: metadata scans for identifiable
block parents/neighbors; sibling reads climb ancestors for nested targets and
retain the self-inclusive FirstChildBlock rule; relevant IDs use the physical
parent and skip exactly one adjacent IAL. Original eight-field metadata, root,
inline, empty, missing and duplicate shapes remain. Native known bad sources,
unbound services, cancellation and released leases reject partial success.
Actual two-Workspace tests use identical IDs with different source hierarchies;
original API checks remain explicit legacy compatibility, not full native UI.
R02 remains doing. Folded-parent/state readers are connected below; lifecycle core preparation is connected below; authoritative commits/history and complete transport remain later stages.

### R02 folded parents and fold state (2026-10-27, still doing)

GetUnfoldedParentID/IsBlockFolded now share docquery.Fold. Private transactions
forward the outer context; Lease.NativeUnfoldedParentID, NativeIsBlockFolded and
NativeBlockFold use one source admission/index gate and the owner's actual
registry/source. Unfolded-parent stops at the first open ancestor and excludes
the root and target's own fold; state checks the target and heading-parent chain
for at most 32 nodes, including root fold and its original isRoot flag.

The original preceding-heading/physical-parent rule is retained, including
same-level heading exclusion, missing/inline results and anonymous-container
early returns. Only fold IAL and logical parent are read, avoiding unrelated
getBlock rendering/OCR/configuration work. No global fallback, main flush, AST
annotations, source repair or new worker. Native errors/cancellation/released
leases reject partial success; legacy keeps original empty/false loading policy.
Real two-Workspace same-ID sources verify isolation, fresh reads, cancellation,
queue/source preservation and no unused AV dependency. R02 stays doing; lifecycle core preparation is connected below, while outer
creation controllers and complete native transport/UI remain pending.

### R02 original document lifecycle orchestration (2026-10-27, still doing)

CreateDocByMd/createDoc, RenameDoc, MoveDocs, RemoveDoc/RemoveDocs and
RollbackDocHistory now use doclife.Prepare/Execute and explicit legacy execution.
Private transactions bind the outer context. Original title/ZWJ/length, parser,
parent/depth, subtree filtering/rebasing, root fallback and duplicate-ID rules
are shared. Legacy move/delete executors retain original file, AV, sort, history,
reference and notification business; restore follow-up uses the managed task
queue instead of a detached goroutine. Delete failures propagate.

Lease.PlanNativeDocumentLifecycle reads the owner's actual configuration,
notebooks, parser, source, registry, descendants and scoped historical input under
one source admission/index gate. It returns detached candidates, source hashes
and an effect manifest. It does not flush main, save source/history, execute AV
mutations, change sorts or publish notifications. Effect intents are not durable
commits or a complete native side-effect executor; those remain R04/R05.
Runtime history input is owner-relative and bounded, not a new archival contract.

Actual two-Workspace tests and desktop lifecycle API regression are recorded in
the plan. Single-notebook lifecycle guards remain code -16. R02 stays doing;
outer clipping/daily-note/duplicate/HPath controllers, other resources and full
native transport/UI still need their own owner bindings.

### R02 peripheral creation controllers (2026-10-27, still doing)

CreateWithMarkdown, CreateDailyNote, DuplicateDoc and createDocsByHPath now share
doclife.CreatePeripheral. Private transactions require the outer context and an
owner gate. Clipping parser options, full HPath prefix mapping/preferred parent,
daily-note reuse/date/template attribute rules, duplicate IDs/internal references,
AV mirrors, sorting and progress retain the original behavior through explicit
legacy execution. Duplicate binding attributes are stripped before persistence.

NormalizeCreationTemplate is extracted from the original normalizeTree and used
by its compatibility facade. Actual Workspace preparation binds local notebook
settings, templates, SQL/path/stat/ref services and AV clone candidates, with a
request-local overlay for newly created parents. Source/AV candidates, source
hashes and effect intents are returned; no main flush, source/config/AV save or
notifications occur. Missing notebook settings use the original read-only default;
missing providers and corrupt/escaping source fail. Leading-slash template names
remain relative to the owner's templates directory. Shared tag normalization keeps
native candidates consistent with original persisted attributes.

R02 remains doing. General template preview/export/dynamic-icon controllers,
flashcard resources and remaining callers still need owner bindings. Authoritative
commits/history/effect execution and complete transport/UI remain later stages.


### R02 original flashcard storage and owned tasks (2026-10-27)

The original FSRS implementation is retained in [internal/flashcard/riff](internal/flashcard/riff/PROVENANCE.md), with its upstream AGPL license and provenance. Storage and clock are explicit ports; the original application, import/export adapters and API use this same core. Original deck/card/monthly-log MessagePack fields and builtin deck ID remain compatible, verified by bidirectional reads with the pinned upstream implementation. No new UI or public API schema was introduced.

Each Workspace owns its flashcard store, cancellable serial gate and per-deck round/skip state. Source reads use the pinned data root; reloads observe agent changes, absent storage stays absent, and malformed identities, corruption, symlinks, missing configuration and cancelled/released owners fail. Close drains accepted work and releases source callbacks, decks and caches. There is no import-started worker. Native navigation now binds actual builtin deck snapshots when no explicit card provider is supplied, once per request. Block sets distinguish notebook, one document and descendant documents; totals count unique bound blocks while New/Due count cards, as before.

Lease.NativeFlashcardDecks/Cards, NativeDueFlashcards, SkipNativeFlashcard and NativeFlashcardBlockIDs expose owner reads/runtime work. PlanNativeFlashcard returns isolated create/rename/remove/add/remove-card/due/review candidates with exact preimages and removal flags, with paths relative to the owner's data root (storage/riff/...). Review candidates include the original monthly-log append. Plans never write or acknowledge a committed review; R04/R05 still own journaling, history, source commit and rollback. The old single-workspace adapter retains its actual writes. Complete add/remove/reset transaction attribute/cache/notification binding, card pagination/UI projection, per-document limit overrides and review confirmation/undo still require owner integration; the existence of a candidate is not completion of those callers.

Six new tests plus the dependency audit passed three race-enabled rounds, covering real same-ID/different-workspace data, search/tree counts, subtree scope, reload, borrowed-card isolation, candidate preimages/logs, absent/corrupt source, symlink refusal, cancellation and close/drain. Full core and original package race suites, core vet, three temporary binaries, original desktop/single API regression and 13 integration tests passed. This is macOS arm64 synthetic-source validation; no personal library, Intel, DMG or complete multi-Context transport/UI acceptance is claimed. R02 remains doing.


### R02 flashcard transactions and review confirmation (2026-10-27)

Original add/remove/reset orchestration now shares internal/flashcard.MutationServices with actual Workspace preparation. A plan carries detached original trees, per-deck card files with exact preimages, tree source hashes, and old/new attribute/cache/updateAttrs effects. Preparation does not publish those effects. Original addFlashcards/removeFlashcards receivers and reset controller use the same rules through an explicit compatibility adapter; non-legacy transactions require an owner, cancellable gate and bound ports. Parent rollback discards queued flashcard cache/notification effects, which publish only after successful finish. The legacy file/queue pipeline still has its existing multi-file durability boundary; atomic source/history/journal/rollback remains R04/R05.

ResetTargets intersects each deck independently. All reset no longer carries the previous deck ID into unrelated decks or creates phantom memberships. Removal still cleans orphan cards even when their source block vanished; add avoids orphan cards, rejects an unknown custom deck, and refuses to overwrite orphan native default-card files. One block still creates at most one card per deck. Tree attribute removal prunes unavailable deck references and preserves other existing memberships.

Original Previous is a UI navigation action; scoring that card again restores its first pre-review state before running FSRS. Shared PrepareReview deep-copies that baseline on every replay, fixing repeated re-rating mutating the undo cache. Context rounds are keyed by workspace and deck. Failure/cancellation cannot accept a new round. PrepareNativeFlashcardReview returns a bounded pending token and original deck/card/monthly-log candidates; ConfirmNativeFlashcardReview accepts only this owner's current token and exact persisted afterimages, once. Abort, a new round, skip, configuration/source changes, or close invalidate pending work. External agent edits invalidate old undo state rather than being overwritten by it. Confirmation is a read-only commit-adapter seam: no public writer or receipt is implemented here.

Eleven new tests cover shared tree/deck/attribute plans, All reset isolation, orphan cleanup/refusal, gate/context binding, commit failure, repeated undo, failed/cancelled review, actual same-ID/different-workspace sources, uncommitted/foreign/replayed tokens, source updates and resource cleanup. Final targeted race suites passed three rounds; full core/original race suites, core vet and original API regressions passed. Original API exercised four re-ratings with reps remaining one, All reset preserving a non-member deck's learning state, and unbind attributes; all 13 integration tests passed. Complete native source publication, card pagination/UI projection, document-specific review limits and import/export upper callers remain pending. R02 remains doing; no UI/schema/resource layout change or personal-source mutation.


### R02 flashcard paging, UI projection and document limits (2026-10-27)

Original GetFlashcardsByBlockIDs, GetNotebook/Tree/DeckFlashcards and due-card controllers now delegate to shared flashcard.ReadPage/ProjectPage/ReadDueView through explicit owner services. NativeFlashcardPage and NativeFlashcardDueView bind actual owned decks, Main block records, source document attributes and language resources in one admission. Missing indexed blocks retain the original placeholder without fabricated riffCard details; reads never flush the pending index. By-ID reads preserve input order and duplicates. Page views preserve original SQL/UI escaping, 36-character presentation settings, due-date ordering and zero-date card-ID comparisons, with deterministic equal-due ties. Blocks are mapped by ID rather than assuming SQL row order; independent results do not borrow cached card/IAL objects. SQL batches are bounded at 512. Invalid nonpositive pages/sizes fail internally (legacy signatures return empty); past-end pages keep totals, and integer page arithmetic cannot overflow even at MaxInt.

DocumentLimits copies workspace defaults then applies original custom-riff-new-card-limit/custom-riff-review-card-limit Atoi rules, including zero, negative, malformed and overflow results. Native root/tree requests read the actual latest root properties, with no implicit index flush; notebook/deck requests retain workspace defaults. Per-call limits do not mutate stored configuration or invalidate another workspace's limits. Original calendar-difference formatting is extracted to conf.FormatDiffTime and Business.DiffTime with detached owner labels; legacy util.HumanizeDiffTime delegates to that same algorithm. Due-card JSON fields and UI remain unchanged.

Six new tests passed three race-enabled rounds, including real same-ID/different-owner sources, unflushed placeholders, all page scopes, latest/corrupt limit source, language snapshots, duplicates, reordered/missing SQL rows, batch limits, integer overflow and cancellation/released owners. Full core/original race suites, core vet, original desktop/single API and 13 integration tests passed. Original API verified page order/counts, past-end and invalid pages, by-ID placeholders, document/notebook scopes, separate new/review limits and next-due labels. R02 stays doing; next are import/export upper caller bindings. Formal source commit/receipt and complete multi-Context UI/handlers remain R04–R08. No personal data, frontend/schema, packaging or public CLI changes.


### R02 flashcard document-archive import/export (2026-10-27)

The flashcard phases inside original ImportSY and exportSYZip now use shared flashcard.ArchiveServices through an explicit private owner environment. Non-legacy callers must supply their gate, outer context and read/stage ports; missing bindings never fall back to global Decks or host paths. The compatibility adapter still performs actual original ZIP/document publication. Its extraction reader pins the chosen archive root, checks storage/riff components and regular files, bounds reads, and rejects symlinks. Read/decode/save errors propagate; export failure does not produce a completed archive link. Old temporary builtin payloads are replaced, including clearing the two stale files for an empty export.

The existing format remains storage/riff/<builtin>.deck plus .cards. Export includes builtin cards from selected document trees, excluding reference-only documents, custom decks and logs; card/block IDs are retained and learning resets, as in the original AddCard-based export. Import creates new card IDs, maps block IDs into the actual imported trees, merges into a detached destination builtin deck, and resets imported learning while retaining existing destination progress. Unmapped source cards are skipped; mapped external targets, ID collisions, inconsistent/corrupt file pairs and foreign paths fail. Imported bindings are normalized to the builtin cards actually present in the archive, so stale custom-deck IDs cannot bind into the destination workspace; unrelated IAL properties remain unchanged.

PlanNativeFlashcardExport reads actual selected trees/decks and returns archive bytes plus source hashes. PlanNativeFlashcardImport accepts bounded copied payloads and detached imported document bytes, validates target notebooks and existing-ID conflicts, then returns tree/attribute/card-file candidates with exact preimages. Neither method writes source or publishes cache/notifications. These are the flashcard phases of document/notebook archives; complete ZIP/assets/AV controllers, full ImportData/ExportData backup flows and atomic publication/history/receipt remain under the other R02 dependencies and R04/R05, not claimed complete here.

Seven new tests passed three race rounds, covering format/reset/remapping, unmapped/foreign cards, detached merge, failure/cancellation, real same-ID/different-workspace decks, existing-ID rejection, environment binding and symlink refusal. Full core/original race suites, core vet, original ZIP export/import roundtrip and all 13 integration tests passed. The API roundtrip validated selected parent/child documents only, source progress unchanged, regenerated IDs, fresh learning, normalized bindings and rejection of a missing-card-file archive without modifying the destination deck. R02 remains doing; next are the remaining AppConf/path/cache/SQL and background caller graphs. No UI/schema, package entrypoint, personal data or release changes.


### R02 full configuration presentation, paths, emoji and block SQL reads (2026-10-27)

The original getConf and Lease.ConfigurationPresentation now share the complete AppData mask/role/sync presentation algorithm. Returned configuration is detached and has no persistence authority; original serialized fields and compatibility single-mode projection remain intact. Acquire binds workspace/data/conf presentation paths to the registered page/runtime instead of provider JSON. RuntimePaths/RuntimePath expose validated selection values only; actual reads retain pinned roots.

Workspace-owned custom emoji snapshots now feed the actual document/GetDoc/info/position/lifecycle, bookmark, reference, index and AV parser paths. Each parser owns both emoji dictionaries; index rendering rules remain unchanged. Source emoji reads never execute legacy read-time rename repairs, reject symlinks/invalid paths, refresh deletions and bound the collection. The compatibility model uses the same owned registry type.

Original GetBlock/GetBlocks and NativeBlock use shared CachedBlocks with the owner's actual database and bounded BlockCache. Reads preserve input order, duplicate values and missing placeholders, parameterize batches, propagate native cancellation/errors, and return copies. Failed batches publish no partial cache; close and committed main-queue invalidation remain owner-local. Original API error-to-log/nil behavior stays in its compatibility facade.

Eight new tests, full core/original race suites, core vet, original desktop/single API and all 13 editor/entrypoint/DOCX integrations passed on synthetic macOS arm64 fixtures. R02 remains doing: original settings/login/config-import and repairing Notebook controllers, remaining SQL/transaction/cache and background caller ownership still need migration. Shared presentation does not mean all HTTP handlers have become multi-Context. No UI/schema, public command, packaging or personal-source changes. Details and evidence are recorded in the AnonyPower execution plan.


### R02 original setting controllers and cache publication (2026-10-27)

Ten original setting handlers now share AppData.PrepareSetting: editor/read-only, file tree, search, AI, flashcards, account, snippets, bazaar and keymap. Original defaults, limits, path trimming, conditional effects and response JSON are retained. Missing required nested objects or invalid payloads return errors instead of panicking. AppConf.ApplySetting validates required effect ports, serializes owner operations with cancellable admission, saves the detached candidate to the private bound path, publishes memory, then runs ordered effects. Save failures publish neither memory nor effects; read-only/unbound owners reject writes. Accepted effects retain request values with context.WithoutCancel after saving: a disconnected HTTP caller cannot skip necessary runtime updates. A post-save effect failure is explicitly marked configuration_saved_effect_failed and does not claim to undo persisted data. The old SaveError read-only behavior remains intact.

PlanNativeSetting runs against the actual acquired page configuration and returns a detached setting/effect candidate plus classified preference Set/Remove patches. Patches are deltas to merge into existing owner documents, not replacement documents. Runtime read-only/path authority is excluded; mixed device/workspace edits stay classified. It does not update live resources, commit source, start tasks or notify UI. Formal publication and multi-Context HTTP assembly remain later phases.

A full-suite failure exposed an existing buffered IAL overwrite before AV-name notification. The shared PublishBlock barrier drains older admissions before setting the new value and waits for publication; the original AV-name adapter uses it before notifying. Normal cache warming remains buffered. Tests now assert the current cache inside the notification callback, without polling or relaxing timeouts.

Nine new tests, full core/original race suites, core vet, actual desktop/single setting HTTP normalization/save/rejection/restore checks and 13 integrations passed using synthetic macOS arm64 fixtures. R02 remains doing. Remaining setting/export/appearance/publish/emoji/virtual-reference controllers, cloud login, config import, Notebook repair and other SQL/transaction/background callers stay open. No UI, public command/schema, package entrypoint, personal-source, commit or release change.

### R02 remaining keyword/emoji/export setting controllers (2026-10-27)

Five more original handlers (virtual include/exclude/refresh, recent emoji, export) now use the shared owner setting pipeline. Original escaped-comma/whitespace/deduplication and upload-name/UTF-8 rules live in internal/textprefs; model/docview/util delegate to that same implementation. Refresh still accepts an empty HTTP body. NoSave and EffectsOnly preserve the original distinctions: recent emoji changes memory without an immediate save, refresh does not replace configuration, and empty keyword additions only notify. Later ordinary compatibility saves can still carry the recent emoji values, as before. Nonempty keywords save before cache reset/config notification. Runtime-only effect failures do not claim a persistent commit.

Export uses explicit SettingServices for current Pandoc selection and validation. The fallback is resolved inside the owner gate, so it follows a preceding accepted tool selection. The compatibility probe uses CommandContext, a five-second timeout, bounded 64 KiB capture and WaitDelay; invalid executables retain the prior selection and original warning/error response shapes. Private read-only/unbound guards run before probing. Trusted Workspace Options bind these capabilities per identity; missing bindings fail closed, and preference JSON cannot install a probe. Native plans retain classified patches/effect intent without source/config/notification publication; runtime executable paths never enter source preferences.

Ten new tests, full core/original race suites, core vet, both original API modes and 13 editor/entrypoint/DOCX integrations passed on synthetic macOS arm64 fixtures, including the actual bundled Pandoc 3.5. Appearance/publish service ownership, cloud account, import/Notebook repair and remaining SQL/transaction/background callers still keep R02 doing. No original UI, public CLI/schema, package entrypoint, personal source, commit or release change.

### R02 owned publishing runtime and original controllers (2026-10-27)

Original setPublish/getPublish now serialize with the owning configuration gate. The setter uses shared normalization/private persistence and an explicit runtime effect; the getter reads a detached snapshot without rewriting preferences. Response fields and existing auth middleware remain unchanged. Listener startup is explicit at compatibility boot, errors are reported, and registered service teardown runs before DB closure. The old delayed publisher goroutine is removed; fixed-port and other startup work remain separate unfinished caller graphs.

internal/publishsvc owns its listener, HTTP server, request cancellation, serving goroutine and retryable Close. Construction starts no IO/worker. Upstream, listen host, transport and account/session ports are explicit. Port zero reuses the assigned listener on identical reads. Replacement binding happens before retiring the working listener, so conflicts do not silently lose the old service. Closing cancels requests, joins handlers/serving and closes owned auth/idle connections; a timed-out Close can be retried. Auth configuration must replace its state atomically and callbacks must be bounded. Network IO runs outside the credential gate so disabling can cancel a blocked proxy call.

The original Basic Auth/session/anonymous-reader and JWT claim protocol remain. Identical reads retain keys/sessions; credential, enabled-state or compatibility-owner changes revoke old keys/sessions. Legacy account/JWT access is synchronized and returns copies, and owned close clears its authority. Backend failures no longer dereference a nil response or leak undelivered new sessions. These are publishing visitors, not the still-unmigrated cloud account workflows.

Options.PublishOptions and StartNativePublish/StopNativePublish bind actual acquired configuration to an optional Context-owned runtime; missing ports fail closed and resource teardown closes it. Native setting plans remain candidates, not source commits. Per-Context listeners were tested with separate loopback ports and real fixture source bytes; single-port multi-Context routing and complete native UI/backend auth assembly remain R03/R06. All network fixtures use loopback, and original-app API probes keep publication disabled.

Ten new tests, full core/original race suites, core vet, original desktop/single API and 13 editor/entrypoint/DOCX integrations passed on synthetic macOS arm64 data. R02 stays doing: appearance copy/theme/icon/watchers and its setter, cloud account/import/Notebook repair, remaining SQL/transaction/background callers and legacy workspace-switch assembly are still open. No UI, public command/schema, packaging, personal source, commit or release change.

### R02 owned appearance resources and original settings (2026-10-27)

Original setAppearance and explicit boot now share conf.ProjectAppearance and internal/appearance.Owner. The original UI/JSON, locale labels, builtin-first theme ordering, icon sorting, theme mode filtering and fallback selections remain. Metadata is resolved after fallback, preventing stale versions/JavaScript flags; missing builtin metadata no longer creates nil theme entries. ReloadIcon refreshes icon metadata only, without installing resources or restarting theme watchers.

New/Prepare start no worker and install nothing. Explicit Apply copies a bounded trusted bundle into device resources, retains installed custom files, then owns the CSS watcher. os.Root confines metadata/copy access; native directory links are rejected, while compatibility can explicitly authorize existing linked resource directories. List/read/copy errors return to the caller. Stop removes every watch before backend Close (including the original kqueue descriptor leak path), cancels and joins the worker; failed setup is cleaned before publication and timed-out Close can be retried. Callbacks must be bounded, non-reentrant and honor cancellation. Only selected custom-theme CSS write events produce the original refreshtheme message.

The setter normalizes before private persistence, then installs/restarts resources, refreshes locale/emoji/status-bar runtime state, conditionally clears marketplace cache and broadcasts the original response. Save failure runs no effect; a post-save install error reports configuration_saved_effect_failed rather than claiming rollback. Device-file copy is not a transactional preference/source commit. Options.AppearanceOptions binds actual Context-owned resources with NativeAppearance/StartNativeAppearance/StopNativeAppearance; read/planning stays detached and read-only, native setting changes remain device-preference candidates. Source and preferences are never written by these runtime methods.

Eleven new tests cover projection, save/effect ordering, real file events, repeated descriptor inventory, failed activation/close retry, original HTTP and same-ID dual-Context isolation. R02 remains doing; cloud login/config import/Notebook repair, other SQL/transaction/background callers, marketplace download paths and complete multi-Context UI transport remain open. Public commands/schema, UI and packaging are unchanged. Verification details are recorded in the shared-wiki execution plan.

### R02 owned cloud-account login, refresh and logout (2026-10-27)

Original Login/Login2fa/RefreshUser/LogoutUser and their HTTP controllers now use internal/cloudaccount with explicit configuration authority, region-selected request, codec and clock ports. Login still persists the selected region before authentication and returns a token without establishing a user; refresh fetches/decrypts the user and persists the inherited encrypted userData shape. The original 2FA token header/body, error-code remapping, empty-token local hydration, three-day expiry threshold and transient-network fallback remain. Corrupt ciphertext, null/malformed cloud responses and invalid request types are rejected without panics. Local hydration writes no file; read-only compatibility refresh can update memory while persistence is skipped, whereas login/logout writes refuse read-only authority.

Each service serializes account operations and links requests to caller/owner cancellation. Persistence applies only account fields under the same configuration setting gate, then publishes detached runtime user state; save failure retains the previous user. GetUser/SetUser now detach nested user/title values. Logout stays local and clears credentials after save; delayed work reads the current user inside the service gate rather than retaining a token from before logout. Close cancels active calls, waits for their real completion and supports retry after timeout.

The original two-minute account refresh is an explicit bounded taskgroup job with one pending job per owner. Shutdown/outgoing workspace teardown cancels and joins it before DB/config replacement; successful close removes the owner's process registration. Native Contexts bind optional private-device CloudAccountOptions and use NativeCloudLogin/NativeCloudLogin2FA/NativeCloudRefresh/NativeCloudLogout plus a lease-owned delayed refresh task. Missing capabilities fail closed; source preferences, credentials and receipts are not conflated. These private device callbacks must never store credentials in shared wiki source. Public native server/CLI authentication assembly remains R03/R06.

Thirteen new tests cover original protocol/codec, actual loopback HTTP cancellation, failed persistence, read-only cache behavior, refresh/logout ordering, close retry, canceled delayed jobs, original controller shapes and dual-owner isolation with unchanged source hashes. Further cloud sync, subscription/announcement jobs, trial/deactivation/inbox/AI/upload-token APIs, config import/Notebook repair and other unfinished callers keep R02 doing. No original UI, public command/schema or package entrypoint changes. Verification and synthetic evidence are recorded in the execution plan.

### R02 configuration import and Notebook repair Contexts (2026-10-27)

Original system/importConf now delegates to the captured AppConf owner's ImportConfiguration and shared conf.ReadImport/PrepareImport. JSON and the original single root-file ZIP export are read without extracting/upload-name filesystem paths; JSON is capped at 8 MiB and compressed uploads at 16 MiB (20 MiB multipart envelope). The original twelve imported groups and no-extra-runtime-effects behavior remain. Present groups replace their previous values; omitted groups now retain owner values instead of silently becoming nil. Explicit null required groups, duplicate keys and malformed mandatory editor/AI/proxy subobjects reject before persistence. Optional UI layout null remains supported. Device identity/paths, runtime read-only/disabled features/Pandoc capability and non-imported account/auth groups stay owned. Valid original full exports remain importable.

Import shares the setting gate, uses the receiver's private persistence authority and publishes memory only after a successful save. Failed saves/cancellation/read-only bindings preserve the old live configuration. Native PlanNativeConfigurationImport uses actual Context configuration to produce classified Set/Remove deltas without saving preferences, running effects or publishing source receipts; R04/R05 still own formal publication.

Original ListNotebooks/lsNotebooks and the compatibility Business reader now use explicit-context, rooted internal/notebooks.Scan. Listing is separate from repair: valid metadata/unknown extensions remain unchanged; missing metadata yields a closed Untitled candidate, known missing guide notebooks stay excluded, corrupt metadata yields an exact preimage/hash deletion candidate and is omitted this pass. Canonical .wiki and legacy .siyuan are read explicitly; native auto mode rejects competing roots and symlinks. Sorting and custom icon filtering share the original rules. Read failures are propagated instead of returning a successful partial list.

Compatibility repair requires the private writable owner and selected-source guard, validates all preimages before mutation, writes missing files with exclusive creation and unindexes only after save. Corrupt removal preserves the original two-pass behavior: next listing sees missing metadata and creates defaults. The former unowned flush/cache goroutine is now synchronous within this explicit operation. Read-only/unbound owners never repair. This compatibility writer is not a cross-process source transaction; earlier successful writes can remain after later IO/effect failure, which is returned explicitly. Native PlanNativeNotebookRepairs returns only candidates with paths, missing markers, preimages and hashes; actual native source repair/backup/index publication remains R04.

Fourteen new tests cover import/ZIP boundaries, ownership/authority, failed saves, cancelled waits, required groups, read-only repairs, original HTTP, preimage conflicts, two-pass repair, symlinks, sorting and real same-ID dual-Context isolation. No original UI, public CLI/schema or package-entry change. Remaining B1 snapshot coverage, other cloud/query/transaction/background callers and formal native publication keep R02 doing; validation and evidence are recorded in the execution plan.

### R02 B1 root shared-preference snapshot component (2026-10-27)

StorePreferencesSnapshot captures only .wiki/preferences.json, not a complete B1 knowledge-store archive. Its versioned root-store-preferences kind, store identity/format, store hash, metadata-tree digest, canonical UTC time, explicit presence marker, exact file path/size/hash, raw bytes and overall digest are verified before use. JSON stores source bytes as base64, preserving formatting and large integers. An absent file is distinct from an empty typed preference document. Invalid scopes/secret or runtime fields, unsupported versions, wrong paths, malformed/null/missing presence fields, duplicate keys and bad hashes reject; SHA-256 is integrity evidence, not source authentication.

SnapshotStorePreferences and PlanRestoreStorePreferences run through the actual StoreContext/shared source admission. A future B1 archive can compose the component with other reads inside the same WithReadContext. Capturing absent preferences creates no source defaults or snapshot directories. No device file, page preference, database or credential is part of this component. Existing .local locks may be provisioned by normal store opening; snapshot capture itself is a committed-source read.

RestoreStorePreferences requires releasing live leases/read admissions. The plan binds the current target tree and file CAS independently of the old snapshot's provenance. The writer verifies the same store identity, then reuses source CAS/journal/recovery/receipts; exact source bytes or absence are restored. The manager drains/rebuilds all page Contexts, retaining page/device preference overlays and leaving other stores alone. Stale hashes/tree, malformed plans and self-upgrade from an active read admission reject. Absent-to-absent returns no_op with SourceCommitted=false. A rolled-back operation remains replay-only; an intentional retry uses a new operation ID. This is actual root-preference publication, not an uncommitted candidate, but it does not implement S09's full archive/new-directory restore or S11 selective/in-place restore orchestration.

Ten new tests exercise exact bytes/large integers, absent/present states, strict envelopes, identity/CAS/secret rejection, shared-admission consistency, all-page invalidation, device/page isolation, interruption/recovery and fresh-process restore into an independent synthetic source without old runtime data. No public CLI command/schema, original UI or packaging entrypoint changes. R02 stays doing; full B1 sealing/transports and new-directory recovery remain S09.

### R02 trial, deactivation and upload-token ownership (2026-10-27)

Original StartFreeTrial/DeactivateUser/LoadUploadToken and HTTP trial/deactivate/upload entrypoints now use the account owner's shared cloudaccount core. Trial/deactivation preserve their original bodyless POST and symphony account cookie, route permissions, response shapes and local-account behavior: success does not implicitly refresh, log out or persist the user. Missing authentication fails without networking; malformed responses and non-success HTTP states no longer panic or falsely succeed. Callers propagate cancellation through the original cloud HTTP transports. No real account operation was performed during verification.

Upload tokens are private per-Service memory, keyed by account ID/token, region and entitlement state. A successful nonempty response alone fills the cache; reuse lasts through the original 3600-second boundary. Expiry, wall-clock rollback, identity/region/entitlement changes, trial/deactivation attempts (including uncertain cancellation/network outcomes), 401 and logout/close invalidate the applicable entry. Concurrent calls on one owner share its serialized fetch. Token business errors now reach the original asset caller instead of returning nil and uploading with an empty/stale credential. No token is added to AppData, source preferences, snapshots or public HTTP responses.

The same UploadAssets executor serves compatibility file uploads and actual native path-based uploads. It preserves resolved-file deduplication, subscriber-only upload-assets behavior, 3/10 MiB limits, up to three oversize notices, file[] multipart, meta-type 5/4 and the original biz-type values. The whole batch holds account identity/token ownership; logout cannot interleave with a file request. Cancellation/Close join actual transport work, progress cleanup runs on errors, and accepted partial remote uploads remain counted rather than claimed rolled back. Files are bounded immutable per-file copies before multipart IO. Legacy reads use a pinned data root, reject traversal/child symlinks and stat oversized files without opening their payloads.

Native trial/deactivation/token/batch methods run as existing lease-owned tasks; drain cancels and joins them. Native batches accept explicit page-relative assets, freeze at most 128 paths/64 MiB under their own source admission (metadata only for over-10-MiB files), then release that admission/resource gate before network IO. Callers must also avoid keeping an outer long-lived WithReadContext around networking. Missing transports/capabilities and unsafe paths reject; no fallback to global Conf is allowed. HTTP/transport callbacks must be bounded, honor ctx and not reenter the same owner.

Thirteen new tests cover actual loopback cookies/bodyless and multipart wire, cache boundaries/identity/concurrency, failure propagation, 401, file changes, partial/canceled upload, source-lock release, drain, source isolation and original HTTP shapes. Original document/AV/embed link gathering and the forum article query/post/source-attribute stage still have legacy callers; forwarding their asset stage/request ctx does not mark the entire forum feature migrated. Activation/inbox/reminder/AI/cloud-space/repo and other cloud graphs remain open. R02 stays doing, with original UI/public command/schema/package entrypoints preserved.

### R02 activation-code ownership (2026-10-27)

Original CheckActivationcode/UseActivationcode now delegate to the same cloudaccount owner through request Context; NativeCloudCheckActivationcode/NativeCloudUseActivationcode run as pinned Lease tasks. Both preserve TrimSpace -> RemoveInvisible -> RemovePUA, the original JSON data field, symphony account cookie, explicit region and original cloud routes. Check retains success confirmation text and collapses nonzero business codes to 1; redemption returns the original business message. HTTP permission middleware and the original check/confirm/redeem/UI-refresh flow are unchanged. Legacy background wrappers remain for existing callers; malformed HTTP data types now return an error instead of panicking.

The account gate serializes check/redemption with login/logout/upload. Redemption invalidates the private upload cache before attempting the request, including rejected or uncertain outcomes; read-only check retains it except on 401. Non-200 responses retain the original connection-failure presentation. Missing accounts fail before networking; malformed replies and transport failures cannot falsely succeed. Request/owner cancellation reaches the original 30-second client; close/drain joins real IO. No local user/configuration/source mutation, implicit refresh/logout, automatic redemption retry, code logging or new public CLI/native HTTP command is introduced. A canceled redemption may already have taken effect remotely; the UI must refresh before deciding whether to retry. Real account redemption was not performed; verification uses synthetic local transport and loopback fixtures.

The original HTTP client retries failures/503 once by default. The compatibility transport now overrides that policy only for the redemption route (SetRetryCount(0)); an actual loopback 503 verifies one send. Injected native Request ports must likewise honor the single-attempt redemption contract. Check and other account routes retain their existing retry policies.

### R02 cloud Inbox ownership (2026-10-27)

GetCloudShorthands/GetCloudShorthand/RemoveCloudShorthands now delegate through request Context to the existing cloudaccount owner. InboxRequest is an optional explicit JSON transport required only by these methods; construction starts no networking and existing account ports are unchanged. Bodyless POST reads, region and symphony cookie, full nested list envelope/pagination, detail item response, and original UI code=1 failure shape are retained. IDs in detail queries are escaped, malformed HTTP input and cloud replies are rejected instead of panicking. The original list audio/video/file descriptions, device-local creation time, raw shorthandMd and Protyle preview are shared; detail alone enables footnotes. Each read owns a captured parser/configuration and a copy of the cloud response, so projection never mutates a shared transport result.

NativeCloudGetShorthands/NativeCloudGetShorthand/NativeCloudRemoveShorthands use existing pinned Lease cloud tasks. Parser settings and emoji state are captured under this page's source/resource admission, which ends before networking. Caller cancellation and drain reach real IO; owner close/drain join actual callbacks. Login/logout cannot interleave within an Inbox operation. 401 clears that owner's upload cache; Inbox otherwise does not persist account state or source/receipt. Removal keeps JSON ids array/order/duplicates and uses a single attempt in the compatibility HTTP client, overriding its default retry; trusted native InboxRequest must do likewise. Cancellation may follow a completed remote deletion and is not a rollback claim.

Original Inbox UI, permissions, pagination and read/create-document/remove orchestration remain. This batch adds no native public HTTP/CLI routes, no cross-endpoint atomic cloud-to-document move, and does not migrate the separate OS-local MoveLocalShorthands importer. Verification uses only synthetic Inbox content and loopback/mock cloud services, never a real cloud inbox.

Direct callers retain nil versus empty removal slices; the original HTTP controller still builds a nil slice for an empty input array. Original-app regression compares all configuration fields except the separately maintained dataIndexState runtime flag; isolated model tests verify the entire configuration file remains byte-identical.

### R02 block reminders and subscription-expiry ownership (2026-10-28)

Original SetBlockReminder and SetCloudBlockReminder now delegate through request Context to the cloudaccount owner and shared internal/reminder preparation. The original subscription/iOS messages, dateparse/device timezone, timed=0 deletion, requested remote ID versus first container-leaf local target, 128-character truncation before zero-width-space removal, IAL/cache key, notification text and 3/5-second durations are retained. ReminderRequest is an optional JSON port preserving numeric timed, symphony cookie and explicit region. Non-200/malformed replies, nil accounts and bad HTTP argument types fail without panic or false success; the original closeTimeout=7000 error shape and middleware/UI remain.

One account admission and the existing attribute gate span preparation, network and local continuation. The compatibility adapter captures owner/workspace/data root/read-only authority, uses cancellable transaction queue drain and contextual original static-content projection, reads attributes from its cache/tree, and stages through the original upsert queue. Network failure changes no reminder attributes; stage refusal restores the in-memory IAL and reports remote_applied_local_not_staged. After queue acceptance, sync/cache/notification effects finish with request identity even if the request disconnects; effect failures report local_staged_effect_failed. Notification follows successful staging. This preserves the queue acceptance boundary, not an authoritative R04 multi-file commit. A canceled/failed remote reply may hide a completed remote change; cloud and source are not atomic and no automatic retry is issued. The original client retry policy is overridden only for this reminder write route.

The global subscriptionExpirationReminded flag and detached legacy delayed notification are removed. The same account Service owns once-per-identity planning, paid/trial 15/3-day cutoffs, remaining whole days, expiry-within-two-days rule and original 7/30-second delays. AppConf taskgroup and Native Lease task own timers and actual callback lifetime; close/drain cancels and joins. Before notification, account token/region/entitlement identity is rechecked, so old-account reminders are suppressed; refresh resets this owner's check only. Native notification is an explicit trusted device port. No expiry state or credentials enter page preferences/source.

PlanNativeBlockReminder is read-only: detached original tree/attribute/content/notice intent plus source hashes, no network or source receipt. NativeCloudSetBlockReminder is the separately explicit low-level cloud port, and StartNativeSubscriptionReminder owns expiry notifications. They do not publish native source candidates or provide a combined atomic save; native commit/handler role assembly remains R03/R04. The original single-workspace GUI uses the actual compatibility execution above. Announcement/AI/other cloud work and the previously observed complete legacy-combination exit timeout remain open. Verification uses synthetic local cloud/trees only, never real account reminders.

### R02 announcement feed, device markers and delay ownership (2026-10-28)

Original getAnnouncements/refreshAnnouncement/RefreshCheckJob now use internal/announcement through the explicitly bound cloudaccount owner and AppConf taskgroup. The version endpoint remains an anonymous GET with ver and explicit cloud region; no login/cookie is required. Announcement Id/title/url/region, first-ID-wins order, recording all regions before showing only the matching region, language 11 URL/title formatting and timeout 0 are retained. The original one-minute delay is cancellable and pending scheduling is deduplicated. Feed caches are private per owner/region, preserving 6-hour/24-hour Docker windows, clock-rollback invalidation and detached values. The announcement path no longer borrows util.GetRhyResult's mutable global cache; other version/update jobs are not migrated by this change.

The seen ledger retains the legacy device-only .config/siyuan/announcement.json array format (including legacy null/empty arrays), outside wiki source/preferences/snapshots. New FileStore construction does no IO. Explicit refresh merges under a device file lock and atomically replaces through a synced private temporary file, preserving other writers' IDs and refusing child links/nonregular files. New writes use 0600. Empty/missing feeds create no marker files; malformed feeds or malformed/unreadable ledgers do not notify or falsely succeed. Corrupt ledgers now remain intact for diagnosis instead of being silently deleted. Feed/ledger budgets are 10000 items and the ledger is bounded at 16 MiB; excess is refused without truncation. Cooperating writers must follow this locking protocol; old external writers that ignore it are not coordinated.

Device markers commit before notification, as originally: cancellation/crash/notification failure after commit may leave a marker without display, not a rollback or exactly-once promise. Region and owner/path authority are rechecked before commit/display; default native region reads use the serialized account snapshot to avoid racing login/region commits. AppConf/Lease tasks own the full delay, HTTP, file-lock and notification callback lifetime; close/drain cancels and joins real callbacks, including callbacks that have not yet returned after a caller timeout. Announcement options and paths are private capabilities, never imported JSON or request parameters. Native StartNativeAnnouncements releases its own source/resource admission before all delay/device/network work; an outer long-lived source admission remains the caller's responsibility.

The original UI is kept. No public native HTTP/CLI command is added, no account/source receipt is published, and device markers are not portable knowledge. Thirteen new tests cover protocol/cache/records/concurrent snapshots, same-device merge, corrupt/link/lock failure, owner paths, cancel/join, native source isolation and explicit region behavior. All success paths use synthetic local feeds; real user markers are not read or changed. AI/other cloud/version-update work and the prior complete legacy-combination exit timeout remain separate R02 work.

### R02 owned AI configuration, source extraction and conversation (2026-10-28)

Original ChatGPT/ChatGPTWithAction use shared internal/aiflow through an AppConf owner; CloudChatGPT delegates to the cloudaccount owner. The HTTP middleware, UI and string-valued success/provider-failure envelope remain. Missing API key still shows language 193 for 5 seconds and returns an empty string; no automatic cloud fallback is added. Bad argument types now fail safely; request cancellation reaches provider IO and is reported explicitly. NativeAIChat/NativeAIAction and NativeCloudChatGPT are internal Lease-owned capabilities, not new public HTTP/CLI routes.

The per-request immutable settings retain key, explicit proxy, base URL, user agent, model, provider/Azure API version, max completion tokens, temperature, timeout and max contexts. The pinned go-openai SDK and original non-streaming/first-choice/finish_reason=length behavior remain. History messages remain user-role messages; empty messages are skipped only by the direct SDK as before. Action prefix and raw exact action-clear versus trimmed plain-chat clear semantics remain. Length continuation preserves the same original message/context arguments and original round bound; this refactor does not invent a new continuation strategy. Selected documents expand every direct child (including original IAL nodes), repeated IDs stay repeated, and the original Markdown exporter/settings and two-newline separators are reused.

The process-global cachedContextMsg is removed. Each owner serializes calls and owns up to 64 history strings (the maximum supported send window), with per-send trimming to maxContexts. Changing key/endpoint/proxy/provider/API version/user-agent/model clears that history. Actions neither consume nor append dialogue history. Successful nonempty replies commit history at completion; provider failure/cancellation does not append the failed turn. Partial text can still be returned, preserving the original UI string behavior, but typed errors remain available to internal callers. This fixes the original swallowed chatErr/history pollution. No prompt, answer, API key or history is persisted to wiki source/preferences/receipts by this flow.

SDK clients are created from the captured configuration for one operation, receive request/owner ctx plus per-round timeout, and close idle connections after use. Proxy parse failure refuses rather than silently using a direct connection; provider diagnostics are sanitized instead of logging prompts/keys/full endpoint/proxy configuration. Progress keeps original Requesting/Continue requesting/error texts and clears using the accepted owner identity even after cancellation; stale compatibility owner/path callbacks refuse. Cloud chat preserves its original region/cookie/messages/whitespace/nil-user no-op and validates replies, using a single transport attempt per round instead of a hidden retry. Login/logout cannot interleave within its one cloud request.

Native block text is frozen from its page's actual source under a short source/resource admission; network IO runs after that admission ends. Explicit AIOptions supplies the provider/UI capability, with a default configuration snapshot under the same owner's resource gate, no global Conf or process environment fallback. Missing capability and failed initialization refuse/close resources; drain cancels and joins actual direct/cloud calls. Consumers must also avoid retaining an outer source read admission around network work. Authoritative source mutation is not part of AI generation; frontend follow-up editor transactions remain their existing flow. Other cloud/version/update work and the existing complete legacy-combination exit timeout remain R02 follow-ups. All successful verification calls use synthetic local providers, never a real paid API or personal wiki content.

### R02 cloud space and repository metadata Context (2026-10-28)

The original cloud-space, repository-directory list, snapshot list and tagged
snapshot list now share an owner-scoped cloudrepo.Service. Compatibility HTTP
requests pass their Context; original wrappers and response projections remain.
Each operation freezes configuration, credentials and private repository paths;
close cancels and joins actual IO. S3's original active-directory selection and
invalid-name normalization save through an owner configuration comparison.

Native hosts explicitly bind CloudRepoOptions.Execute. The default Load snapshots
this page's configuration; NativeCloudRepoQuery retains a Lease task while IO
runs outside resource admission. No implicit legacy/device capability or public
native route/CLI assembly is added. Directory writes and source checkout are not
part of this read capability.

The pinned dejavu module is retained under third_party/dejavu (see WIKI_PATCHES.md
and its license); go.mod uses a local replacement, so include it in source builds.
Official HTTP requests, S3 SDK contexts and WebDAV transports receive operation
cancellation. Scoped stores bypass legacy global ID caches and release private
codecs; tag-index reads suppress the legacy global UI event. Original algorithms,
UI and snapshot/source formats are retained.

Synthetic tests cover original protocols/projections, actual HTTP cancellation
for all three network providers, truthful close/join, foreign cached identical
IDs, detached owners, configuration CAS and native drain/source invariance.
Directory create/remove/set, snapshot tag mutation/upload/download/purge, local
repo writes, global cloud-lock refresh, full sync and other legacy subscribers
remain R02 TODOs. This batch must not be interpreted as a native write capability.

### R02 cloud directory mutations and owned renewal (2026-10-28)

Directory create/remove/set now use a separate cloudrepo.Options.Directory port;
metadata Execute grants no write capability. Original wrappers and HTTP controls
pass Context and keep permission middleware, naming/provider rules and response
shapes. Set and active-directory removal save through private configuration CAS;
remote Applied/Uncertain/Saved outcomes survive cancellation and stale settings.
No shared wiki preference, source receipt or automatic retry is created.

NativeCloudDirectory retains its Lease task until actual IO and cleanup finish,
without retaining source/resource admission during network IO. Hosts must bind
Load/Directory to the same private device authority, including real persistence;
missing write capability fails closed. Public native routes/CLI role assembly
remain later phases.

The pinned fork's cloudlock package retains lock-sync/deviceID/time, three
5-second contention retries, 30-second renewal and 65-second expiry. Renewal IO
has a 20-second limit. Each directory operation uses a unique token; actual IO is
joined before independent cancellation-detached, bounded unlock. Renewals verify
ownership and cancel their held Context on failure. Legacy repo renewals now own
instance leases instead of a global stop channel and bind their held provider to
that lease Context. The transitional repository coordination gate is cancellable
for new directory callers; full sync/source/task migration is still pending.

Local write endpoints must be absolute and independent of workspace and private
source/repo/history/temp roots, including existing symlink ancestors. No invalid
configuration falls back to cwd. Original UI progress remains owner/path guarded.
Tests use temporary Local roots and loopback HTTP only. The original remote lock
protocol is not atomic CAS/conditional delete, so this does not establish strict
cross-process exclusion or exactly-once effects. Snapshot transfer/tag mutations,
purge, local repo writes, complete sync and legacy subscriber bodies remain TODO.

### R02 snapshot tags, transfers and manual purge (2026-10-28)

Original tag add/overwrite/remove, cloud tag removal, encrypted upload/download,
and manual local/cloud purge now use the separate Snapshot capability. Metadata
and Directory ports grant no snapshot write authority. Original HTTP middleware,
request fields/envelopes, naming normalization, missing-key and membership no-op
rules remain. NativeSnapshot pins its Lease through actual worker IO, traffic
accounting and unlock cleanup; partial Applied/Uncertain outcomes are retained.
No native public route/CLI role assembly or authoritative source receipt is added.

The pinned fork reuses original encrypted/compressed index/file/chunk algorithms.
Scoped workers are bounded, cancel on first failure and always join; traffic is
synchronous within the operation. Local index/tag and remote tag publication only
follow successful object transfer/verification. Interrupted downloads recheck
chunks of already cached files; wrong IDs/hashes and mismatched local upload tags
are rejected. Private repository roots/symlinks cannot alias authoritative data.
Download populates repository cache only and never checks out Wiki documents.

Scoped purge retains the complete reference graph and fails closed on unreadable
retained indexes/files. Local remote listings recursively include tagged refs.
Failures can leave partial cache/deletion effects; no rollback or exactly-once
claim is made. Original lock-sync is still not atomic CAS/conditional deletion.

Tests use synthetic Local roots and loopback IO, including encrypted round trip,
resume after failure, real HTTP worker cancellation/join, traffic join, damaged
retention protection, original API shapes and native source/task invariance.
Local repo key/index/checkout/reset, automatic cleanup, full sync parent/source
queues, other legacy subscriber bodies and complete-combination shutdown remain
later R02 work. Preserve the pinned fork and its license in source builds.

### R02 local index and automatic retention (2026-10-28)

LocalRepo is a separate explicit capability sharing the owner's cancellable gate
with metadata/Directory/Snapshot. Original local snapshot memo/key rules, HTTP
success null, source ignore patterns, encryption/chunking and full-latest msgpack
format remain. Reading syncignore never creates source files. Scoped index workers
cancel on first failure and actually join; large-file readers close exactly once.
Index closure is verified before publishing latest. Partial private cache/deletion
effects remain observable via Applied/Uncertain; cancellation is not rollback.

Automatic retention owns first-sync eligibility, six-hour throttle, frozen policy,
clock and random state per Service. Today's indexes and the original daily newest/
bounded-random rule remain; fewer than three retained IDs skips purge. Pagination
uses the actual page-count return slot. Existing reference-closure protection also
covers automatic purge. AppConf owns deduplicated 12-hour cron/post-sync tasks,
cancels them and waits for actual completion on close. Legacy checkIndex business
is only wrapped by its captured compatibility owner task: its SQL/source repair,
checkIndexOnce and complete sync remain pending.

NativeLocalRepo pins source-read admission throughout indexing, with no resource
serial lock held across IO. Auto-purge only touches private cache. The bound port
must use the same page source and repository; metadata authority grants no writes.
No public native HTTP/CLI assembly or source receipt is added. Key initialization/
reset, checkout/source recovery, full sync and other legacy callbacks remain R02
work; R03-R10 are still pending. Tests use synthetic sources/private temporary
repositories, never personal knowledge or paid cloud services.

### R02 private repository key lifecycle (2026-10-28)

RepoKey is an independent capability for original random generation, passphrase
KDF/Base64 reuse, import and reset. Original 32-byte key format, UI notices and
HTTP envelopes remain. Derivation/validation precedes filesystem changes; private
configuration uses a 0600 temporary file, file sync and atomic rename before
memory publication, with Repo/Sync comparison at commit. AppConf.m is not held
while waiting for the repository gate. Key/password material is never logged.

Old private cache moves into a sibling quarantine until configuration commits.
Save failure/pre-commit cancellation restores it; rollback failure is uncertain.
Post-commit cleanup is cancellable, retaining Applied/CleanupPending and any
quarantine instead of claiming rollback. This is not a cross-file crash transaction
or authoritative-source backup. Interrupted quarantines are retained for inspection,
not automatically replayed or deleted; complete S09 recovery remains pending.

Initialization retains original cancellable two-stage notices and first encrypted
index. An unfinished initial index does not undo a committed key. Reset clears the
private cache/key, disables sync and owns its compatibility-only delayed UI reload;
close cancels and joins it. NativeRepoKey retains its Lease through actual completion,
uses source-read admission for initialization, none for reset, and no resource serial
lock across IO. Private capability binding is mandatory; no public native CLI/HTTP
assembly or source receipt is added.

Legacy newRepository binds captured owner/key/path admission. Queued old-key IO,
index/reset and Sync/manual transfers are rejected after rotation, before cloud
locking in sync entrypoints. This adds no global configuration or key state and
is not the remaining full-sync/source/checkout Context migration. Native public
handlers, checkout/source recovery, full sync/checkIndex and remaining legacy
callbacks remain pending; R02 is still in progress. Tests use synthetic sources
and private local caches; no personal data or paid cloud writes are exercised.

### R02 checkout preflight and read-only candidate (2026-10-28)

PrepareCheckout is an explicit independent capability. Original encrypted file/
chunk and compressed index codecs verify the complete target, streaming hashes
without returning body content. The detached manifest contains normalized relative
paths, original IDs/chunk IDs, sizes/timestamps and plaintext SHA256. Reject empty
indexes, unsafe/ambiguous paths, duplicates, file/directory collisions, broken
chunks/lengths and overflow. Private-cache symlinks are refused; original global
file/index caches are bypassed. Cancellation discards partial candidates; close
and drain retain actual callback ownership. Read-only AppConf works unchanged.

NativePrepareCheckout reads private cache under its pinned task, without source/
resource locks across IO. It grants no source write, source diff, backup guarantee
or transaction receipt, and adds no public CLI/HTTP or native restore handler.
Applying later must reverify the target and source CAS through the source writer.

Compatibility checkout preflights before pausing sync/watchers or backup creation.
Original Repo.Checkout rechecks the complete target within repository coordination,
rejects source-tree symlinks, normalizes relative metadata for source-path comparison
and cleans failed temporary output files. Formats/IDs remain. Valid original HTTP
requests keep asynchronous null success; malformed arguments are rejected.

Only this read-only prerequisite is complete. Original checkout source writes,
queue/watcher/reindex/notification lifetime and full sync remain R02 work. Native
apply requires R04 source CAS/journal/receipt and Context invalidation, never a call
to compatibility Checkout. No crash-atomic rollback or strict hostile-concurrent
symlink replacement safety is claimed. Tests exercise original restore only on
synthetic sources; no personal knowledge or real cloud data is changed.

### R02 compatibility restore ownership and original rebuild stages (2026-10-28)

Restore is a separate trusted compatibility capability. Original HTTP keeps async
null success: request Context controls admission; accepted jobs belong to AppConf.
ContextTask occupies the original serial queue slot through actual callback return,
not a UI timeout. Pending cancellation does not run; running cancellation requests
stop. Owner/path capture, per-target dedup, close/retry join and switch/exit barriers
prevent old jobs from following a new workspace. No NativeRestore or public CLI/
HTTP/role expansion is added; native apply still requires source CAS/journal/receipt.

Preflight, queue/repository-lock wait and backup are cancellable. After write
admission cancellation is deferred through original multi-file IO and synchronous
original rebuild stages. Preserve BackupID and Applied/SourceTouched/Reindexed/
SyncPaused/Uncertain/CancellationDeferred; never imply rollback. Rebuild/cleanup
failure or rebuild panic gates ordinary requests while diagnostics, retry restore
and workspace reopen remain available. Successful retry clears the runtime gate;
this is not durable crash recovery. Original watcher resume and delayed notice are
owner/identity guarded. DB/path teardown only follows actual restore/repo drain.

Original scoped indexing used path+second metadata identity/comparison. Real GUI
regression exposed same-second edits missed by backup and restore. Scoped indexing
now reads all source content, reuses truly unchanged metadata IDs and derives new
40-hex opaque IDs from versioned metadata/chunks. Existing IDs remain immutable;
original JSON/compression/encryption/chunk formats and source document/block IDs
remain. Legacy incremental indexing preserves opaque IDs of unchanged entries.
Owned restore materializes the authoritative target rather than skipping equal-second
mtimes. Full source hashing and target rewrites increase IO for large repositories;
preparation remains cancellable, and historical uncaptured versions are not recovered.

Original rebuild functions run in the owned serial handler, without detached flush
or queued rebuild children. Their lower-level global SQL/cache/watcher dependencies,
other legacy tasks/timeouts, full sync/checkIndex and native source transactions are
still pending. Compatibility backup is not a transaction-consistent source snapshot
or complete S09 archive. R02 remains in progress; tests only touch synthetic sources.


### 2026-10-28: compatibility full-sync and index-repair ownership

Original normal/boot/exit/upload/download and boot-followup controllers now enter
an explicit FullSync capability sharing the owner's repository gate. The original
network/source phases interleave, so cancellation is deferred after admission
until merge/reindex really returns; close deadlines never pretend completion.
Sync counters, schedules, flags and file sets belong to AppConf. Boot workers own
separate codecs/providers and joined results; transfer workers are bounded and
join accepted IO on failure/panic. Delayed notifications and UI waits are owned.

Before sync, the original-codec owned indexer hashes source content for the
pre-overwrite backup. Other legacy second-level diff/conflict comparisons remain
pending. The five original repair stages accept Context, propagate obtainable
IO errors, drain SQL and use a per-owner successful-once marker; failure/cancel
allows retry. Partial source repair is not rolled back. Actual synthetic Local
upload/download/merge and duplicate-block repair with blocktree/SQL pass.

This supersedes the earlier controller/checkIndex-once pending notes only.
Underlying global configuration/SQL/cache/source-write/event dependencies remain
pending, as do Native source CAS/journal/receipt and handler assembly. Native
owners receive neither capability implicitly; old sync stays disabled for shared
sources. Original UI/protocol and source .docj IDs/formats are retained except
original repair behavior for duplicate IDs. R02 remains in progress.


### 2026-10-28: index-repair database and registry contexts

Compatibility repair now captures one owner/path/config and main/blocktree DB
binding. RepairQueries propagates scan/iteration/cancel/closed-DB errors, bypasses
block value caches and allowlists the original three duplicate-query tables.
The shared indexrepair core preserves original mirror fallback, old-updated and
ten-minute reconciliation rules. Failed reads never authorize cleanup. Registry
queries, path differences, removal and upsert retain captured handles, pass the
actual Context and return errors instead of logging-and-continuing.

Lease.InspectNativeIndexRepair reuses this core under one source admission and
resource gate. It diagnoses CURRENT MATERIALIZED indexes only: no queue flush,
source scan, automatic registry rebuild, correction or source write authority.
Pending work may produce candidates; execution requires revalidation. Two owners
with identical IDs remain isolated. A missing synthetic FTS table stops the real
five-stage repair before source mutation; restoring it allows the same owner to
retry and complete duplicate-ID/blocktree/SQL repair.

Original source file IO/reset/corruption moves, queue/flush error propagation,
reference refresh/delayed counts, sync merge/incReindex/indexBox and remaining
cache/event dependencies are still pending. Compatibility adapters still require
single-workspace lifecycle barriers. Native formal source writes remain R04;
no public HTTP/CLI additions. R02 remains in progress.


### 2026-10-28: reference refresh, owned counts and main-queue failures

Original backlink/repair reference grouping and doc/block count rules share
indexrepair services. Accepted main-index work drains before discovery; source
loads complete before indexing, and count targets are published only after real
flush. Compatibility requests borrow an explicit owner capability. Seven legacy
count enqueue sites now capture owner/path/DB and use tracked per-block debounce,
cancellation and actual join. Delayed reads revalidate identity before original
WS count publication. Native reference refresh uses this page's original parser,
projection, value queue, cache invalidation and flush; it returns post-commit
count snapshots and changes derived indexes only, with no delayed UI worker.

Compatibility main entries carry DB identity and cannot coalesce across owners.
FlushQueueContext admits before taking a batch, consumes confirmed commits only,
retains failed/unstarted work before newer entries, returns errors and clears
failed caches. Active legacy derived transactions defer cancellation until actual
completion. WaitFlushTxContext reports known failures; old void wrappers log.
Repair stages, restore rebuild and sync tail now consume drain failures.

This does not contextualize every original helper/config/cache/progress callback
or void caller, especially switch/exit. Queued work still requires the legacy
single-workspace lifetime barrier; an ambiguous commit/external side effect is
not a source rollback guarantee. Unadmitted asynchronous document-index tasks
are not included in this drain. Legacy LoadTree may repair old Spec; native
source writes and source repair permissions remain R04 work. No new public
HTTP/CLI, source authority or single-mode exceptions. R02 stays in progress.


### 2026-10-29 R02: explicit compatibility source repair

Index repair and compatibility backlink refresh now borrow fixed `os.Root`
handles through `filesys.TreeContext` / `indexrepair.Source`. Reader mode does
not migrate Spec, correct IDs or create parent documents. Repair requires an
explicit owner/path/DB/read-only authorization callback. Original parsing,
Spec migration, path ID correction, parent creation, formatting and empty
paragraph rules remain; malformed parent JSON now returns an error rather
than being silently replaced. Cache/registry effects are explicit adapters.

Loaded documents retain a SHA-256 revision, without retaining every parsed tree.
Writes reject a changed source and publish through a synced sibling temporary
file under the original in-process file lock. Duplicate-root reset shares the
original algorithm with explicit ID/time/language and AV binding callbacks;
AV relation storage uses the fixed source root and reports decoding/write errors.
Invalid-name archives use unique paths, sync the backup file and ancestor
directories before deleting source, and report errors. Legacy history removal
is restricted to the notebook history subtree and rejects symlinks. Every
borrowed root closes before the operation returns; no new background work.

These are trusted compatibility repair capabilities under the existing
single-workspace lifecycle barrier. They do not grant Native source writes,
provide cross-process CAS, multi-file rollback/journaling or R04 receipts.
Post-publication failures remain errors and can leave a published repair;
partial duplicate recreation requires retry/reconciliation. Other legacy
LoadTree/WriteTree/move/reset callers, sync merge/incReindex/indexBox,
configuration/cache dependencies and switch/exit error consumption remain.
R02 stays in progress; public UI, HTTP/CLI and packaging are unchanged.


### 2026-10-29 R02: owned incremental index rebuild

Sync merge and the original upsertIndexes/removeIndexes HTTP routes now share
`indexrepair.Incremental` through fixed compatibility source/registry/main-DB
bindings. The entire document path plan is validated before indexing. One
optional leading slash, input order, data-root orphan skipping and non-document
metadata skipping remain. Global/notebook assets and legacy history are not
notebook source documents. Folder expansion uses the fixed source root, rejects
traversal/symlinks and fails before indexing on an incomplete scan.

Accepted older SQL work drains first. Each run removes before upserting, with a
real SQL flush between phases so same-ID moves preserve the new path. Source
loading uses the explicit repair capability from the previous batch; source,
registry, query or queue errors no longer become successful ID lists. Failed or
canceled callbacks and panics drain accepted work with cancellation deferred;
failed SQL tails remain retryable. This is not a source transaction or rollback.
IAL invalidation now has a per-tree batched publication barrier: older buffered
admissions and deletions finish before the rebuild reports success.

Independent HTTP operations borrow a separately bound IncrementalIndex owner
capability. They serialize with sync/restore/repair, receive request cancellation
and remain joined until callbacks return. Original routes, role/read-only guards
and successful null envelopes remain; malformed arguments return the existing
error envelope. Native/metadata capabilities do not acquire this operation.
Sync invokes the shared core inside its existing admission, avoiding a recursive
gate, and does not schedule successful merge UI after an indexing error.

Full original indexBox/IndexRefs startup work, AV/history associations, conflict
copies, plugin/OCR/flashcard reload dependencies, old listSyFiles consumers and
switch/exit failure handling remain R02 work. An unadmitted async startup index
is still outside this operation's drain and can leave pre-existing duplicates;
this batch is not the full startup barrier or R04 Native source commit protocol.
No new public route/CLI, UI change or packaging change. R02 stays in progress.


### 2026-10-29 R02: notebook initialization and reference rebuild ownership

The original indexBox algorithm now runs through fixed source/registry/main-DB
bindings and an explicit `internal/notebookindex` executor. At most four workers
retain original parsing/projection/AV collection, with an independent parser per
document. The executor owns its worker task group, propagates failures/cancellation,
joins actual callbacks, and drains accepted SQL before AV binding and history
completion. Repeated initialization uses the original difference/upsert projection
so an already indexed tree is not inserted twice. Missing updated fields retain
the original repair rule; invalid filenames/hidden/assets remain skipped.

The read-only indexrepair graph contains source scanning and plans only; the
worker executor is a separate dependency of the compatibility model. No worker,
legacy global import or new dependency whitelist exception was added to the
Native read path. History initialization timestamps now belong to AppConf, with
original history consumers reading the same owner map. AV batching retains the
original mutex/msgpack/member order and deduplication, with errors returned.

Notebook index/unindex/reference/init/full operations borrow an explicit owner
capability serialized with sync/restore. Desktop startup and workspace setup
consume initialization errors. Restore/sync call the same core inside their
existing admission, avoiding recursive gates; references use rooted reads and
captured registry/SQL with actual drain failures. Notebook metadata repair captures
identity before its settings snapshot callback to avoid reentering settingsGate.
Unindex preserves source documents, reports recent-storage errors, and uses the
fixed root for recent pruning/title/order and original SQL deletion.

FullReindex keeps its original asynchronous null HTTP response but schedules one
owned parent job through the legacy serial ContextTask slot, preserving ordering
with previously accepted original tasks. The owner waits for its actual result,
including after cancellation. It no longer queues unconditional success children or exits the
process after a source error. Workers and accepted SQL/embedded-index tails finish
before release; UI reload occurs only on successful owner-valid completion.
Source/registry/AV/recent writes are not one atomic transaction; partial failure
requires retry/reconciliation. Native source commits remain R04 work. Original
conflict-copy/import/mount void callers, flashcard/OCR/plugin reload internals,
embedded/virtual-reference SQL/config dependencies and switch/exit combinations
still need their remaining contextual adapters. R02 stays in progress.


### 2026-10-29 R02: conflict preflight and actual create results

Sync conflict copies now capture owner preferences, temporary-root identity and
fixed source/registry/main-DB repair bindings. The complete document path plan,
rooted temporary reads and original read-only JSON parsing finish before the
first AV/source mutation. Missing/corrupt files, traversal, symlinks and nil
entries return errors instead of silently skipping a conflict and continuing
successful merge UI. Temporary backups remain read-only. Canonical original documents are byte-identical
in the fixtures; old finish helpers can still repair old-format parent documents.

Original resetTree services preserve Conflicted titles, IDs, internal-reference
remapping and root AV-binding removal. AV registration uses the explicit source
capability. The copy still executes original doCreate/legacy finish on the same
FIFO transaction worker. A private per-transaction result channel reports actual
callback completion, not mere queue admission. Accepted callbacks are joined
even after cancellation; queued work rechecks owner/path/DB/read-only identity
before execution and before original finish. Scoped failures/panics return a
nonfatal result and do not poison later queue work; ordinary legacy submissions
retain their previous behavior and JSON shape. Main SQL is actually drained
before moving to sort/merge success, including failed/canceled exits.

This is a trusted compatibility adapter under the existing repository lifetime
barrier, not a Native/R04 writer or commit receipt. Original commitLegacy still
uses global save/cache/UI/dynamic-ref/sync helpers; its delayed value-only UI
children retain the old task scope. The old addSort adapter is identity-guarded
but still has global read dependencies and a void error contract. Conflict history
indexing likewise remains pending. Source/AV/registry side effects can precede a
later failure; retry does not promise idempotent copies or atomic rollback.
No new public handler, UI, route, CLI or packaging. R02 stays in progress.


### 2026-10-29 R02: conflict sort and history-directory index Contexts

Conflict ordering now uses fixed source roots, captured notebook/global settings,
explicit main-DB reference counts and the original doctree sorting/projection
core. Streaming property reads retain large-document behavior without borrowing
global IAL caches or moving corrupt files. Hidden documents, folder merging,
custom/reference/name/time/size sort rules and list limits remain shared. The
original addSort value/notification loop is extracted as doctree.InsertSort;
unrelated map keys survive. A guarded revision-checked source rewrite must succeed
before the original sort notification. Parse/read/write/cancel/identity errors
return to merge and prevent history/success follow-up; source copies already
created are not rolled back.

History-directory indexing now has a pure rooted read-only plan with original
JSON parser, document/asset field layout, title fallback, asset-ID and timestamp
rules. Directories are no longer emitted as asset files. Existing malformed scans,
symlinks and invalid names return errors before any batch admission; a missing
sync history payload is an explicit no-op. The compatibility Context captures its
workspace/history root, labels, history DB/queue and notification port. Accepted
SQL finishes before release even after cancellation; notifications occur only
after successful commits and recheck owner/cancellation.

Compatibility history queues belong to DB identity; stale handles cannot enqueue
or flush into a newly selected DB. A directory reindex replaces only that path
prefix in the same transaction as original 512-row inserts, so failed-tail retry
and repeated scans do not duplicate rows or delete siblings. Original append/purge
interfaces retain their rules. Queue close/rebind drains before closing the DB,
retains failures, and leaves closed admission closed. Desktop startup/workspace
setup consume history initialization errors through the contextual initializer.
Physical replacement, once started, finishes connection/schema setup before
returning a later cancellation. Old void lifecycle/rebuild callers still need
complete error consumption.

Sync merge consumes both actual sort and history errors. The original Local sync
fixture now includes a real history DB and checks committed history before return;
a deferred enqueue is no longer accepted as evidence. No new public route/CLI,
role change, UI layout or packaging change. Native source writes/receipts remain
R04, original transaction finish/global reload/history generation dependencies and
full switch/exit recovery remain R02 work. R02 stays in progress.


### 2026-10-29 R02: original commitLegacy Context binding

The original FIFO and transaction operations now bind their commit owner before
admission. Source save, original document-info projection, captured IAL/ref caches,
seven-round anchor propagation, derived SQL drain and sync scheduling run through
explicit Context-bound services. Parent metadata reads do not migrate old source.
Errors reach the caller; accepted SQL work is joined before releasing dependencies.

Successful UI messages are immutable values owned by the captured task group,
retaining the original payloads and 500/200ms delays. Failure, cancellation, stale
owners and close suppress pending success messages. Conflict-copy notices wait
until their sort batch succeeds. Existing desktop/single UI and Native entrypoints
remain unchanged.

This remains a compatibility adapter, with no multi-file source rollback or new
Native write grant. Deep legacy AV grouping/text/relation helpers, other old void
callers, flashcard/OCR/plugin reload internals and complete switch/exit recovery
still need contextual work. R02 remains in progress; R04 owns source transactions.


### 2026-10-29 R02: deep AV commit dependencies

The original commit chain now uses an operation-local AV Context for rooted,
locked source/relations reads, detached old-format adaptation, version-checked
saves, original group rendering, block text, related/rollup values and templates.
It captures language, parser and DB dependencies without calling the old global
AV/SQL rendering facade. Source codecs and missing-field repair are shared with
the original compatibility entrypoints; UI rules and payloads remain unchanged.

Invalid source, related reads, templates and SQL queries reach commit as errors.
Success messages remain owned by the commit task group. Source publication can
still be partial on later failure; this is not an R04 multi-file transaction or a
Native write grant. Other legacy AV callers, background work and complete
switch/exit verification remain R02 work.


### 2026-10-29 R02: outer AV Context adapters and live rendering

Legacy field/option/order/binding/append/management, row and cell group/text/
related-read ports now capture an AV binding at adapter construction. Explicit
Native environments are selected before constructing compatibility dependencies.
Source-event text and related refreshes report errors and admit owned UI messages
only after success, without changing sync scheduling.

The original 100ms update refresh now runs as an owner task with copied IDs and
frozen dependencies. Close/cancellation owns the actual callback lifetime. Live
HTTP AV rendering passes request Context through group assembly, related filters,
labels, counts and near-item reads while keeping the original response shape.
History/snapshot rendering remains separate. Initialization/check/upgrade and
other legacy source mutations still require migration; R02 stays in progress.


### 2026-10-29 R02: live AV initialization and repair Context

Live rendering now uses captured-owner storage waits, rooted raw AV reads,
create-only initialization, original view-selection rules and version-checked
repair/upgrade saves. Default factories and check values are shared with the
original compatibility facade. Language/format are captured, node and Spec0 icon
reads do not migrate document source, and actual save errors reach the caller.

Corrupt, linked, invalid and future-version sources are not overwritten as if
missing. Historical/snapshot permissions and other legacy mutation adapters remain
separate work. Sequential compatibility saves can be partial on later failure;
this does not enable Native source writes or close R04. R02 remains in progress.


### 2026-10-29 R02: AV mutation source and relation Context

The compatibility configuration, field, option, view-setting, ordering and batch
append adapters now share an operation-owned raw reader, version-checked upgrade
save and group graph. Row view selection reuses its transaction binding. Warning
and reload queues are isolated from parent finish hooks and admitted only after
that operation succeeds. Existing algorithms and original UI/HTTP shapes remain.

Relation-index writes use the original lock and ordered deduplication/empty-key
rules with rooted source IO and version checks. Missing removal is a no-op;
corrupt/null/linked sources and publication failures return errors. Destination
AV save errors can no longer be overwritten by reload success. Import relation
finalization uses the same port, while import registry/binding writes remain.

Compatibility saves remain sequential: a late index failure may follow saved AV
files, and no success notice is scheduled for that failed operation. This is not
R04 atomic source commit or a cross-process receipt. Cell/row/binding/management,
name/preview/image/block-key readers, defaults/reverse, historical permissions
and remaining background/exit combinations still need work. R02 stays in progress.


### 2026-10-29 R02: AV cell, row and bound source Context

Cell, row and bound-operation compatibility adapters now use one captured source
reader, version-checked upgrade save and group graph. Node reads use rooted,
read-only document loading and the captured registry, while retaining pending
transaction edits, including new nodes not yet indexed. Names retain the original
missing-AV skip rule and return corruption/IO errors. Cell attribute and reverse
relation failures propagate through the actual adapters.

Consecutive edits invalidate prior render projections. Row defaults share the
captured template function environment and reverse relation writer. AV warning
and reload completion occurs after successful operations; batch replacement
completes once after its final source save. Original UI and HTTP shapes remain.

Mirror registry IO, original attribute/cache/broadcast writers and row history
remain guarded compatibility effects; this batch does not claim independent
Context implementations for them. Management/name and other outer adapters,
history/snapshot permissions and background exit combinations remain. Source
writes are sequential, not R04 atomic commits. R02 stays in progress.


### 2026-10-29 R02: AV management and name source Context

All 26 management actions and database naming now use captured raw reads,
version-checked upgrade saves and the same transaction candidates. View selection,
group counts, sorting time and new-layout labels/defaults use the operation's
binding. Mirror enumeration uses rooted blocks.msgpack reads under the original
lock and original set/dedup projection, including admitted pending nodes.

Name refresh preserves trimming, newline replacement, the 512-rune limit and
missing-member rules. It reuses pending document trees, publishes to the captured
cache before notification and does not introduce general attribute writes for
derived names. Known dependency IO failures propagate; AV success notices are
admitted only after the operation succeeds. Real unbound runtimes are distinct
from explicit fixtures exercising the old best-effort algorithm.

Mirror registration writes and other attribute/cache/broadcast effects are still
compatibility operations with owner guards. History, remaining outer AV readers
and background lifecycle work remain. Sequential partial saves are possible;
this does not provide R04 rollback/receipts or Native source writes. R02 continues.


### 2026-10-29 R02: AV peripheral readers and query-only rendering

Preview, image, block-key panel and identity adapters now use captured source
bindings; five HTTP handlers pass their request Context without changing their
parameters or original response/fallback shapes. Preview and image rendering
repair detached candidates only. Query-only scopes cannot save/create or modify
registries, retain this restriction across Context replacement and do not replace
an admitted transaction's document revision during metadata reads.

Image queries retain the original table projection, card-field visibility,
filter/sort and identity rules. Related-source prefetch skips genuine absence,
while corruption/IO/SQL failures propagate. Field rendering now iterates a stable
snapshot so removing a missing field cannot skip or duplicate later valid fields.

The block-key panel intentionally keeps original key-order/view/binding repairs.
It uses captured nodes, registry queries, versioned saves and rooted mirror
upsert; null registries fail without panic/overwrite. Other duplicate/import and
mirror writers, historical permissions and background lifecycles remain work.
Sequential repairs may be partial; R04 source transactions remain deferred.


### 2026-10-29 R02: actual AV duplication Context chain

Database duplication now captures the request/source owner and decodes one raw
snapshot for both its source object and full-JSON copy. Original Spec, ID text
replacement, timestamp name, one-way relation conversion and row-BlockID binding
identity remain intact. Occupied source/block IDs and mirror reservations are
checked under the original registry lock; target publication is create-only.
Corrupt/null/link errors propagate without replacing those sources.

Real binding refresh uses captured documents/revisions, source CAS, the fixed
primary DB queue and a joined drain. It keeps original attribute/cache/broadcast
and derived-name rules; document-info JSON is frozen before success-only owned
500ms scheduling. Shared import binding refresh reuses its captured binding.
Its remaining registry/archive controllers are still pending.

This preserves sequential compatibility publication, not multi-file atomicity:
reservations, target copies or document/index changes can survive later errors.
Same-process reservation is not a cross-process receipt. Original attribute
normalization, written-cache/large-file warnings and binding broadcasts remain
guarded compatibility effects. Standalone duplication is not a new performTx
action; Native authoritative writes and R04 receipts remain deferred. Other
mirror/import writers, history permissions and background lifecycles remain.


### 2026-10-29 R02: mirror ports and actual AV post-import Context

Update registration, bound-block pre-removal mirror detection/removal and
post-import batch registration now use captured source roots, the original
registry lock and version-checked writes. Preserve single-upsert changed flags,
full deduplication, raw pre-removal counts, ordered surviving duplicates and
empty keys. Removal includes admitted transaction nodes; lookup failures are
errors, not absence. Bad/null/linked registries and IO failures are not replaced.

AV post-import uses the transaction binding where present. Actual read failures
propagate; genuinely missing AVs retain their original skip. Preserve registry,
binding and last-relation projection order, with owned success notices admitted
only after all relation updates. A no-runtime legacy policy fixture now uses
explicit injection; actual unbound completion rejects even an empty batch.

The original archive extraction/copy/mounting controller and request-owner entry
capture remain pending. Legacy attribute/cache/broadcast effects, other mirror
writers, history permissions and background lifecycles are not completed here.
Sequential effects may remain after failure; no R04 rollback/cross-process receipt
or Native authoritative source writes were enabled. Public interfaces and the
original editor UI remain unchanged.


### 2026-10-29 R02: archive entry, upload and document preparation

The original import HTTP entry captures its request owner before multipart input.
Uploads use a unique owned scratch directory, never the supplied filename, with
captured temp-path checks and cleanup. The model wrapper still supports existing
three-argument calls plus optional Context. Missing/invalid destinations and
canceled admission return the original error envelope rather than panic.

ZIP members are validated before extraction; traversal/aliases, links, duplicate
members and CRC/IO failures propagate. Preserve the original single-root and
full-backup rejection rules. Document scanning/preparation uses the captured
parser/locale and retains timestamp IDs, references, block hyperlinks, embeds,
emoji filtering and old relative paths. Invalid/duplicate IDs fail before live
publication. The input archive and live source remain unchanged by preparation.

Import locks/unlocks the same captured sync runtime and rechecks cancellation
on acquisition; waiting on the old mutex is not immediately interruptible.
AV post-import reuses the entry binding; AV read/write/copy failures no longer
report success. Remaining data paths/preferences are captured with phase guards,
but resource merge, sort, mount/index, flashcard outer wrapper, IncSync and
UpdateIDs still need independently owned ports. This is only the first part of
the full archive controller, not R04 atomic publication or Native source writes.


### 2026-10-29 R02: archive resource and sort publication

Imported AV directories, assets and emojis now merge through captured source
roots and per-file version checks. Incoming files retain the original overwrite
policy, unrelated files and nested/empty directories remain. Resource discovery
matches actual assets/emojis directory components, fixing accidental substring
matches. Emoji filtering retains the original rules, handles renamed nested
directories and rejects sanitized-name collisions rather than losing a file.

Sort merging uses rooted source/destination reads, original mapped-ID projection
and preservation of existing entries. Genuine absence stays a no-op; corrupt,
null, linked or unwritable sources fail without pretending success. The scratch
sort is removed only after successful version publication. Links/nonregular IO,
old/closed/readonly owners, cancellation and stale revisions fail explicitly.

Writes remain sequential: an earlier file can survive a later error, and failed
resource directories remain in scratch until request cleanup. Notebook target
lookup, whole-directory mount/index, flashcard outer wrappers and IncSync/UpdateIDs
ownership remain pending. This is not R04 rollback/cross-process receipt or Native
authoritative source writing. Original editor UI and public interfaces are intact.


### 2026-10-29 R02: archive target, mount, index and completion

Archive target lookup uses the captured registry and original root-path query.
Missing targets and query errors now fail. Whole-archive mounting uses rooted
per-file versioned merging, retaining root/nested paths, non-document copies and
hidden-directory copy-without-index rules. Captured tree loading retains the
original repair algorithms; implicit parents/empty documents use fixed registry
and primary-index admission ports too.

A fixed-handle Index port shares the original queue coalescing implementation.
Admitted indexing is drained before return even if a later tree fails. Load,
registry and primary SQL failures propagate. Overall UpdateIDs and sync planning
only follow a successful mount/drain, using frozen JSON, captured sync preferences
and owned tasks. ID notice tasks retain owner-local order and pending chains are
cancelled/joined on close. The public updateids event remains unchanged.

Earlier AV post-import scoped information can still precede final mounting.
Sequential source/index effects are not rolled back. Flashcard outer wrappers,
remaining scratch renames/scans, other side effects/history/background work and
R04 receipts remain pending; Native authoritative writes are not enabled here.


### 2026-10-29 R02: flashcard archive outer Context

Legacy flashcard archive wrappers now use captured owner/Context, configuration
and deck snapshots under the original deck lock. Import reuses its archive entry
binding; export captures its flashcard binding at ZIP-controller entry and the
single-document HTTP handler passes request Context. Storage-sync waiting uses
the captured runtime and is cancellable. Original Native gates remain unchanged.

Builtin-only selected-tree export, learning resets, new import IDs, existing-card
preservation and bindings retain the original algorithms. Source reads/writes use
fixed roots, original file locks and per-file versions. Corruption, links, stale
versions and changed owners propagate; partial pair failure does not replace the
cache. Export clears only the builtin pair and does not change source files.

The published cached deck uses workspace lifetime rather than its completed HTTP
request and is independent of replaceable search DB handles. Review logs/save
remain supported after import and index rebuild; owner close/change or readonly
state still rejects writes. Legacy deck/round maps remain guarded compatibility
publications, not a complete migration of other flashcard callers.

Remaining scratch renames/scans and non-flashcard export controllers still need
migration. Pair writes remain sequential; R04 rollback/receipts and Native source
writes are deferred. Public envelopes, event shapes and original editor UI stay.


### 2026-10-29 R02: archive scratch controllers

AV scanning/remapping, document rewrite, directory remapping and data-bearing
storage cleanup now use captured Context and relative operations under the owned
scratch root. Scan/missing/permission/link errors propagate. Read and validate the
AV set before changing scratch files; ambiguous IDs, generated collisions and
occupied targets fail. Keep whole-JSON ID substitution and the old AV Spec.

Document serialization keeps the original renderer, inline/Spec conversion and
format preference; version writes and create-only renames preserve existing
targets. Rename directory components deepest-first, retaining unknown/prefix-like
siblings instead of rewriting raw string prefixes. Storage cleanup validates its
subtree and must succeed before mounting; it cannot follow outside links.

Scratch and prior source publication remain sequential and may be partial. Final
deferred request/extraction cleanup is still best-effort. Non-flashcard export
controllers, remaining side effects/history/background work and R04 receipts
remain pending; Native authoritative writes are not enabled. Original UI/public
interfaces remain unchanged.


### 2026-10-29 R02: archive export selection and sort

Single-document and notebook export capture Context before lookup. Notebook
metadata comes from the owned registry snapshot; notebook HTTP uses request
Context. Preserve recursive selection order and reserved/tmp filtering without
deleting old source tmp files. Missing optional child folders are empty; missing
notebooks, scan/permission/link/registry errors fail explicitly.

Selected documents use captured source/parser without repair or revision
remembering. Apply old-Spec inline conversion only to detached candidates. Sort
projection reads captured source and writes under the scratch root; absent/empty
projection removes stale output, while bad/null source fails before replacement.
Public arguments and response fallback remain unchanged.

This is not a complete read-only or consistent-snapshot export. Legacy reference
recursion may still repair sources, and raw document copies reread source bytes.
Assets/emojis, AV relations, final ZIP isolation/error handling and deferred
cleanup remain. R02 stays open; R04 receipts/rollback and Native authoritative
writes are not enabled. Original UI is unchanged.


### 2026-10-29 R02: archive reference traversal

Archive reference discovery now binds the original shared traversal to captured
registry/source/parser and an operation-local tree cache. Preserve block refs,
block links, root cycle/dedup/order rules and AV bound-document discovery, including
detached-row exclusion and the original lookup Block.ID versus recorded BlockID.
Reference documents load without repair/remember; AVs use the detached legacy
decoder without upgrade/save. Cache hits must match root/box/path identity.

Missing index entries or missing AV files remain dangling references. Existing
source/SQL/permission/link/shape errors stop archive success rather than yielding
a partial successful ZIP. Canceled/changed/closed owners fail. Other export
controllers retain compatibility adapters around the same traversal.

Raw copies still reread source bytes separately. Assets/emojis, AV relation
copying, final ZIP ownership/error handling and cleanup remain pending; this is
not a whole-export snapshot. R02 stays open, with no R04 rollback/receipt or
Native authoritative writes. Original UI and public interfaces are unchanged.


### 2026-10-29 R02: archive document bytes and scratch publication

Selected and referenced documents now share a per-export first-read cache. A
local TreeContext callback captures the exact bytes parsed into each detached
candidate; it does not publish global revisions. Old-Spec conversion remains
candidate-only. Document copies use those bytes without reopening the live file,
retaining whitespace, opaque fields and original Spec even if the source changes,
is deleted or becomes a link after the read.

Keep nested selected paths and flattened reference paths. Validate candidate
pointer/root/box/path identity; reject moved/duplicate paths for the same ID. Publish under the scratch root using the
existing version-checked atomic writer. Path/link/permission/IO failures stop
archive success. Existing first-read failure fixtures start a fresh export scope.

This pins documents independently, not one simultaneous workspace snapshot.
Resources/AV/sort and final ZIP root/error/cleanup remain pending. Raw document
bytes add per-export memory proportional to the document set. Multi-file scratch
work can still leave partial/stale files; no R04 rollback/receipt or Native
source writes. Original UI/public interfaces remain unchanged.


### 2026-10-29 R02: archive resources and related AVs

Archive asset discovery uses captured notebook order (including closed notebooks)
and the owned source root: later notebook matches win, then global assets win.
Retain nested assets, hidden/build-directory filtering on macOS, covers, rtfd
folders, custom emojis and PDF annotation sidecars. The original link collector
now accepts an AV reader; other callers retain its compatibility adapter.

Reference discovery, collection and AV export share first-read raw AV records.
Keep original Spec/opaque fields, relation recursion/cycle dedup and related-only
raw-copy policy without upgrade/save. Bound-only AVs keep the original mAsset-only
policy. Missing ordinary assets/AVs remain omitted; actual path/link/permission/IO
errors abort archive success. Implicit source symlink following is rejected.

Attachments stream through the shared 64 KiB cancellation-aware loop into
exclusive temporary files, sync/close, then atomically replace scratch output. Preserve original source file mode and
file/directory modification times.
Canceled copies preserve prior files and clean their temporary files. Keep the
original overwrite policy; this is not cross-process CAS. AV JSON uses the
existing version-checked byte writer. Retired global archive AV helpers are gone.

Documents/AVs are pinned independently and resources stream later, so this is not
a workspace-wide snapshot. Final ZIP root/error/cleanup and stale scratch lifecycle
remain pending. R02 stays open, with no R04 multi-file rollback/receipt or Native
source writes. Original UI/public interfaces remain unchanged.


### 2026-10-30 R02: private archive output and cleanup

Each archive gets an exclusive work directory outside the served export root.
Document/resource/sort and flashcard writers borrow its payload root. Check the
temp/export/work/payload directory identities; titles become safe single-segment
ZIP roots, with empty/dot names falling back to Untitled.

Stream checked members with the original Deflate, metadata and empty-directory
layout. Propagate copy/ZIP close/flush/file sync/close errors. Publish the closed
ZIP to a unique download filename using an exclusive hard link. Keep HTTP shapes,
single-export name and empty-ZIP fallback; ZIP download names gain a unique suffix.

Cleanup runs against captured roots on success, failure and cancellation. Remove
only this operation's work directory, verify ownership before rolling back its
published file, and suppress the download URL on cleanup failure. Never reuse or
sweep historical title directories/ZIPs. A replaced work entry is reported and
left intact; permission/handle failures may leave reported owned residue or an
unreturned complete artifact. No durable cleanup receipt, restart recovery, TTL
change, hostile rename CAS or R04 transaction is provided. Hard-link support is
required; unsupported filesystems fail explicitly. Import outer cleanup, other
export adapters and remaining background work stay pending. R02 remains open.


### 2026-10-30 R02: owned upload/extraction cleanup

Uploads and extraction use exclusive directories under the captured workspace
temp root. Never extract beside an external caller ZIP. Track parent/directory
identity and live owners under a mutex; borrow independent descriptors for owned
ZIP inputs, scratch helpers and prepared-document scans. Retain original ID
remapping/rendering while rejecting input file links and replaced owners.

Prepared/import-reader cleanup now joins failures with the primary error, closes
owned roots and removes only matching entries through the captured parent.
Cancellation/temp switching does not redirect cleanup; replaced directories are
reported and preserved. Repeated close returns its original in-memory result.
HTTP reader-close and multipart cleanup errors use the existing error envelope.
Register returned-Form cleanup before checking parsing errors: a query parse error
may coexist with a successfully parsed, disk-backed multipart Form.

Cleanup failure may follow partial or complete source publication, so it is not
rollback or a safe automatic-retry signal. Permission/identity/close faults can
leave reported residue. No durable receipt, restart recovery, orphan sweep or
hostile-rename CAS is provided. Multipart internals before a Form is returned
remain standard-library behavior. Other controllers/helper-close returns and
remaining background work stay pending; R02 remains open, with no R04 transaction
or Native authoritative writes. Original UI/public shapes remain unchanged.

### R02：批量Markdown导出准备（2026-10-30）


本批完成批量Markdown/Pandoc族与整本Markdown的入口Context、Export值快照和选中文档/引用树准备：11类文档转换HTTP调用及笔记本入口传Request.Context，model保留可选Context兼容签名；空选集直接返回原空字段。选集按每个ID所属笔记本枚举子文档，修正跨笔记本批量选择误用首本枚举器的问题。树与块/链接/AV引用准备复用归档只读源端口和原递归算法，首读字节/局部树缓存与取消/路径身份检查保持；原SQL子块引用计数改由捕获主库查询并传播错误。顶层标题/YFM/引用/嵌入/标注/tag/inlineMemo选项来自捕获配置，准备失败在进入旧输出阶段前返回。

边界：仅完成入口、配置与准备阶段；深层exportTree、动态引用/嵌入/AV/标注、NewLute及其他配置/源读取仍含全局依赖或旧修复行为，不能据此认定整个导出只读或隔离。资源复制、global Pandoc调用、按标题复用输出目录/ZIP及其关闭/清理错误传播仍待；单文档正文ExportMarkdownContent、HTML/PDF等其他准备控制器尚未迁移。源.docj格式不变，Markdown仅为既有导出格式。HTTP/CLI参数/返回形状、帮助/导航和资源打包布局不变，无需机械修改；R02仍doing，R03–R10 todo，未启用Native正式源写、无R04持久receipt/回滚保证。既有17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：Markdown资源、转换与输出（2026-10-30）


本批接通批量Markdown/Pandoc族和整本Markdown的资源/转换/输出阶段。每次使用捕获temp下的独占工作目录，复用归档输出身份核对、关闭并同步ZIP后独占发布、归属清理和失败抑制URL；下载文件名改为标题+唯一ID+格式扩展+.zip。Markdown ZIP仍无外层包装目录，保留文档层级、不同内容重名追加ID、Deflate及空目录行为。旧标题目录和历史ZIP保留，不再参与新导出或被覆盖。任何单文档复制/转换/打包失败结束本次导出，避免返回部分成功ZIP；HTTP仍沿用原空zip失败回退和原name行为。

资源索引改为捕获Notebook顺序及Data root读取，保留后Notebook覆盖、全局assets优先和缺失资源略过；目录资源递归复制包含隐藏项/空目录，目标为各文档相对资源目录。复制沿用有取消检查的流复制与本次scratch替换；链接/越界/权限等真实错误返回，不再吞掉。空父文档的子目录判定改为捕获root读取；资源收集使用显式AV读取端口。源root关闭错误也抑制下载URL并触发输出回收。

Pandoc执行不再调用旧util.Pandoc：入口捕获已选择的设备工具路径及容器能力，转换时使用显式命令和请求Context，输入文件限于本次私有工作目录；保留原from/to/resource-path/-s/-o参数，取消可终止子进程并限制管道等待。不需要转换时直接写本次payload文件；成功必须实际产生普通文件并完成同步/关闭，输出与清理错误返回。工具stdout/stderr不归档正文。设备工具初始化/安装/运行时选择仍是原全局兼容桥，尚未宣称完整Workspace设备能力装配；本批不安装Pandoc、不新增外部依赖。

边界：深层exportTree、动态引用/嵌入/AV/标注、NewLute及其余配置/源读取仍含全局依赖或旧修复行为；整个导出尚不能宣称只读、全源一致性快照或完整隔离。尚待单文档正文ExportMarkdownContent、HTML/PDF等其他控制器和R02其他副作用/后台链路。本批正常流/错误流的临时文件归属不等于R04持久receipt、进程骤停恢复或恶意跨进程改名CAS；极端身份/权限/句柄错误仍可能有已报告残留。源.docj格式不变，Native正式源写未启用；R02仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：Markdown块引用渲染（2026-10-30）


本批接通批量Markdown/Pandoc族及整本Markdown的块引用锚文本、mode4路径/锚点定位和blockLink2Ref递归。复用原导出函数与遍历/转换算法，通过可选的单次渲染适配器提供捕获registry、主SQL、只读树准备及错误返回；没有另写一套渲染UI。无适配器的HTML/PDF/单正文旧调用继续原路径。每个渲染适配器保留首个错误，SQL/registry/源读取/缓存身份失败不再当成缺失引用或回退进程全局资源；拥有Context的Markdown入口将失败传至外层，不发布ZIP。

入口在同一设置快照中捕获Editor的锚文本最大长度和ParserOptions，复制emoji映射；exportTree入口和Markdown正文渲染器使用捕获的Editor parser，动态锚文本使用捕获配置的Index parser及原indexdb.ReferenceText算法。保留静态锚文本、缺失SQL定义的原“block not found”文本、search标记清理、HTML反转义、截断与左右装饰，保留mode2协议链接/mode3文本/mode4相对文件链接和块哈希规则。递归保留原4096深度边界、标题子块遍历及本次树缓存；源只读端口保留首读字节/旧Spec兼容，验证rootID/Box/Path一致性。

边界：原渲染器会修改本次候选树，本批不宣称候选不可变或整个导出全源只读/一致性快照。resolveEmbedR及其内部查询/渲染、文件标注、AV表格/过滤/汇总、标题getBlock、widget/CSS和其他parser构造/配置仍有全局依赖；singleFile脚注定义收集/解析及HTML/PDF/单正文调用尚未完整接入。本批接入的批量路径使用singleFile=false，不据此宣称单文件脚注完成。设备工具初始化兼容桥及R02其他导入/镜像writer/缓存/广播/历史/后台仍待。源.docj格式不变、Native正式源写未启用，无R04持久receipt/回滚或恶意跨进程改名CAS保证。R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：Markdown嵌入渲染（2026-10-30）


本批完成批量Markdown/Pandoc族及整本Markdown的resolveEmbedR嵌入渲染Context接入。通过既有单次渲染适配器传递捕获的主SQL、registry/源树读取、Search.Limit、Editor.HeadingEmbedMode和parser；继续使用原解析查询/选集顺序、原文/引述、标题0/1/2与custom-heading-mode覆盖、折叠子标题过滤、层级调整/六级上限、属性转移及resolved/七层深度边界。未提供适配器的旧HTML/PDF/单正文调用继续原兼容路径。

SelectReadBlockRows复用原SelectBlockRows算法，并在输入及实际执行的重写SQL上验证单条只读SELECT；保留原分页、显式limit及CTE等原raw分支，不在此重写其历史分页语义。SQL/registry/源读取、命中但缺节点、缺脚本及格式化错误保留为首个渲染错误，返回拥有Context的入口并抑制ZIP，停止把真实故障降为空嵌入。只读校验拒绝写语句/多语句；本批不新增数据库写权限。

嵌入会调整标题或格式化节点，因此每次使用都从已准入的首读.docj字节重新ParseJSONWithoutFix，复制定位信息，旧Spec仅在候选内转换。候选独立，避免重复嵌入累积改变层级或污染后续渲染；源字节、未知字段和外部后续版本不会被回写。此机制是请求内首读文档快照，不是SQL与整个源仓同时一致性快照；外层源root关闭返回值审计仍按原待办保留。

边界：renderBlockMarkdownR及单文件脚注定义中的另一条嵌入查询/渲染调用链尚未接入；本批批量路径singleFile=false。AV表格/过滤/排序/汇总、文件标注、标题getBlock、widget/CSS/其他parser、设备工具初始化兼容桥及其他导出控制器和R02其他副作用/历史/后台仍待；不能宣称整个导出全源只读或完整隔离。源.docj格式不变，Native正式源写未启用，无R04持久receipt/回滚/进程骤停恢复或恶意跨进程改名CAS保证。R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：Markdown文件标注（2026-10-30）


本批完成批量Markdown/Pandoc族及整本Markdown的文件标注读取和链接转换Context接入。沿用原两种链接AST/页码展示：mode0为“文件名 - p页码 - 锚文本”，其余模式仅锚文本，跳转仍为资产路径#page=N。正常原生“文件名-22位节点ID.扩展名”保持原展示名，短/非标准文件名安全保留，不再做可能越界的切片。未传适配器的旧HTML/PDF/单正文调用仍走原兼容读取分支，只有链接AST构造提取为共享纯函数。

标注资产查找使用捕获的Data root和Notebook顺序：全局文件优先，否则按各Notebook词法遍历的首个后缀匹配，保持原GetAssetAbsPath的选择顺序。源和.sya使用root相对检查，拒绝链接/越界/非普通文件，固定路径的进程内锁保护侧车读取，关闭错误合并返回。请求内缓存整份首次成功读取的.sya记录，不同annotation ID不会混读后续文件版本；返回缓存前仍检查取消/路径归属。只校验实际引用的记录，其他扩展字段保留在源中，不要求全部记录同构。

.sya读取/JSON/缺记录/空pages/缺index/类型错误/负数/非整数/溢出等均明确失败，不再靠强制断言或把故障当成可忽略引用；页码仍由0基index加1。首错返回拥有Context的Markdown入口，失败抑制ZIP并沿用私有输出清理。Markdown资源复制去掉导航fragment后定位文件，修复#page=造成PDF漏包；标注生成的链接按实际选中资产复制，避免标注按首Notebook读取而普通资源索引按末Notebook覆盖造成不一致。未被标注选中的普通资源保留原索引规则；不额外复制.sya到Markdown ZIP。

边界：标注缓存只是请求内首读侧车快照，不等于PDF、文档、SQL与全仓原子一致性快照。AV表格/过滤/排序/汇总、标题getBlock、widget/CSS/其他parser、renderBlockMarkdownR/单文件脚注、设备工具初始化和其他导出控制器/R02其他副作用/历史/后台仍待；不能宣称整个导出只读或完整隔离。旧HTML/PDF/单正文标注的全局读取及异常结构处理仍需迁移。源.docj/.sya格式及未知字段不改，Native正式源写未启用，无R04持久receipt/回滚/进程骤停恢复或恶意跨进程改名CAS保证。R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：Markdown标题与挂件parser（2026-10-30）


本批完成批量Markdown/Pandoc族及整本Markdown的文档标题投影，以及挂件Markdown/打包资源收集解析器的Context接入。标题不再走getBlock→全局SQL投影parser/fromSQLBlock→全局Search配置的兼容入口，改为捕获候选身份检查、ProjectDocumentBlock原根节点分支及原blockview.FromRow规则。Search.CaseSensitive与parser使用入口快照；保留原HTML实体处理、零宽清理、5120字符UI截断及IAL转为标题属性的逻辑。标题开关和插入一级标题的原算法不变。

ProjectDocumentBlock只对独立根节点/IAL副本调用原buildBlockFromNode的Document分支，不遍历正文、查询SQL/registry、读取资产/AV或启动OCR。复用原投影而非重新拼写标题语义，避免NodeHash排序改变调用者IAL；返回的Block/IAL独立。上下文、树/根身份、节点类型、IAL结构和投影失败返回错误，首错抑制ZIP，并在标题阶段前后停止后续渲染。此处仅消除标题读取自身的隐式依赖，外层导出仍会使用捕获SQL/registry。

挂件data-export-md仍先HTML反转义、启用YFM并替换为原解析节点；使用与原util.NewLute同类型的nativeparser.NewLute，配置改为捕获Markdown值，不替换成Editor parser。打包前重新解析正文收集资源也用同一捕获值。未提供适配器的HTML/PDF/单正文等旧调用仍保留getBlock及util parser兼容路径，不据此宣称其已上下文化。

边界：AV表格/过滤/排序/汇总、CSS主题资源、聚焦导出的标题级别辅助读取、renderBlockMarkdownR/单文件脚注、设备工具初始化、其他导出控制器及R02其他IO副作用/历史/后台仍待。整个导出尚不能宣称全源只读或完整隔离；本批根投影无正文/SQL依赖不等于整个导出无这些依赖。源.docj格式和未知字段不变，Native正式源写未启用，无R04持久receipt/回滚/进程骤停恢复或恶意跨进程改名CAS保证。R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：脚注与递归Markdown中层（2026-10-30）


本批完成collectFootnotesDefs/collectFootnotesDefs0、resolveFootnotesDefs与renderBlockMarkdownR的可选Context中层适配，复用已有捕获registry/主SQL、只读查询、解析配置与独立首读候选端口。保持原脚注编号/去重/本页锚点排除、静态锚文本及最大长度、标题/文档/列表节点展开、同文档段落64字符缩略、HTML/换行解码、rendered集合循环去重及收集器4096深度边界。没有另写脚注渲染算法。

脚注定义保留原loadTreeWithCache的“请求ID”缓存规则：请求块ID未命中时，不复用已经被blockLink2Ref转换的根ID候选，而从已准入的源字节创建独立树，并缓存到该块ID下。这样保持原链接转换时机；根或块缓存均校验定位身份。递归Markdown每次源加载同样借用独立首读候选，避免移动节点污染下一次读取。源、registry、查询、缺节点/脚本、取消和路径切换错误保留首错；拥有适配器的失败不返回部分脚注块或部分节点列表。

重要状态：批量ZIP仍使用singleFile=false；单正文/标准Markdown HTTP控制器仍使用原无适配器兼容路径。本批验收的是中层显式端口和拥有Context的singleFile内部链路，并未宣称这些HTTP入口已经完成迁移。控制器的聚焦准备、CSS、订阅/云资源等依赖仍须后续统一装配；不能为了接通脚注而在未满足依赖时回退全局读取或丢弃原请求选项。原无适配器路径保留旧读取/错误兼容行为。

边界：AV表格/过滤/排序/汇总、CSS主题资源、聚焦辅助、设备工具初始化、其他导出控制器和R02其余IO副作用/历史/后台仍待。候选树仍会被原渲染算法修改，首读文档候选不是SQL与全仓同时一致性快照；不宣称整个导出只读/完整隔离。源.docj及未知字段不改，Native正式源写未启用，无R04持久receipt/回滚/骤停恢复或恶意跨进程改名CAS保证。R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：聚焦准备与标题级别（2026-10-30）


本批完成聚焦准备与标题级别调整的Context中层端口。selectExportTree提取原prepareExportTree的选择算法，保留整篇、标题及其后代、单块的范围和末尾原占位节点；旧入口仍使用原全局loader/parser与原临时根行为。新prepareFocusedExport使用捕获registry/独立首读候选/parser，验证命中节点后再执行共享选择逻辑，错误返回而不回退全局。

拥有Context的候选将Tree.ID和临时Root.ID统一为源根ID，并补齐Root的Box/Path，避免新读者把临时根当作源根缺失；源文件ID不变。只登记由本次准备端口生成的候选，记录根指针和定位信息；任意同ID克隆、根替换或ID/Box/Path/HPath/类型变化不能凭字符串相同进入标题投影。登记的候选可以使用已有的标题端口，输出属性仍独立；调用者拿到的定位结构也是副本。

引用/脚注读取现在在拥有Context时保留请求块ID，以识别聚焦范围外的同文档定义。范围内读取复用聚焦候选；范围外读取独立完整源候选，不覆盖当前聚焦范围。标题级别调整共享原算法，读取捕获的定位及本次渲染实际使用的addTitle参数：无标题时首标题提升一级、有文档标题时提升二级、整篇不调整、保持原直接标题/层级传播与1–6边界。拥有Context的空树安全不处理；旧入口继续原配置路径。

状态边界：单正文/标准Markdown HTTP入口仍未装配这些端口，CSS/订阅云资源和完整控制器准备仍待；本批验收的是拥有Context的准备/内部聚焦渲染及原HTTP范围/级别兼容。AV表格/过滤/排序/汇总、设备工具初始化、其他导出/旧标注兼容分支和R02其余副作用/历史/后台仍待；不宣称整个导出只读或完全隔离。候选会被原渲染算法修改，请求首读源候选不是SQL与全仓原子快照。源.docj和未知字段不变，Native正式源写未启用，无R04持久receipt/回滚/骤停恢复或恶意跨进程改名CAS保证。R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：Markdown主题变量Context（2026-10-30）


本批完成Markdown渲染的CSS变量Context端口。捕获导出所属AppConf的外观模式/主题选择，以及入口当时的设备主题资源目录；只复制标量，不保留可变外观配置指针。主题资源仍是设备资源，不纳入wiki源数据。拥有Context的内部渲染通过显式theme端口读取，不在替换时访问当前Conf或ThemesPath；批量ZIP原有fillCSSVar=false行为不变。

提取并复用原主题解析和IAL替换算法，保留daylight/midnight基础主题与自定义主题拼接顺序、原粗略模式选择器优先级、同优先级后者覆盖、CSS解析器双连字符兼容、嵌套变量解析/循环与深度限制及未命中var()回退。缺默认CSS仍允许选中主题继续，缺选中CSS仍得到空映射；其他实际IO错误在拥有Context的路径上传播，渲染失败不返回部分正文。

拥有Context的读取验证绝对资源根与单级主题名，借用os.Root和既有Source只读端口，拒绝穿越与根/目录/文件符号链接，返回关闭错误；首用后缓存本次解析值，不重复追随主题文件变化。验证候选归属和Context存活，缺配置/缺端口/取消/关闭不回退全局。旧无端口入口仍保留原日志、路径/符号链接和缺文件兼容行为；严格读取策略只适用于新端口，尚未成为HTTP主题导出的新默认。单根文件检查不构成恶意跨进程改名CAS，跨主题文件读取也不是原子快照。

边界：单正文/标准Markdown HTTP入口仍未装配已有聚焦、脚注、标题和CSS端口；原UI保留，当前验收是内部Context端口及原入口兼容回归。AV表格/过滤/排序/汇总、订阅云资源和完整控制器依赖、设备工具初始化、其他导出/旧标注与其余R02副作用/历史/后台仍待。源.docj不改格式、不写回，Native正式源写未启用，无R04持久receipt/回滚/骤停恢复。R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：Markdown AV表格Context（2026-10-30）


本批将exportTree内的AV表格转换接入显式attributeTable端口。无端口时仍执行原文件存在检查、ParseAttributeView/getAttrViewTable/GetFurtherCollections/Filter/Sort和原日志/跳过规则；拥有Context时使用本次导出的只读AV scope，原表格AST生成和所有字段显示代码保持共用。表格/卡片/看板均遵循原表格投影：表格保留列隐藏/换行属性，卡片与看板按原ID-only字段投影；保留视图过滤、排序及不受UI分页限制的导出行集合。

新scope复用已完成的commitAVContext与avrender，而非另写渲染器。两个可选只读源端口复用导出已捕获的AV原字节和独立完整文档候选；默认事务/查询scope不设置这些端口，原行为不变。AV相关读取/模板SQL/块属性使用所属Context的reader、parser、SQL、语言与时钟，旧格式仅在内存升级；首读原字节与依赖缓存不因后续源变化被重定向。每次表格转换重建可变候选图，防止上一次自动填值/布局投影/排序污染下一次输出，也不修改归档准备保存的原候选。临时表格缓存不冒充卡片/看板布局缓存。

只读scope禁止save，源端口禁止借给可写scope；缺失AV仍作为悬空块跳过。已有文件坏JSON、身份错配、非法布局/字段、符号链接、依赖失败、取消或所属Context关闭均向上返回，不回退全局，不发布部分Markdown/ZIP，不覆盖既有产物并清理临时工作目录。原单正文/标准Markdown HTTP入口仍未装配Context端口；本批实际装配的是原有带Context的批量Markdown路径及内部渲染。CSV/HTML等其他导出控制器仍有单独迁移工作，不能以本批推断全导出已隔离。

边界：本次读取仍非SQL/文件/全仓原子快照，不新增跨进程CAS或R04持久receipt/回滚/骤停恢复；既有借用root关闭返回值等独立待办保留。源.docj/AV JSON格式不变、正式源写未启用、原UI保留。R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。后续优先补订阅云资源与完整控制器依赖，迁移单正文/标准Markdown HTTP入口，再处理其他导出、设备工具、历史/副作用/后台收尾；复杂跨库递归/循环和其他控制器需各自验收。

### R02：单正文与标准Markdown入口Context（2026-10-30）


本批实际装配单正文exportMdContent与标准Markdown copyStdMarkdown的Context调用链，覆盖上一批“HTTP入口仍待”的状态。新增错误返回的ExportMarkdownContentContext/ExportStdMarkdownContext与类型化选项，HTTP传递Request.Context；省略refMode/embedMode时由所属AppConf导出快照决定，显式0仍保留，处理器不再为这些默认值读取model.Conf。原公共模型函数保留签名并封装新入口，失败返回原空字符串兼容值；成功响应仍分别为hPath/content对象与字符串，不新增UI或HTTP字段。

入口用已有独立聚焦准备、稳定源根身份、引用/脚注/嵌入/标题/CSS/AV端口执行原Markdown算法。保留整篇/单块/标题后代范围、标题级别、YFM（单正文默认true、标准入口无YFM）、资源空格替换/imgTag参数与原字段格式。标准入口refMode4仍按原遍历顺序收集存在的定义ID并去重，随后复用原脚注/锚点逻辑。单正文不会无意启用订阅云资源前缀。

订阅状态使用既有AppData.IsSubscriber规则（终身或正期限且状态0），只捕获当前所属配置的区域与用户ID形成云资源基址，不保留User指针或token。GetCloudAssetsServerForRegion提取原区域选择算法，原全局helper仍兼容；标准入口把捕获的基址交给原renderer，后续全局区域/账户变化不重定向本次导出。此过程只读本地配置，不发云端请求。

Context/配置/依赖错误向上传递且不返回部分hPath/正文；HTTP异常返回code=-1和空data，合法但缺失块保持原成功空结果兼容。请求取消会在配置捕获/源读取/渲染守卫处停止，不再从处理器降级为Background；未改动的旧直接调用wrapper仍显式使用Background。源.docj/AV JSON不改格式、不写回，Native正式源写未启用；本批仍是现有兼容runtime的Context装配，不代表Native多workspace路由或R04原子提交/receipt/恢复已经启用。请求首读和配置捕获不等于SQL/文件/全仓原子快照。

R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。单正文与标准Markdown的这部分HTTP装配不再整体列todo；后续转向CSV/HTML/PDF等其他导出控制器、设备工具与旧标注兼容分支，继续其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限及后台完整退出。真实云账户/外部网络和复杂跨库循环需单独验收，不能由合成订阅测试推断已验证。

### R02：AV CSV导出Context（2026-10-30）


本批完成AV CSV导出控制器的Context接入。HTTP exportAttributeView传递Request.Context到新增ExportAv2CSVContext；原ExportAv2CSV签名保留，封装Background兼容调用。复用已验证的导出Context和只读AV表格能力，节点从所属registry/文档首读候选解析，校验选中块确为请求的AV，按该块保存的viewID选择视图；不再在控制器中调用全局getNodeByBlockID、ParseAttributeView或SQL渲染/过滤/排序。

复用原表格/卡片/看板投影和完整过滤排序，CSV仍导出所有匹配行及全部列（含隐藏列），不跟随UI分页。writeAttributeViewCSV提取原字段显示代码：UTF-8 BOM、标准CSV转义、日期/创建/更新时间、图片/文件链接、其他资源、逐行行号与Value.String回退保持；日期等修改仅作用于可变候选。数据库名使用本次捕获的未命名语言值，源.docj/AV JSON不改格式、不写回。

输出复用archiveExportOutput的独立工作目录、关闭/同步后打包、完整ZIP才发布及清理。增加可选的受约束发布子目录，默认归档/Markdown行为不变；CSV保留/export/csv/下载路径、.db.zip后缀及单个平铺“数据库名.csv”成员，外部ZIP文件名增加独立ID避免同名覆盖，不再清理旧同名文件夹。子目录拒绝符号链接并校验目录身份，已发布输出不能重定向子目录；碰撞、目录替换或外来文件替换不覆盖/误删无归属文件。单文件发布不是全仓原子快照，也没有恶意跨进程改名CAS或R04持久恢复保证。

补齐CSV Flush/Error、短写、文件Sync/Close、打包/最终关闭错误返回，以及逐行取消检查；失败不返回下载URL，既有HTTP code=1、closeTimeout=7000错误结构保持。缺源、坏源、块/AV不匹配、非法ID、源/目标符号链接与Context关闭均失败，不回退全局。此为现有兼容runtime的Context装配，不能视作Native多workspace路由或正式源写已启用。

R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。CSV控制器不再整体列todo；下一步为HTML/PDF等其他导出控制器、设备工具/旧标注依赖，之后处理其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限和后台退出切换。真实Excel、复杂跨库递归、外部Pandoc/PDF设备能力需独立验收，不能由CSV合成样例推断。

### R02：HTML片段预览Context（2026-10-30）


本批完成/api/export/preview的HTML片段预览Context装配。HTTP传递Request.Context到PreviewContext；原Preview签名保留Background兼容wrapper。源候选由既有独立聚焦准备和稳定源身份端口产生，原exportTree使用本次导出的引用/动态文本/嵌入查询/脚注/标题/AV端口和捕获配置，不读取当前全局Export/Appearance。预览仍按原行为固定#标签包裹、过滤隐藏列、处理标题级别和完整引用内容；AV卡片/看板保留原ID-only表格投影规则。

renderHTMLPreview保留原内存转换流水线：强制行内语法、启用脚注、添加块IAL、调整聚焦标题、删除超级块外侧IAL、为脚注后括号文本插入零宽空格，然后Markdown格式化/重新解析/ProtylePreview，关闭远程图片标记。该格式化步骤直接调用原lute.FormatNodeSync并返回错误，避免通过原FormatNode的fatal日志路径退出；全库FormatNode未修改。

CSS变量在这次格式化派生的局部树上应用本次捕获的主题值，复用原主题合并/解析/回退算法及受约束的主题资源读取，不放宽通用源树主题端口的身份检查。主题首读缓存和配置标量保持独立；后续全局配置或主题文件变化不重定向同一次预览。源码候选与派生树均不写回.docj/AV JSON，不创建导出产物或临时打包目录。

HTTP成功仍返回html/fillCSSVar字段，原User-Agent判断保持：Chrome等支持环境保留变量，Safari等需替换的环境读取主题值。合法但缺失块仍返回空html和原fillCSSVar值；非缺块的Context/配置/依赖错误返回code=-1且无部分data。取消在准备、遍历、格式化前后和最终渲染后检查，不降级为全局服务。此为现有兼容runtime的Context端口，Native多workspace路由和正式源写仍未启用；请求首读与配置捕获不是全仓原子快照，无新增R04持久receipt/回滚/骤停恢复保证。

R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。/api/export/exportPreviewHTML的PDF/图片预渲染、完整HTML/MarkdownHTML文件导出、合并子文档、设备字体/脚本/图标资源复制及PDF/DOCX工具链仍待独立迁移。下一步从这些文档准备和资源依赖继续，之后推进其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限与后台退出切换；HTML片段预览不再整体列todo。

### R02：导出文档准备与子文档合并Context（2026-10-30）


本批完成HTML/PDF/DOCX共用的文档准备与子文档合并Context中层端口。prepareHTMLDocument复用已有聚焦准备，返回本次独立树/选中节点/定位；merge开启时由显式guard/list/load服务完成目录枚举和子文档读取，缺端口或错误不回退全局，失败不向调用者交付部分树/节点/定位。完整文件导出控制器尚未装配该端口，设备资源/工具依赖仍待；这与上一批已完成的/api/export/preview片段预览是不同范围。

原mergeSubDocs/buildBlockChildren/walkBlock/loadTreeNodes保留无端口兼容路径，新增可选Context服务，复用原插入与标题算法：按目录排序后的深度优先顺序展开，逐文档合成标题、标题ID沿用源文档ID、深度超过6时标题限制为6，不调整原正文标题；保留空父文档插入点及末尾空段落规则。修复walkBlock对loadTreeNodes错误漏赋给返回值的问题，旧路径也不再吞掉这一加载错误；上层完整文件控制器的错误/产物处理仍需各自迁移验收。

拥有Context的目录服务复用doctree.List的隐藏/笔记本优先排序/全局排序/自定义sort/每目录MaxListCount/引用计数/子文档数规则，捕获FileTree标量；笔记本配置、排序文件和计数从所属源与SQL读取。展示用的人类化时间字符串不参与此内部列表，仅原元数据排序字段用于合并。缺配置文件使用原默认值，不创建配置；坏配置/坏文档直接错误，不搬移损坏文档，也不清理过期.tmp。目录、文件和元数据路径限制在选定笔记本，拒绝穿越/符号链接与关闭后的读取，受借用root关闭错误约束。

统一抽取exportTreeAt，从本次已记录原字节重新解码独立候选；嵌入引用继续先做原registry定位核验，目录发现的未入索引子文档可按受约束路径读取，不要求先写索引。Properties身份与原根身份校验，旧Spec适配仅在候选中进行；排序元数据使用原字节与源属性，不复用全局文档属性缓存。合并后的父根保留既有来源绑定，标题投影可用，子文档候选的拼接不修改保存的原树或.docj/未知字段。目录/配置/SQL/文档首读并非全仓原子快照，无跨进程改名CAS或R04持久receipt/回滚/骤停恢复保证；Native正式源写/多workspace路由未因此启用。

R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。文档合并中层不再整体列todo；下一步补HTML/MarkdownHTML文件及PDF/图片预渲染的脚本、字体、主题、图标、表情、附件和输出能力，再实际装配文档准备端口。之后继续其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限和后台退出切换。

### R02：HTML设备资源与附件复制Context（2026-10-30）


本批完成HTML/MarkdownHTML及PDF附件阶段共用的资源复制Context中层端口。copyHTMLResources只接受本次已登记的源候选与本次archiveExportOutput私有payload，拒绝外来克隆/输出；附件继续复用已有显式AV读取、资源索引/全局优先及标注选中文件映射、URL查询/片段处理、rtfd目录/隐藏文件/空目录复制。static=false只复制附件，沿用PDF等无需打包程序静态目录的分支；static=true增加原HTML/MarkdownHTML的程序资源和表情。端口不发布ZIP，由后续完整控制器在全部步骤成功后发布。

捕获所属Appearance模式/主题名/图标名、WorkingDir与AppearancePath设备根，以及原生emoji静态表；配置标量与目录选择不跟随之后的全局变化。保留原主题根别名支持：显式配置的设备根/选定主题或图标根在捕获时解析为物理目录，记录目录身份，复制前重新校验；这仅授权选定根，不代表递归跟随内部链接。主题/图标名单级路径校验，目录内部文件/目录及目标链接仍由Source约束拒绝，已被替换的根失败。捕获仅冻结选择/目录身份，不冻结资源文件字节，不构成跨文件或SQL/文件原子快照。

复制布局保留stage/build/export、stage/protyle（含字体）、选中主题全目录、当前图标icon.js与自定义图标的material后备，图标其他文件不额外复制。空图标/ant/material遵循原后备规则；缺失的可选图标/表情保持省略，真实IO/权限/越界错误向上传递。表情路径在所属data/emojis内，保留原文档图标/NodeEmojiImg及去重规则，支持原生中文文件名和编码URL；坏引号或编码穿越返回错误，避免原字符串截取可能的panic。原生/远程/API图标不当作本地表情读取，不新增网络请求。

资源流复用既有有界复制、源/目标检查、文件模式/时间及私有临时文件替换，借用设备/数据root的关闭错误加入返回值。必需程序目录/主题缺失失败，部分资源仅留在私有工作目录，失败不发布；Wiki源.docj/附件/表情及设备资源均不写回。旧无端口ExportHTML/ExportMarkdownHTML控制器保持原路径，新端口与其在合成资源夹具下做布局/字节对照，不能据此宣称完整HTTP控制器已隔离或真实发布资源已验收。

R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。资源复制中层不再整体列todo；下一步为完整HTML/MarkdownHTML及PDF/图片预渲染的原渲染/输出/错误返回与设备工具依赖，实际装配已有文档准备、合并和资源端口。之后继续其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限和后台退出切换。Native正式源写/多workspace路由、R04持久receipt/原子恢复、真实PDF/DOCX工具及实际应用资源全量发布不在本批新增验收范围。

### R02：PDF/图片预渲染HTTP Context（2026-10-30）


本批将原/api/export/exportPreviewHTML实际控制器接入请求Context及所属导出读取端口，保留id/image/keepFold/merge参数和id/name/content/attrs/type响应字段。该接口为PDF/图片提供预渲染DOM，不保存文件；image仍沿用原PDF式DOM处理，不新增图片渲染行为。不存在的有效块保留空结果，取消、依赖失败及坏iframe引号返回错误而非部分成功内容。

复用前批所属文档准备/合并，串联原PDF目录锚点、exportTree引用/AV渲染、标题层级调整、iframe转链接及ProtyleExportRenderer。直接位于blockquote下的标题沿用原规则不生成目录锚点，keepFold/合并/焦点范围及原始HTML组件保持原行为。attrs/type在iframe转换后读取，与原接口一致；不把转换前的NodeIFrame类型返回给原UI。捕获本机链接前缀，模型内重写本机/#锚点，控制器不再在请求末尾读取全局端口；配置/源树/解析器/查询依赖继续通过已绑定Context端口读取。

目录标记、iframe转换和最终HTML renderer抽为原文件导出及新Context共用函数，原无端口文件入口仍保留兼容包装。新端口循环与渲染前后检查Context，失败不返回部分DOM；缺少iframe src保留原文本回退，src引号不闭合改为明确错误，避免原字符串截取panic。该只读路径不写回.docj/AV/附件，不创建导出文件或调用设备工具。捕获不构成跨文件/SQL原子快照。

R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。PDF/图片预渲染HTTP装配不再整体列todo；完整HTML/MarkdownHTML文件导出仍需串联已有文档/资源/私有输出端口并传播错误，ProcessPDF后处理及Pandoc等设备工具仍待。其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限和后台退出切换继续后续串行推进。此处仍为原兼容运行时接入，不代表Native正式源写、多workspace路由或R04持久receipt/原子恢复已完成。

### R02：HTML/MarkdownHTML文件模型装配（2026-10-30）


本批完成HTML/MarkdownHTML文件导出的Context模型装配端口withHTMLFile：按原分支选择所属文档/合并/焦点、HTML或MarkdownHTML renderer，再复制原静态设备资源/主题/图标/表情与所属附件到本次私有payload，最后调用受生命周期约束的消费者。PDF预备分支仅复制附件，MarkdownHTML/Word预备分支保留原静态布局。调用者不能在此传入任意savePath；借用输出仅在消费者内存活，端口统一关闭与清理。

新增renderMarkdownHTMLDocument，将原exportTree引用/AV配置、文档名、可选Word iframe转换及MarkdownHTML renderer接入所属Context。MarkdownHTML沿用原ProtylePreview，Word预备DOM沿用ProtyleExportDocxRenderer；不套用ExportHTML的标题层级调整。原表情/emojis相对路径、列表起始值/首条编号及行内code后零宽空格处理抽为共享函数，原ExportMarkdownHTML亦调用该函数，避免分叉；其文件资源/保存路径控制器仍为旧分支。新路径读取捕获配置和解析器，检查树归属，查询/循环/渲染失败不向消费者交付部分内容。

私有输出复用已有archiveExportOutput：渲染或资源复制失败不调用消费者；消费者失败、取消、panic或最终Context检查失败时清理本次工作目录和已发布ZIP；只有消费者正常返回且最终检查成功，才允许保留它已发布的ZIP。正常仅消费DOM/资源而未发布时也清理私有目录；关闭错误加入返回值，借用输出结束后拒绝再次使用。此处未新增跨请求receipt或原子快照承诺。

原UI的文件导出仍为后端准备资源/DOM、前端补齐HTML，再由浏览器exportBrowserHTML打包或桌面前端写index.html的两阶段流程。withHTMLFile是完整模型装配端口，尚未装配exportHTML/exportMdHTML/exportBrowserHTML的跨请求目录交接、桌面保存路径授权及完整错误返回；不宣称三个HTTP控制器已经隔离。PDF后处理/实际Pandoc调用、真正Word文件生成和实际应用完整静态资源发布亦未在此新增验收。

R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。HTML/MarkdownHTML完整模型装配与MarkdownHTML渲染端口不再整体列todo；下一步为保留原UI两阶段协议，接入临时资源目录的交接/归属/关闭、原browser打包回调及桌面savePath控制器，再推进ProcessPDF/设备工具、其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限和后台退出切换。此处仍为原兼容运行时端口，不代表Native正式源写、多workspace路由或R04持久receipt/原子恢复已完成。

### R02：浏览器HTML两阶段导出Context（2026-10-30）


本批将exportHTML/exportMdHTML的空savePath分支及exportBrowserHTML实际HTTP控制器接入请求Context，保留原id/name/content/folder和zip响应、前端补齐HTML后回调打包的两阶段UI协议。HTML保留pdf/keepFold/merge语义，MarkdownHTML沿用原控制器不合并的行为；桌面非空savePath仍为旧分支，本批不宣称已隔离。准备阶段调用前批withHTMLFile完成所属渲染/资源复制/私有输出，再将资源复制到所属temp/export内的独立目录，成功关闭和最终检查后才返回folder；失败返回code=-1且无部分成功Data。

每个AppConf拥有私有handoff表，记录temp/export/资源目录的身份和创建时间，不持有跨请求os.Root或请求Context。后续回调重新捕获所属读取端口，只接受该实例登记的单级folder；不存在记录、路径穿越、外来目录、已替换根/目录与内部链接均拒绝，不通过文件系统中“碰巧存在”获取权限。目录名保留html-/htmlmd-前缀但改用唯一ID；一条记录最多消费一次，实例内互斥避免并发回调重复打包。只有目录身份被记录，资源内容不是不可变字节快照。

回调将资源有界复制到本次私有payload，只在私有副本写index.html，再用既有检查后的关闭文件/硬链接ZIP发布；保留原平铺ZIP布局和URL字段，文件名改为唯一值，不覆盖同名已有包。回调结束前关闭借用root、清理交接目录，再决定是否保留本次ZIP；复制/写入/关闭/清理/最终Context失败不返回下载URL且清理本次发布。目录身份不匹配时拒绝删除外来替换，返回错误；不能据此承诺能清理攻击者或其他工具移走的旧目录。

最多32个待回调目录，30分钟有效期：新准备请求惰性清理过期记录，过期回调失败并清理；没有周期扫描后台线程。实例关闭/原repo关闭和legacy服务退出清理已登记目录并注销closer；登记表不序列化，进程重启后旧folder不能继续回调。崩溃遗留临时目录不自动认领，持久receipt/跨进程恢复仍属R04。写入权限仅显式授予本次临时输出，目录根逐成员复制，不放宽Source.Check对根路径/链接的约束，不授予Wiki源写权限。

真实导出资源构建发现webpack.export缺少SINGLE条件定义，补为false后原npm run build:export可完成；无UI设计或HTTP参数改动。R02仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变。浏览器两阶段临时目录交接/打包回调不再整体列todo；下一步为桌面非空savePath的路径授权/资源输出/错误传播，以及ProcessPDF/设备工具，再继续其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限和后台退出切换。本批不代表Native多workspace路由/正式源写或R04原子恢复已完成。

本批打包同步：统一wiki-app脚本新增webpack.export构建，输入门禁要求非空protyle-method.js与base.css；相应缺失/空文件测试及IronMan手动构建README同步。本地依赖模式下完整wiki-app资源构建通过，仍保留构建前stage/build快照恢复。此验证不包含Tauri/DMG或实际发布。

### R02：桌面HTML/MarkdownHTML保存Context（2026-10-30）


本批将exportHTML/exportMdHTML非空savePath桌面分支实际控制器接入请求Context，保留id/name/content响应及原前端生成最终HTML的分工。HTML继续使用pdf/keepFold/merge，MarkdownHTML仍沿用原不合并控制器语义；调用前批withHTMLFile完成所属文档/渲染/资源准备，准备失败时不新建外部目录。桌面保存资源不再调用无端口ExportHTML/ExportMarkdownHTML；旧函数仍供尚未迁移的其他业务兼容。

显式桌面目录授权只覆盖所选输出：要求绝对路径、拒绝上跳/反斜线/文件或最终目录链接；解析并捕获已存在祖先的物理目录及身份，支持macOS /var、/tmp及显式父级别名，缺失子目录经借用root创建。后续检查父级别名、根身份和新建子路径，拒绝父级/根替换及内部链接。输出与所属workspace/data/temp、repo/history/conf及捕获的程序/主题/图标资源根双向重叠时拒绝，包括祖先路径和保守大小写别名；共享文本前缀的路径组件兄弟目录允许。不得把输出权限用来写Wiki源。

私有payload准备后，对既有输出完整预检：所有资源的文件/目录类型、链接及原前端将写入的index.html均先检查，再逐项复制。保留原资源合并/同名资源覆盖和无关文件保留；不会由后端提前覆盖index.html。复制有界、逐文件临时替换、显式临时/所选输出授权，最终检查及借用root关闭错误向上传递；失败返回code=-1且无部分成功Data。输出目录是顺序合并，并非多文件原子提交：外部复制中途失败时，已完成的资源可能保留，不删除既有输出或承诺回滚。

原Save流程补齐失败响应分支，失败不再继续onExport包装/写盘；原最终HTML写入提为nativeHTMLFile小模块，独占创建本目录临时文件、写入/fsync/关闭，再rename替换index目录项，避免直接截断符号链接或硬链接目标。冲突不删除外来临时文件，失败按文件身份清理已创建临时文件并显示错误；原路径/模板/导出后行为保持。后端最后检查到前端最终写入之间仍有跨请求窗口，不宣称阻止其他进程在该窗口重定向整个目录，也不宣称跨文件CAS/事务；最终Native身份协议与R04/R06仍待。资源/HTML字节来源均不写回Wiki源。

原生Electron renderer构建配置补SINGLE=false以编译该原生保存分支，输出验证放在临时目录；统一wiki-app构建和独立原生构建均需验证，无新增依赖/命令/打包布局。R02仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变；桌面HTML/MarkdownHTML资源保存控制器不再整体列todo。下一步为ProcessPDF与设备工具，再继续其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限和后台退出切换。本批不代表Native多workspace路由/正式源写或R04原子恢复已完成。

### R02：PDF文档与书签Context（2026-10-31）


本批完成ProcessPDF的所属文档准备及书签Context端口。preparePDFPostprocessDocument先通过所属块索引定位根文档，再加载完整所属文档并按原排序合并子文档；即使传入标题ID，也保留原ProcessPDF加载整个文档的语义，不误用HTML预渲染的焦点裁剪。标题直接位于blockquote下时沿用原排除规则；资产目标经显式AV读取端口收集，供后续附件阶段使用，仅收集不嵌入/复制。候选与文档/索引读取均受原Context归属检查约束，不创建输出或写回docj/AV。

书签生成使用捕获的所属SQL句柄及请求Context读取标题，查询/取消/关闭/候选归属失败向上传递，无全局SQL回退。原processPDFBookmarks与PdfListToCLinks改为共享错误返回算法的兼容包装，仍供原ProcessPDF路径调用；保留页码排序、URL QueryUnescape、重复标题控制字符去重、页面/纵向定位和标题等级树。原兼容包装仍按旧调用方式记录错误，不能据此宣称完整HTTP错误链已迁移。

修复原算法在已有父书签时遇到无PDF链接的子标题会空指针的问题：缺少链接或SQL条目跳过该节点，其后有链接的标题按等级挂接到现有祖先。重复/空标题ID或非法等级拒绝，避免循环书签；nil PDF上下文和错误批注类型返回错误而非panic。生成完整书签计划后才调用pdfcpu.AddBookmarks；该调用只修改调用者持有的内存PDF Context，不输出文件。pdfcpu发生内部修改错误时不承诺回滚内存上下文，未来文件控制器必须只在所有步骤成功后发布。

真实PDF验证使用显式测试配置和私有输出，不初始化用户字体目录或改pdfcpu配置。pdfcpu.AddBookmarks不会立即刷新内存目录缓存，验收按写出并重新读取PDF后的目录进行。原ProcessPDF仍含全局DisableConfigDir/UserFontDir/字体装载、附件链接/嵌入、水印及VersionStr/文件写回依赖，完整HTTP控制器本批未切换到新Context端口；旧路径回归仅证明共享书签算法兼容，不等于其他PDF功能已隔离。

R02仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变；PDF完整文档/标题及书签Context中层不再整体列todo。下一步串行推进附件链接/嵌入、水印/字体和pdfcpu全局配置/版本依赖，再装配带身份校验与失败传播的PDF文件读取/替换和原processPDF实际控制器。后续仍有其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限及后台退出切换。本批不代表Native多workspace路由、正式源写或R04原子恢复完成。

### R02：PDF附件链接与嵌入Context（2026-10-31）


本批完成PDF附件链接/嵌入Context中层。pdfPostprocessDocument.assetLinks在所属候选检查后借用data根，复用原附件索引的notebook遍历/全局优先规则；只准读取该文档收集的附件及显式目录附件的成员，拒绝PDF额外插入的未选中本地附件。URL只绑定捕获的本机origin，其他端口/外部链接保留；支持/assets与/export/temp/assets形式，路径只解码一次，查询参数不参与文件定位，片段保留给相对链接，编码的中文/空格/井号/百分号按真实文件名定位，拒绝上跳、反斜线、坏编码与源链接。

保留原两个分支：removeAssets=false把当前本机资源链接改为相对URI；true将源文件流嵌入PDF，并沿用原FileAttachment标注位置、颜色、时间、文件名/UF和FileSpec结构。附件描述改为attached by wiki。新端口直接从受约束os.Root打开文件，经取消/owner/源根身份检查的64 KiB分块reader送入pdfcpu.NewEmbeddedStreamDict，不向库暴露全局绝对路径；单文件64 MiB、单次累计256 MiB，stat预检和增长时读取检查均执行。不创建输出文件，不写回docj/AV/附件。

共享applyPDFAssetLinks供新Context端口与原processPDFLinkEmbedAssets兼容包装使用，PdfListLinks亦共用安全批注扫描。修复原“移除全部批注再重建链接”的行为，只替换Link批注，保留笔记/标注；普通外部URI在新端口中不做额外解码。兼容包装保留旧宽链接分类/外链解码及单项嵌入失败跳过规则，仍用旧绝对路径解析器；因此兼容包装被调用不代表完整ProcessPDF已隔离。新端口的源读取/限额/库错误直接返回，调用者必须丢弃失败的私有PDF；pdfcpu内存修改不是可回滚事务。

缺失资源沿用原规则：无任何可解析文档附件时不处理链接；已有资源触发处理后，缺失文件在嵌入模式省略，相对链接模式保留其相对URI。目录资源成员可读取；目录本身作为嵌入流不支持，新端口返回明确错误，兼容入口继续原单项失败跳过。所有借用文件/root关闭错误向上传递。新端口不读取字体、改全局配置/版本或新增后台任务，资源身份检查不等于跨文件字节一致性快照。

R02仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变。PDF附件链接/嵌入中层不再整体列todo；下一步为水印/字体及pdfcpu全局配置、版本依赖，然后装配PDF文件身份/读取/成功替换/错误链和原processPDF实际控制器。其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限及后台退出切换继续串行推进。本批不代表完整PDF HTTP已Context化、Native多workspace路由/正式源写或R04原子恢复完成。

兼容修复补充：原接口前置存在性检查未解码文档中的百分号编码地址，会把真实中文/空格附件当成不存在而整段跳过；兼容包装现在仅对文档目标做一次规范化后检查，链接路径仍按原流程解码，不把已经解码的文件名二次解码。该修复由真实WebKit PDF/原processPDF回归发现，已补编码文件名兼容测试。

附件类型补充：嵌入前先stat确认普通文件，再open并复检，避免命名管道等特殊文件在打开时阻塞；新端口报错，兼容入口继续单项跳过。该检查有实际临时FIFO回归，不授予特殊文件读取权限；不宣称跨进程文件类型CAS。

### R02：PDF水印与字体选择Context（2026-10-31）


本批接入PDF水印设置、素材选择和中文字体选择的Context中层。捕获导出设置时一并冻结水印值、描述、文字/图片/PDF模式及默认字体资源身份；保留原“路径不存在即作为普通文字”的规则，后续出现同名文件不改变已捕获模式。空值和禁用水印保持no-op；只在实际启用的非空水印上报告依赖或素材错误，不阻断其他导出。

pdfPostprocessDocument.watermark使用所属文档检查与显式设备端口，复用原pdfcpu文字/图片/PDF解析及AddWatermarks和OnTop=true行为。图片和PDF交给解析器的是内存字节，不再让库自行打开素材绝对路径；素材和字体只读，支持显式文件别名但捕获后必须保持同一物理目标、父目录及文件身份/大小/修改时间。读取前检查普通文件与大小，读取中每64 KiB检查owner/取消/身份，素材32 MiB、字体16 MiB上限；文件和root关闭错误返回，失败的私有PDF须由调用者丢弃，不承诺内存回滚或跨文件一致性快照。

中文文本通过受信宿主提供的字体枚举/安装端口选择原有字体，支持库返回的带附加信息名称；描述参数排序确定化，预装指定字体时无需安装能力，找不到时沿用appearance/fonts/LxgwWenKai-Lite-1.501/LXGWWenKaiLite-Regular.ttf回退。所有端口错误向上传递，不向HTTP/CLI暴露回调；未绑定设备时拒绝执行，不隐式借用兼容进程。原processPDFWatermark包装显式复用共享核心，保留原UI错误通知方式。

边界：compatibilityPDFWatermarkDevice仍使用pdfcpu全局字体缓存、UserFontDir及默认配置；中文/PDF真实库测试在独立子进程及私有字体目录执行，不能据此宣称生产设备执行器已隔离。完整ProcessPDF仍有全局字体目录/配置/版本设置、旧文件读取/替换及错误控制器待改。本批未新增输出发布能力，也未完成完整HTTP多workspace装配。R02保持doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：独立PDF设备执行器（2026-10-31）


新增internal/pdfdevice的一任务一进程执行器与所属pdfPostprocessDocument.watermarkBytes中层。受信宿主通过Bind捕获绝对可执行文件及别名/文件身份，传入显式字体TTF和版本；调用前后验证执行文件身份。调用者的PDF与字体切片复制后传输，素材/默认字体沿用上一批的身份、有界读取和owner检查。设备失败或owner关闭时不返回任何产物，没有路径发布能力，也不隐式退回原全局状态。

wiki-kernel、wiki-cli、wiki-compat均在正常启动前识别内部--internal-pdf-device入口。父进程通过stdin发送严格JSON字节任务，只含PDF、文字/图片/PDF水印、字体字节和捕获版本，不含知识仓/素材/输出路径；stdout只返回PDF，失败非零及固定错误。子进程使用独立0700工作目录和字体目录，清空继承环境，仅提供私有HOME/TMPDIR及系统必需SystemRoot；pdfcpu默认配置、字体缓存/目录与Wiki producer版本修改均发生在一次性进程内。父进程有60秒上限、取消kill与Wait、有限输出/stderr和临时目录清理，清理错误亦丢弃结果。

输入PDF64 MiB、输出128 MiB、素材32 MiB、单字体16 MiB、字体累计32 MiB/最多8个、JSON传输256 MiB；描述16 KiB、文本1 MiB，拒绝未知字段、额外消息和路径型字体名；显式字体标识限1–128位ASCII字母/数字/下划线/连字符。复用原字体描述选择与水印解析算法，中文判定保留Han/Lm/平假名/片假名/韩文；支持显式预装字体字节及默认中文字体回退，父进程不读取pdfcpu用户字体缓存。库运行错误只返回失败，不发布部分PDF。

范围：本批具备可实际调用的独立设备和所属水印字节入口，尚未将完整书签/附件流水线及原ProcessPDF控制器装配到设备；原兼容接口仍沿用旧配置/版本/文件读写。旧用户字体库存的所属捕获/缓存适配也随完整装配继续推进。不能将独立执行器通过视为完整HTTP多workspace已隔离。后续需完成PDF文件身份/读取/成功替换与错误链，再处理其余R02依赖、历史与后台生命周期。R02仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变。

同步影响：没有新增可分发程序或第三方依赖，内部入口复用现有三个程序；公开CLI命令/参数及帮助、原UI/导航和资源清单保持。新入口已补真实三程序回归和依赖边界审计，打包脚本继续构建原三个入口，无需新增资源项。相对上一批未压缩临时二进制，kernel约30.3→36.3 MiB、CLI约29.6→35.8 MiB、compat约101.1 MiB不变；这不是最终压缩DMG大小。本批不提交/push、不构建DMG或安装。

### R02：PDF字节流水线与旧字体库存（2026-10-31）


本批完成所属processPDFBytes字节后处理流水线：从所属SQL捕获所选标题后，先由一次性设备进程预检当前origin的本机附件链接；父进程按原文档附件选择范围和显式目录成员解析索引、读取必要材料，再由第二个一次性进程依次执行原书签、附件相对链接/嵌入、水印及版本/序列化，返回前重新读取并校验PDF。两个阶段共享捕获的输入PDF，错误、取消、owner关闭或借用root/文件关闭失败均不返回产物。字节入口没有文件发布能力，尚未接入原processPDF HTTP控制器。

书签和附件算法迁入internal/pdfdevice供新设备及原兼容包装共同调用；保留缺失嵌套标题跳过、标题解码/重复名标记、附件布局/颜色/时间、普通外链及笔记标注保持等业务规则。标题只从所属文档所选heading的SQL记录捕获；设备不查询旧全局SQL。附件预检只返回当前origin链接标识；父进程沿用notebook遍历/全局附件优先索引，只读取文档附件或显式目录下实际被引用的成员。路径只解码一次，保留URL片段、忽略查询参数，拒绝未选中路径、穿越及源链接；目录引用有无末尾斜杠均可识别。相对链接分支不读取附件内容；嵌入分支保留缺失源省略规则，空文件必须明确标为已读取，未加载材料不可静默嵌入为空文件。目录本身不可作为嵌入流。

旧字体库存按设备资源只读捕获：兼容来源为home/.config/siyuan/fonts，不加入知识仓。捕获目录/普通.gob文件元数据与名称集合，最多8192条；仅在文字水印确实选择库存字体时读取该缓存（包含中文和拉丁文字），捕获后新增字体不会自动混入，替换/变更/源链接/限额失败明确返回。未知字体仍走原默认字体回退；坏的未选中缓存、禁用或未指定库存字体的普通文字水印均不读取这些字体文件的正文。旧gob在子进程私有目录中由原字体加载器解析，不在父进程解码或加载全局缓存；保留库存文件名作为查找key，即使与内部PostScript名不同。显式TTF也在私有安装目录中转为所选key，父进程和原缓存均不写回。

协议新增严格标题/附件清单、附件预检和互斥的TTF/旧缓存字节。单附件64 MiB、附件累计256 MiB、最多4096附件及预检链接，预检响应4 MiB；标题最多8192、单标题16 KiB/累计8 MiB。原PDF64 MiB、输出128 MiB、素材32 MiB、单字体16 MiB/累计32 MiB和每进程60秒不变；为容纳附件base64，JSON传输上限由256提升至640 MiB。字体标识扩展为Unicode字母/数字/下划线开头，其后允许字母/数字/下划线/点/空格/连字符，最多128字符，仍拒绝路径型名称；附件时间必须为可编码的PDF日期。设备只获得相对资源标识及字节，不获得知识仓、素材或最终输出的绝对路径。

R02仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变。完整字节流水线及选中旧字体库存适配不再整体列todo；下一步为PDF输入/输出文件身份、读取、成功替换与完整错误链，以及受信设备绑定和原processPDF控制器实际装配，再推进其余R02源写/历史/后台生命周期。此处的一次性本地设备进程不是CLI的网络server，不代表Native多workspace路由/正式源写或R04原子恢复完成；身份检查也不等于跨文件一致性快照。

同步影响：两README、设计/基线/计划、原兼容/设备/模型及三个真实入口测试已同步；公开CLI命令/帮助、原UI/导航、依赖和打包资源清单不变，无新增分发程序。现有打包脚本继续构建原三个入口。临时未压缩二进制kernel约36.5 MiB、CLI约35.9 MiB、compat约101.2 MiB，不代表最终DMG体积。本批不提交/push、构建DMG或安装。

### R02：PDF文件替换与原接口Context装配（2026-10-31）


本批将原/api/export/processPDF切换到ProcessPDFContext(c.Request.Context())，再由所属文档、独立设备字节流水线及受约束文件控制器完成后处理。原ProcessPDF签名保留为Background兼容包装；HTTP实际入口使用请求取消信号。设备由受信宿主绑定当前可执行文件的内部处理入口，不从请求接收执行路径、不搜索PATH、不回退旧全局处理。原ProcessPDF路径不再直接修改父进程pdfcpu配置、字体目录/缓存或版本；旧私有书签/附件/水印包装只为兼容与回归保留，不再构成实际后处理路径。

目标必须是已存在的普通.pdf文件，绝对路径，不允许上跳、反斜线、叶级符号链接或目录；输入64 MiB、输出128 MiB。显式选中父目录的系统/用户别名解析为物理目录后捕获身份，处理期间复核别名、目录及文件inode/类型/大小/时间/权限。拒绝捕获的workspace/data/temp/repo/history、应用资源目录内的目标，及与水印素材相同inode的文件；这不是未装配Native StoreRoot路由的跨store授权。原文件以不截断的读写权限打开，避免同目录替换绕过只读文件的访问限制。原前端生成raw PDF、选择路径、后处理和完成回调的业务顺序保持，不在本批新增原生UI写桥。

读取与写入按64 KiB检查所属Context/取消及身份；保存首次输入SHA-256，提交前重新读取原文件，既检查元数据也比较摘要，检测恢复mtime后的同尺寸修改。新产物先写到目标同目录随机、排他创建的.wiki-pdf-*.part，校验尺寸、摘要和身份，保留原权限位，文件Sync/Close后再次确认原文件与临时文件，再用同目录Rename替换并同步目录。新文件inode会变化；不承诺保留原owner/ACL/xattr。普通处理、写入、同步、关闭或rename前失败保留原文件；清理只删除仍匹配本次创建inode的临时文件，被替换的外来条目不删除。

新增PDFProcessError，稳定Code用于HTTP，完整原因通过Unwrap保留（包括errors.Is取消及加入清理错误的链）。Rename完成即记committed；此后目录Sync、身份复查、取消或关闭失败不做危险回滚，错误文本明确PDF已更新，HTTP code=-1且data={committed:true}。提交前错误仍code=-1/data=null，成功仍HTTP200/code=0/data=null；参数缺失或类型错误明确返回失败，不再断言panic。committed只说明本次替换发生，不保证其后没有外部修改或已确认耐久。身份/摘要前置检查与单次rename不是对不合作外部进程的跨进程CAS，也不是多文件恢复事务。

实际原接口现在复用上一批规范编码的相对URI：中文/空格及特殊字符以可解析URI保存，而非旧包装的裸字符串。原前端回归用真实file URL解析和读取目标文件验证定位，嵌入分支继续检查原附件字节，外链及笔记保持。该编码表示变化不会改变接口成功envelope；失败现在实际传回，原fetchPost的错误处理阻止误触发成功/附件清理回调。未更改原UI布局、命令帮助、导航、依赖或分发资源清单，无新增程序。

PDF文件控制器、受信设备绑定、完整字节后处理及原HTTP请求Context接入不再整体列todo。R02仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变；继续串行推进其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/handler/闪卡map调用者/行历史、历史/快照独立源修复权限及后台完整退出切换。本批实际HTTP仍运行在现有兼容服务与owner捕获边界，不代表Native多workspace路由、原生UI桥或R04原子恢复已完成。本批不提交/push、DMG或安装。

### R02：ImportSY借用root关闭与错误收尾（2026-10-31）


本批完成ImportSY文档归档调用链的借用root收尾。ArchiveImportContext.openScratch现在返回可报告错误的幂等关闭函数，独立借用描述符与临时目录owner分离，关闭能力在借用时捕获；借用子目录时父句柄的检查、交接关闭或子目录打开失败均关闭已取得句柄并合并错误。owned归档输入文件的父借用关闭也复用该能力；异常情况下关闭已打开文件，不返回可继续消费的文件。关闭能力属于受信资源后端，未加入HTTP/CLI参数或持久配置。

AV临时映射、文档回写、目录改名、storage移除及prepare阶段均通过errors.Join归还关闭错误；关闭失败时不返回成功样式的ID映射或prepared结果，prepare仍回收本次独占解包目录。发布资源、合并sort、挂载和复制阶段改从同一openScratch入口取得描述符，owned临时目录不再在这些阶段绝对路径重开；外部显式目录保留原普通目录检查与原合并语义。成功/错误/取消时均释放借用资源，不因关闭需要重新选择当前workspace或依赖请求仍然有效。

新增openRepairIOChecked及受信资源关闭后端版本，读/写权限、原文件锁、Remember/Upsert/Written回调不变；读写源的data/workspace root按逆序全部关闭，错误保留、关闭幂等。第二个root打开失败也会归还第一个及其关闭错误。ImportSY合并/排序/挂载使用该检查型接口；loadRepairTree也改用检查型关闭，关闭失败丢弃返回树。挂载仍先排空已接受的固定SQL索引队列，再归还root，晚关闭失败阻止finishArchive调用成功通知。

边界：原顺序发布可能已写入资源、sort或文档；关闭错误的准确上报不等于撤销这些写入，不是R04多文件原子恢复。owner临时目录清理由既有身份校验/幂等close负责，本批不扩张源写权限。为控制串行迁移范围，旧openRepairIO仍保留void关闭适配；当前model生产代码还有27处旧调用，需继续逐项迁移，不能标记所有借用IO均已完成。ImportData完整数据ZIP和ImportFromLocalPath/HTML资产转换等原全局调用链也仍待接入。

R02仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变。ImportSY借用资源关闭与失败收尾不再整体列todo；下一步优先迁移27处旧IO关闭调用，再推进ImportData/本地Markdown导入，以及其余writer/属性/缓存/广播/私有wrapper/handler/闪卡map调用者/行历史、历史/快照独立源修复权限和后台完整退出切换。Native路由/原生UI桥与正式源事务继续按后续阶段验收，不以此提前完成。

同步影响：两README、设计/基线/计划和模型回归同步；公开HTTP/CLI参数、成功envelope、帮助、原UI/导航、依赖、分发程序及资源布局均不变，无需机械修改入口或打包配置。本批不提交/push、DMG或安装，不处理现有其他未提交改动。

### R02：其余27处检查型IO关闭接入（2026-10-31）

本批迁移上一批记录的27处生产openRepairIO调用，覆盖归档导出选集/资源源读取、AV源/树/封面图片判断、数据库复制占用/预留/创建/绑定、关系及镜像写、冲突排序、增量/笔记本/引用索引、索引订正、同步冲突副本及commitLegacy提交。旧void关闭适配已删除，生产与测试调用统一使用openRepairIOChecked；这仅完成该接口调用集，不表示所有直接os.Root/os.File关闭点或完整R02均已完成。

关闭能力在借用时捕获，为所属indexRepairContext的受信后端，不接受HTTP/CLI参数或全局覆盖。data/workspace root逆序全部关闭，重复关闭返回缓存结果，主错误和关闭错误均保留；显式关闭与defer兜底不会重复拼接同一关闭错误。读取的切片/映射和占用/图片判断在错误时清空或返回false；已完成增量索引的ID结果仅作为错误伴随诊断保留，调用者仍须先检查error。

事务与同步冲突完成通知、引用索引成功状态、重复ID订正刷新和笔记本historyGenerated标记，均在相应root成功关闭后发布；已有队列排空/工作线程等待边界保留，关闭错误不绕开外层订正的固定队列收尾。AV保存/初始化/复制创建在成功关闭后才更新scope版本与缓存，晚关闭失败清除本次条目以允许从实际源重读。关闭失败可能发生于源字节或派生索引已经写入之后，不承诺多文件回滚，也不能把错误伴随ID结果当成成功。

R02总体仍doing，R03–R10 todo；原UI与业务语义优先保留，17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一步串行推进ImportData完整数据ZIP及ImportFromLocalPath/HTML资产转换，再检查仍直接关闭的导出目标root/文件等资源，以及其余writer/属性/缓存/广播/私有wrapper/handler/闪卡map调用者/行历史、历史/快照独立修复权限和后台完整退出切换。Native多workspace路由、原生UI桥和R04正式源事务仍按后续阶段验收。

本批仅调整资源完成/错误传播边界；公开HTTP/CLI、帮助、导航、原UI布局、依赖和打包资源无须变动。两README及设计/基线/计划同步，未提交/push、构建DMG或安装，保留既有其他改动。

### R02：完整数据ZIP导入的所属Context（2026-10-31）

本批完成完整数据ZIP的ImportData兼容链路。DataImportContext在HTTP解析multipart之前捕获ArchiveImportContext及所属owner、data/workspace/temp、语言、SQL/registry与同步间隔；显式本地ZIP的旧ImportData入口也经该Context执行。锁定与解锁使用同一个捕获owner，等待后再次验证归属/取消/只读/关闭状态；未装配Native跨workspace写路由的边界保持。

上传使用本次独占wiki-data-import目录，解包复用受控ZIP预检和独占wiki-extract目录，拒绝越界/反斜线/符号链接/重复条目及CRC或IO失败。完整数据ZIP与文档归档分别校验：保留单根目录及根下直接docj拒绝规则；data-20时间戳根仅完整数据入口可接受，文档归档入口仍拒绝。保留原合并覆盖、不删除未涉及文件、空目录及原文档/AV ID和元数据，不运行ImportSY的ID重映射或升级。emoji名称清理限定于独占解包源，冲突/改名错误在目标发布前返回；目标通过捕获root与逐文件版本检查合并，目标符号链接不跟随。

借用源与目标root关闭错误、独占上传/解包清理错误均归还；借用句柄和独占目录全部归还成功后，才向捕获owner申请原后台完整索引任务。重建任务的准入与同步计划更新共用owner退出门，失败/取消/拒绝准入不执行同步完成效果。原scheduleFullNotebookReindex也复用同一准入实现，保留原串行task槽、owner任务组、取消及真实等待生命周期。

HTTP importData与importSY共用multipart读取/reader关闭/表单清理助手，仍分别在读取前捕获Context；importData无需notebook/toPath，原HTTP200/code=0/data=null成功封装保留，异常返回code=-1且保留主/清理错误。HTTP reader或multipart的最后关闭清理可能晚于模型层后台任务准入，返回失败不撤销已接受的任务。

边界：完整数据发布仍为逐文件合并，后续错误可留下已写字节，没有跨文件回滚；HTTP成功代表源发布/模型清理完成且后台完整索引任务已接受，并不代表异步索引已经完成，其后错误仍由原owner任务通知。原完整重建算法、UI布局、公开CLI和打包资源保持；不借本批宣称Native多workspace路由或R04正式事务已完成。

R02仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变。ImportData不再整体列todo；下一步单agent串行推进ImportFromLocalPath/本地或ZIP Markdown与HTML资产转换，再检查直接root/文件关闭、其余writer/属性/缓存/广播/私有wrapper/handler/闪卡map调用者/行历史、历史/快照独立修复权限和后台完整退出切换。

两README、设计/基线/计划及测试同步；公开帮助/导航/依赖/资源分发边界未变，无须机械修改。保留其他既有改动，未提交/push、构建DMG或安装。

验证边界：本批完整数据导入专项、完整Go/原前端导入及16项集成通过；受控编辑器浏览器回归两次失败，已确认相关内核/CLI及前端核心包与上一批字节完全相同，尚未修复。该项仍作为验证遗留问题跟踪，不影响本批导入完成项的局部验收，也不能据此宣称R02或发布验收全部通过。

### R02：编辑保存验证屏障与归档目标关闭（2026-10-31）

本批先解除上批受控编辑器少尾字符的验证误判。真实键盘/HTTP诊断表明：失败时DOM仍含完整输入，保存队列dirty=true，原测试仅等待源文件12秒便超时；继续等约3331毫秒后dirty=false且完整标记已写入源。该复现证明保存尚在排队，不能将它报告为已确认的数据丢失；不据此推断所有可能编辑问题均不存在。

浏览器回归现在按iframe名称记录独立dirty状态，对对应编辑器在最多30秒内等待保存完成，再执行原精确磁盘内容、源隔离、撤销/重做和原生扩展字段保留断言；原12秒文件断言与240秒整项时限未删除或放宽。新增真实HTTP边界故障场景阻塞最后一个heading保存，验证未完成队列仍dirty、已有文件只含旧前缀且干净兄弟编辑器不能提前确认本编辑器；解除阻塞后仍必须写入完整标记并通过原全部断言。生产Protyle、session、UI布局、输入/保存与撤销算法均未改变。逐键原生快照的串行保存延迟仍存在，可作为性能优化另行评估，不能标为本批已优化。

同时完成归档文档字节、附件/emoji/AV资源及排序投影的3处目标root关闭接入。openExportIO捕获所属archiveExportContext的受信关闭后端，返回独立借用句柄和幂等/缓存错误的关闭函数；不新增HTTP/CLI选项。原输出owner根保持打开，owned输出仍从payload借用，不转回绝对路径重开。所有目标关闭错误与原写入错误合并，资源源/目标同时关闭失败也保留全部原因；取消或归属失效后仍可归还已借用句柄。

晚关闭失败可发生在暂存文档/附件/sort已改变之后，不承诺文件回滚；上层原归档控制流中止后续pack与下载URL交付，并由既有finish(false)清理私有输出。ZIP格式、名称/ID、字节/Spec/opaque字段、资源选择、sort投影和源只读保持。仅完成该归档目标借用root调用集，不代表其余直接文件关闭或全部R02调用已完成。

R02总体仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变。上批编辑尾字符验证误判与这3处导出目标关闭不再整体列todo；下一步单agent串行推进ImportFromLocalPath/普通或ZIP Markdown与HTML资产转换，再处理其余writer/属性/缓存/广播/私有wrapper/handler/闪卡map调用者/行历史、其他直接文件关闭、历史/快照独立修复权限及后台完整退出切换。Native多workspace路由与R04正式源事务仍按后续阶段验收。

两README、设计/基线/计划及测试同步；帮助/导航/CLI参数、生产原UI、依赖与打包资源未变，无需机械修改。保留其他既有改动，未提交/push、构建DMG或安装。

验证收尾：新增5项归档目标关闭回归、专项及完整Go竞态/vet、三入口临时构建、16项集成、原HEAD desktop/single兼容与带最后保存阻塞的完整浏览器编辑回归均通过。浏览器中断运行未算通过，重跑该项完成后才收尾；编辑保存串行延迟未优化，R02仍doing。

### R02：Markdown导入候选与链接准备Context（2026-10-31）

本批接入ImportFromLocalPath的准备阶段，不将完整Markdown/ZIP导入和HTML资产发布提前标记完成。候选importTrees与searchLinks原为进程全局变量，现移入每次调用独立的markdownImportPreparation；成功/失败/取消均释放本次树和链接表，准备失败的Context保留失败状态，不能换一批树继续当作成功调用复用。十二个并发准备实例可在相同人类路径下使用独立ID和标签配置，不相互解析到对方候选。

准备开始捕获所属ArchiveImportContext、语言及原EditorOptions（Markdown语法、编辑器解析偏好和独立emoji映射）。标准Markdown解析和后续行内合并分别复用原nativeparser.NewStdLute/NewEditor，YAML前言复用doclife.NormalizeCreationTemplate的原业务规则并使用请求Context；保留date生成ID时间、lastmod、title、YAML代码块、缩进代码、禁止自动URL转换及原HTML链接/图片到行内节点和宽高属性转换。纯HTML块转换未执行源文件或附件写入，整个parse前后检查归属/取消。

文档/标题链接索引、相对.md/.markdown链接、wiki链接的动态/静态锚文本、标题锚点、basename回退、未匹配链接保留、标签转换与嵌套行内合并算法保持；只改为读取本次候选和捕获的语法设置。链接与嵌套行内合并的AST遍历逐节点检查取消和所属Context，任一步失败不向原发布循环交付本次候选。标准解析器/HTML解析器内部的单次同步调用仍在返回后检测取消，不承诺强行中断其内部CPU执行。

原ImportFromLocalPath保留三参数调用形式，并增加与既有兼容入口一致的可选请求Context；普通Markdown与ZIP Markdown控制器将Request.Context传入准备阶段。锁定/解锁使用同一个捕获owner，等待和旧事务队列排空后复核归属；候选发布前逐树复核。目录Walk、文档读取和显式附件复制失败现在返回实际错误，避免忽略Walk结果或在错误候选上继续发布；失败后下一次实际单文件导入不会带入前次目录候选。源文件读取、部分资产选择与复制、旧indexWriteTreeIndexQueue、sort及IncSync等发布依赖仍有旧调用，不能以准备层Guard代替完整依赖注入。

边界：本批只完成本地/ZIP Markdown的调用内状态、配置/解析与链接准备及请求取消传递；ZIP multipart入口的共享目录/文件名、解包/上传关闭清理、目标定位、源/目标root IO、SVG/base64资产转换、固定SQL队列/排序/完成通知仍需后续串行接入。原流程可能在准备失败前已复制部分附件，本批无多文件回滚保证，也未授权Native跨workspace写入。生产UI、公开请求字段与成功envelope保持，数据仍写docj，不更改知识仓目录契约。

R02总体仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变。候选/链接全局残留与准备算法不再整体列todo；下一步先接ZIP上传/解包的独占生命周期与入口前捕获，再推进本地源/目标定位、附件/SVG/base64及写入/固定索引/排序/完成通知；其余writer/属性/缓存/广播/私有wrapper/handler/闪卡map调用者/行历史、其他直接文件关闭、历史/快照修复权限和后台完整退出切换仍待。

两README、设计/基线/计划、模型及HTTP回归同步。帮助/导航/CLI参数、前端代码、依赖、资源清单和打包配置未变，无须机械修改；未提交/push、构建DMG或安装，保留其他既有改动。

验证收尾：新增8项model/1项HTTP测试通过，含12个并发准备命名空间和实际目录读错后再次导入；专项/完整Go竞态、vet、三入口临时构建、16项集成与原HEAD desktop/single兼容通过，新增Markdown导入真实HTTP11项断言保持。受控编辑器本批未修改，产物摘要与上批一致，未重复运行其完整浏览器用例；不将该项算作本批新增通过。准备阶段完成，源/资产/发布和ZIP入口生命周期等仍待。

### R02：ZIP Markdown独占生命周期（2026-10-31）

本批完成ZIP Markdown入口与独占上传/解包生命周期：在解析multipart前捕获请求Context、原解析偏好和owner；上传使用独占0700目录与固定内部文件名archive.zip，解包使用另一个独占root，上传文件名仅经过滤后作为页面根显示名，不参与磁盘路径选择。复用原Markdown导入解析、链接与页面树业务，传入显示根别名，避免把随机临时目录名写进标题或层级。普通本地导入仍保留原三参数兼容入口。

归档解包拆出不限制布局的共享检查层，拒绝路径穿越、绝对路径、反斜杠、链接、特殊文件与重复条目并传播CRC、IO、关闭及清理错误；SY与完整data ZIP继续在外层应用原单根目录/格式规则。ZIP Markdown的模型层等上传文件、输入归档、成员、提取目录和上传目录关闭/清理成功后，才通过捕获owner的完成屏障安排同步；准备/导入/清理失败不执行这次完成步骤，保留多重错误。四个同名上传实例独立持有目录；单个捕获实例不可重复执行。

HTTP共用上传读取/关闭/表单清理辅助层并保持HTTP200/code/data封装，新增的filename参数仅交给Markdown显示名处理。模型完成之后HTTP读句柄关闭或multipart RemoveAll仍可能报错；此时返回失败但不承诺撤销已准入的同步任务或已写源文件。本次无多文件事务/回滚保证，旧本地资产复制和写入过程也可能已部分生效。

边界：本地源/目标root IO与目标定位、附件/SVG/base64转换、旧writer/固定SQL队列/sort及剩余UI通知仍待接入；其余writer/属性/缓存/广播/私有handler/闪卡map/行历史、其他文件关闭及历史/快照修复权限和完整后台退出切换继续串行推进。Native正式多workspace路由未据此启用，R02总体仍doing，R03–R10 todo；既有17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：Markdown正文源root与目标准备（2026-10-31）

本批接入普通本地与ZIP共用的Markdown正文读取及目标准备：每次导入打开选定目录的只读os.Root，单文件只登记所选文件，目录通过root.FS扫描登记本次清单；后续遍历、Markdown存在性判断、同名目录/文档判断和正文读取只使用本次root与清单。保留原字典遍历顺序、隐藏文件跳过、目录层级、同名目录与Markdown合并及原解析/引用处理。修正root.FS根节点名为“.”导致被当隐藏目录跳过的问题，传入真实根名称而不改ZIP显示别名。

选择路径的父目录先规范化（兼容macOS /tmp等系统别名），选定节点和扫描到的后代拒绝符号链接及特殊文件。读取前后复核根与文件身份、类型及文件大小/修改时间，传播读取、权限、扫描与关闭错误，复制正文时检查请求取消，入口及各阶段继续验证捕获owner。源root幂等关闭并合并错误，关闭成功后才允许普通导入执行本次同步完成；ZIP仍在外层等待上传/解包资源关闭。目录清单不是跨文件一致性快照，不承诺抵御同inode/同大小/同mtime的全部并发编辑；扫描后新增文件留到下次导入。选定根目录外同名.md不再参与该根的空文档合并判断，避免越过选择边界。

目标准备改用捕获blocktree/配置owner和数据root，检查notebook目录与登记、目标路径、registry查询、源.docj可读性/解析及根ID，不再调用进程级GetBlockTreeRootByPath或在目标缺失时返回成功；借用目标root的关闭失败同样传播。修正目录导入到已有页面下时根HPath遗漏父级的问题，物理层级和显示路径均保留父页面。目标读取检查是准备阶段验证，尚不构成目标版本锁或跨发布阶段CAS。

边界：root限制本批只覆盖目录元数据、Markdown正文和目标准备读取，链接附件复制、SVG/base64与资产目录选择仍有旧绝对路径IO，包含相对链接资产时不能据此宣称完整导入已拒绝全部越界；目标源写入、indexWriteTreeIndexQueue/固定SQL队列、sort及UI通知仍待接入。旧发布忽略写入返回值等问题仍列待办，发生错误仍可能已写部分资源/文档，不提供多文件回滚或持久receipt。Native正式多workspace路由未因此启用；R02总体doing，R03–R10 todo，既有17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：Markdown链接附件与资产复制（2026-10-31）

本批完成普通本地/ZIP Markdown导入的链接附件与目录附件读取、资产目录选择和复制Context接入。目录导入只从已有源清单读取附件；单文件导入可在所选文件的捕获父root下借用相对附件，目录附件先做root内扫描/链接/特殊文件检查。所有读取限制在该root内，拒绝越界、反斜杠、链接和扫描后替换；缺失附件仍保留原链接，其他权限/读取错误不再被存在性探测吞掉。沿用原来源路径文件锁，等待后复核归属，复制时逐块检查请求取消与源/目标归属。

捕获data root与关闭器，按原“文档目录assets→笔记本assets→全局assets”顺序选目录，不再依赖全局getAssetsDir/util.DataDir；候选是链接、非目录或权限失败时返回错误。附件使用原文件名过滤与新ID命名，O_EXCL仅新建，流式写入、同步和输入/输出关闭错误合并；校验源与输出身份后才交付文件名并登记本次去重表，保留原文件权限及文件/目录修改时间。普通资源、图片、链接和目录附件共用该复制端口，目录附件保留相对文件名、隐藏子项与空目录，已复制同一来源继续复用原映射。资产root与源root关闭成功后才允许本次同步完成，ZIP仍等待其外层上传/提取清理。

修正原相对Markdown链接分类误读上层绝对路径的问题：仅本次源目录内实际assets路径段下的.md/.markdown按资源处理，上层目录名含assets不再把普通文档链接变成附件，原页面/标题链接转换保留。图片/HTML宽度/链接/页面树原业务继续复用，未替换前端UI，也未把知识源改成Markdown。

失败边界：普通文件复制失败会按本次创建文件的身份尝试清理，关闭/清理多重错误均返回，遇到碰撞或外来替换不会覆盖或删除该文件。输出为顺序写入，不保证流式文件在完成前不可见，也没有跨文件一致性快照/跨进程CAS/多文件回滚。目录附件在后续子项失败时可能保留已复制子项和新目录，不递归删除以免误删外来新增内容；同一次导入此前成功复制的资源也可能留下，失败不执行本次同步完成。

剩余：SVG/base64转换仍调用旧实现（含共享base64临时目录、绝对路径写入和仅日志错误），虽然其资产目录已由本次root选择，但不能认定所有资产写入已上下文化；下一步串行接入这两类转换。原文档writer返回值、目标源写root/固定SQL队列/sort与其余UI通知，以及其他writer/属性/缓存/广播/私有handler/闪卡map/行历史、文件关闭、历史/快照修复权限和后台完整退出切换继续待办。Native正式多workspace路由未启用，R02总体doing、R03–R10 todo，既有17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：SVG/base64图片转换（2026-10-31）

本批完成原SVG/base64图片转换后端的Context接入。移除processBase64Img/processHTMLBlockSvgImg及共享temp/base64暂存、旧绝对路径复制/写入；普通本地与ZIP Markdown使用已有捕获资产root，HTML转换捕获同类owner/data root并持有到转换/渲染结束。HTML没有外部源目录，不伪造Markdown源路径；资产发布复用O_EXCL、逐块归属/取消检查、输入/输出关闭和按本次文件身份清理的原Context端口。

保留原PNG解码/编码、JPEG质量100、SVG data URI原字节、大小写不敏感%0A替换、alt命名/过滤/新ID、图片宽高等属性；受支持类型解码/编码或发布失败直接返回，成功发布后才替换节点链接。修正TextMark a节点误写Tokens的问题，改写实际href并保留文字。已解码但不支持的MIME保持原URI不发布；原PRE中SVG首个匹配提取、base64 SVG内联展开、表格文本管线/数学标识和自定义renderer规则保持。原SVG正则不匹配时仍保留原块，不新增XML标准化或SVG消毒策略。

HTML2Markdown保留旧两参数形式并接受可选Context；新增错误返回HTML2TreeWithContext，仓内粘贴和剪藏转换分支传Request.Context并检查错误，不再忽略fallback错误。HTML2Tree旧两返回值包装保留供兼容，失败记录日志/返回nil，仓内业务已改用错误版本，包装本身不再保留旧全局IO。HTML粘贴转换失败仍保留Failed to convert的data，但code改为-1并附实际错误；剪藏转换失败同样返回失败。公开请求字段及成功返回/原UI不变。

边界：解码器和HTML parser内部的同步CPU调用不能强行中断，在调用前后及编码writer/publish阶段检查取消/owner。PNG/JPEG先编码到调用内内存buffer再发布，不创建共享临时文件；生成器失败无输出节点/名称，资源关闭失败不算模型成功，但此前成功生成的其他资产或后续失败前已写文档可能保留，没有多文件回滚、原子整批或持久receipt。转换依赖合法的当前可写兼容owner；Native正式多workspace写路由尚未启用。

剩余：原文档writer错误返回/目标源root、固定SQL队列/sort/其余通知；HTML控制器的util.NewLute构造及粘贴后续本地文件复制、剪藏multipart上传/远程抓取/其他外层全局业务尚待独立接入，不能将本批图片转换等同整个粘贴/剪藏流程上下文化完成。其他writer/属性/缓存/广播/私有handler/闪卡map/行历史、文件关闭、历史/快照修复权限与后台完整退出切换继续单agent串行推进。R02总体doing、R03–R10 todo，既有17设置/72事务/AV60/60/49订阅/42业务体统计不变，知识源仍docj。

### R02：Markdown文档writer与固定索引（2026-10-31）

本批接入普通本地与ZIP Markdown导入的实际文档writer、块树登记和固定SQL主队列。原indexWriteTreeIndexQueue在写源前登记块树、调用者又忽略其错误，现改为publishDocuments：每批捕获data root、目标notebook目录身份、原Markdown渲染设置/单行保存/大文件阈值及语言，复用filesys.TreeContext原JSON、空段落、Spec/type规范化和Source.Rewrite；实际写入返回值逐级返回，源未写成不登记块树或主索引。

目标与ID路径校验、source链接拒绝、data/notebook根替换和owner/只读校验均在真实writer端执行，保留原目标路径文件锁。导入分配的是新文档ID，已有源文件/索引ID碰撞返回错误，不再静默覆盖无关文档。JSON编码与缓存更新使用捕获的配置和原commit IAL发布器，不再经全局filesys.WriteTree或晚查当前缓存；大文件提示使用捕获阈值/语言。空文档复用原补空段落和写后Upsert hook，完整树投影只登记一次；新增RepairRegistry.Index捕获DB并沿用原块树锁，登记失败可见。

主队列使用捕获main DB的LegacyMainQueue.Index，关闭writer前Flush已准入前缀；后续文档失败或请求取消也使用WithoutCancel排空已准入工作，再关闭源root，错误合并。writer close幂等，不重刷后来的队列。原文件级Rewrite允许单次已准入源发布在临界区结束后才响应取消，但不再准入下一篇；普通/ZIP外层仍在所有writer/资产/源及上传提取资源关闭成功后才计划本次同步。原文档标题/层级/引用/图片与.docj格式保持。

边界：源与派生索引为顺序发布，没有跨文件回滚、持久receipt或跨进程CAS。索引/缓存/关闭失败可能发生在源文件已落盘之后，返回错误、不计划本次同步；此前成功前缀或资源也可能保留，需要原索引重建/恢复流程处理。未把准备期目标检查升级为跨阶段版本锁；既有Source.Rewrite的临时清理细节与其他历史/修复文件关闭仍需后续审计。本批只推进此导入链，未授权Native正式多workspace写入。

剩余：原box.setSort仍有全局路径、仅日志错误和即时排序通知，需下一步接捕获root/版本化持久化和关闭后owner通知；其他通知/原控制器依赖也未据此全部迁移。HTML parser构造、粘贴后续本地复制与剪藏multipart/远程抓取/外层业务，以及其余writer/属性/缓存/广播/私有handler/闪卡map/行历史、其他文件关闭、历史/快照独立修复权限与后台完整退出切换继续单agent串行。R02总体doing、R03–R10 todo，既有17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：Markdown排序与关闭后通知（2026-10-31）

本批完成普通本地/ZIP Markdown导入的排序文件与排序通知Context接入。保留原hPath字典序、父路径分组、重复路径去重及组内rank生成算法，仅替换box.setSort调用。使用已有捕获资产data root读取notebook/.siyuan/sort.json，保留未涉及的既有项并合并本批rank；缺少sort文件仍跳过、不创建。JSON/null/结构/权限/链接、查询及写入错误均返回，不再仅日志后算成功；更新沿用原目标文件锁和Source.Rewrite的before字节校验/同目录发布，并检查notebook目录身份。

排序通知在写前由捕获blocktree查询准备，保存原filetreeSortChanged所需parentPath/childIDs和rank比较结果，派发时不再通过全局treenode.GetBlockTree查当前页面。保持原消息字段与相同rank的未指定顺序，不在本批重设分组/通知业务规则。排序成功只产生调用内pending completion，不立即广播；候选树/链接清理不会提前丢掉该pending，普通导入等待writer/索引/资产/源root关闭，ZIP还等待上传/提取目录清理，之后才走捕获owner的completeUI通知与同步准入。

所有失败退出会丢弃本批pending；完成函数无论成功或失败都清理pending，ZIP整体退出也清理，避免下次导入复用旧通知。completeUI继续深拷贝通知字节，准入前取消/owner变化拒绝；准入后通知由owner任务组/关闭屏障管理，关闭会取消并等待未发任务，不把已成功操作的通知生命周期绑定到HTTP返回后的客户端断开。仅新增filetree-sort完成事件到原dispatcher，实际广播为保存的payload，不增加前端协议或全局块树回查。

边界：文档/索引和排序仍顺序发布；排序失败或后续关闭/清理失败时，之前源文件、索引或已写排序可能保留，返回错误但不计划本次同步、不发送本批排序通知，无多文件回滚、持久receipt或跨进程CAS。共享Source.Rewrite的临时清理细节继续后续审计。其他调用者的box.setSort/pushFiletreeSortChanged仍为旧兼容路径，本批不认定所有排序入口完成；其他进度/大文件提示与通知业务也未因此全部迁移。

剩余：HTML parser构造、粘贴后续本地文件复制、剪藏multipart上传/远程抓取/外层全局依赖，以及其他writer/属性/缓存/广播/私有handler/闪卡map/行历史、文件关闭、历史/快照独立修复权限和后台完整退出切换继续单agent串行。R02总体doing、R03–R10 todo，Native正式多workspace写入未启用，既有17设置/72事务/AV60/60/49订阅/42业务体统计不变，知识源仍docj。

### R02：HTML粘贴配置与本地资源（2026-10-31）

本批完成html2BlockDOM粘贴入口的原parser配置、容器策略、后续本地资源复制和最终DOM渲染Context接入。入口只负责参数/返回，模型在同一次owner设置快照中捕获Markdown与System.Container，使用原nativeparser.NewLute及HTMLTag2TextMark/HTML2MarkdownAttrs配置；同一捕获资产root覆盖HTML图片转换、Markdown中间格式、二次解析、原段落/列表/单格表格规则、资源复制、TextMark/嵌套行内处理与Protyle渲染，关闭成功后才返回DOM。不再晚查util.NewLute、Conf.System.Container或util.DataDir。

保留原file://、绝对路径、Windows前缀及URL解码规则；相对/网络/缺失来源、无法解析的URL和非std容器按原规则保留链接。每个显式本地来源借用自己的文件/目录root，检查链接、文件类型与身份，复用已上下文化的流式资产复制和关闭错误链；权限/读取/写入/关闭失败返回失败且不返回部分DOM。复制仍为每次引用分配新后缀，即使原名称已含ID也不复用；原图片字节、文件权限/时间、目录子项与空目录保留。共享复制端口增加可选可信名称，未提供时Markdown导入沿用原命名规则，未更改知识源docj格式。

成功/失败envelope保持前批约定（失败code=-1/data=Failed to convert），原前端无需改参数。解码器/parser/渲染器单次同步CPU调用只在调用前后检查取消，逐节点和复制writer继续检查owner/取消；发生后续失败时已生成资产可能留下，无多文件回滚/原子整批或持久receipt。转换仍依赖合法的当前可写兼容owner，未启用Native正式多workspace写路由。

剩余：剪藏extensionCopy的parser构造、multipart上传/读取/关闭、远程抓取和其他外层全局业务尚待接入，不能将本批粘贴入口完成等同所有HTML/剪藏入口完成。其他排序/通知调用者、writer/属性/缓存/广播/私有handler/闪卡map/行历史、共享Source.Rewrite临时清理、其他文件关闭、历史/快照修复权限和后台完整退出切换继续串行。R02总体doing、R03–R10 todo，既有17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：剪藏表单与远程来源（2026-10-31）

本批完成extensionCopy剪藏入口的parser、multipart上传、资产发布、远程抓取和返回Context接入，接续上一批HTML粘贴。入口捕获原owner/Markdown配置和语言服务后即借用目标data root，再读取表单；同一捕获root持有至原HTML/Markdown后处理完成。逐次读取检查取消/归属，上传Open/Read/Close、目标写入/同步/关闭、目标root关闭及已解析multipart表单RemoveAll错误汇合；成功结果只在关闭与表单清理后交付，失败code=-1且data为空。HTTP层仍拥有请求Body本身的关闭责任，模型只恢复其包装并清理表单临时文件；解析失败时标准库负责其尚未交付的multipart临时文件。

保留Gin原默认32MiB表单内存阈值、dom/notebook/href/clipType字段、现有笔记本assets目录优先/不存在或普通文件时全局assets回退、原URL解码与二次解码、%u兼容、文件名过滤/新ID、MIME/SVG判断和imageView2链接映射；无效URL跳过，缺dom/坏表单明确失败，notebook ID和链接目录校验阻止越界。原上传MIME判断仍需整文件缓冲，未宣称定额内存流式转码。HTML图片转换仍按原规则使用全局assets，上传文件按所选notebook/global目录，未改变相对assets返回。

保留两站点文章raw URL映射、part剪藏绕过远程、缩进代码围栏转换/数学识别、iframe去换行、段首空白/图片alt与title纯文本/TextMark及行内展开与Markdown输出。远程请求使用请求Context和原30秒cloud客户端，关闭自动响应预读后通过受控读取关闭body；普通无body网络失败沿用HTML回退，取消/归属撤销以及响应读/关闭失败不伪装成功。修复req取消时非nil包装但内部HTTP响应nil导致的空指针访问；即使fetch错误带body，也检查关闭并交付失败。不再晚查util.NewLute、util.DataDir或Conf.Language。

归属撤销在外部调用前后和逐块读取检查，原parser/formatter与第三方同步调用不强制抢占；HTTP请求取消传递到网络层。已成功发布的资产可能在后续失败时保留，不承诺多文件回滚、原子整批或持久receipt。仍是合法可写兼容owner，不等同Native正式多workspace路由。R02总体doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体既有统计不变。剩余共享Source.Rewrite临时清理、其他排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史、其他关闭错误及历史/快照修复权限和完整后台退出切换继续串行。

### R02：共享Rewrite临时清理（2026-10-31）

本批完成共享indexrepair.Source.Rewrite的失败临时清理、父目录归属、发布前身份/版本与授权检查，接续上批剪藏之后列明的待办。公开Source字段和Rewrite签名不变，临时文件仍为同目录.wiki-repair-随机后缀、独占创建，保留目标原权限、写入→Sync→Close→原子rename以及nil-before只允许新建/非nil-before要求原字节的业务。新增短写检查，文件Write/Sync/Close、失败清理和借用父root关闭错误汇合返回，释放原文件锁后不再有本次资源操作。

父目录通过已打开的root保留到收尾，初次及发布前核对可见父目录身份；父路径被挪走/替换时拒绝发布，清理仍针对已捕获父目录的本次文件，不使用替换后的父路径删除。临时文件用打开描述符Stat记录身份，发布/清理前要求相同普通文件；被其他文件/链接/目录替换则返回归属变化而不删除该替换物。成功rename后不再删除原临时名称，避免误删后来复用该名称的文件；失败清理时正常不存在（含检查之后由别处移除）不新增错误。无法取得初始文件身份或清理权限失败时保留无法安全清理的临时文件并返回错误，不隐瞒为清理成功。

沿用临时输出阶段的WithoutCancel收尾策略：进入前和等待文件锁后取消不启动写入，进入后完成本次发布/清理；发布前再次以该critical Context校验owner权限，独立的只读/关闭/归属撤销仍拒绝发布。上层Authorize若同时检查其捕获请求的取消，也会拒绝发布并完成清理，不绕过该更严格策略。发布前对捕获父root再核对原目标字节/新建占用，已观察到的外部版本变化不覆盖。检查与rename/unlink仍有系统调用间隙，不宣称跨进程CAS、任意外部编辑排他、目录fsync掉电持久性或多文件事务；成功rename后的父root关闭失败会返回错误但不会回滚已发布文件。

原docj格式、UI、HTTP字段/返回、CLI参数和依赖/打包路径未改。共享调用者包含原修复树写入、AV绑定、归档准备与导入/导出资源、普通/ZIP Markdown writer和sort，因此用真实调用链回归，不能仅依据底层单测宣称全链完成。R02总体doing、R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计保持。其他排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史、其余关闭错误与历史/快照修复权限及后台完整退出/切换仍待。

### R02：历史清理与损坏源归档（2026-10-31）

本批完成Source.RemoveLegacyHistory与Archive的捕获目录/文件身份、授权/取消和资源生命周期接入，接续上批Source.Rewrite之后的待办。公开Source字段及两方法签名不变，原兼容notebook/.siyuan/history路径与corrupted/<stamp>-<随机后缀>/<原路径>保留，未改变docj源格式或原索引订正阶段顺序。model/index_fix.go仍在resetDuplicateBlocksOnFileSys内先清历史，再归档非法文档文件名；其他业务、UI与HTTP/CLI字段未重写。

历史清理先做完整只读清单，拒绝链接、特殊类型、坏目录和未完成扫描；已存在的history普通文件作为未知对象保留并返回错误，不按目录清理。父目录与history借用root保持到收尾，子目录按递归深度借用，检查可见目录和打开目录的身份。清单内存随条目数增长；删除时再次枚举/核对已扫描项，逐文件/空目录检查授权、取消、身份、普通文件size/mtime，只使用Remove，不对原路径盲目RemoveAll。新增/更新/替换条目、目录被挪走/链接替换或权限撤销都会停止；后来加入的未扫描文件不会被递归带走。初始缺失仍成功；执行中取消或错误可能已删除前缀，明确返回错误，不宣称目录删除原子性或回滚。

损坏源归档捕获源父root和普通文件身份，受控读取并检查Read/Close，再用既有Rewrite发布独占备份，保留完整备份路径逐级目录Sync/Close。删除源之前验证备份父目录/同步过的各级目录、备份身份和原字节，并重新读取源、核对相同文件身份与原字节；同字节但不同inode的替换源也保留，源或备份被改动/替换不继续删除。最终删除通过捕获源父root，删除前再检查owner权限。进入备份发布前遵循请求取消，发布成功后的同步/核验/删源保留原WithoutCancel收尾；上层Authorize更严格的捕获请求取消/只读/归属撤销不绕过。

源/备份文件、目录Sync和借用root关闭错误逐级汇合，备份未完成或验证失败保留源。Archive一旦选择输出位置，错误时也返回候选dest供检查；候选不证明文件已存在/完整，成功仍返回原相对路径。既有model调用者只消费error，公共HTTP返回未变。晚期父root关闭错误可能发生在已归档并删除源之后，返回dest与error、不隐瞒已发布状态，也不回滚。检测与rename/unlink之间仍有系统调用间隙，不宣称跨进程CAS、任意外部写入排他或多文件事务。

R02总体doing、R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。本批不等同所有历史/快照业务完成；共享Source.Remove/Rename其余调用者、其他排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史、其余关闭错误与后台完整退出/切换继续串行。

### R02：共享删除/改名与调用锁（2026-10-31）

本批完成共享Source.Remove/Rename及原索引修复、导入scratch改名、导出sort和闪卡导出清理调用链核对。保留公开Source字段、方法签名和原路径/文档/目录业务；动作前捕获源条目及父目录身份，锁后及动作前复核授权、取消、源身份/普通文件size与mtime，Rename复核目标不存在。借用父root关闭错误与动作错误汇合；动作已完成后的检查/关闭失败明确返回错误，不回滚已删除或移动的文件。

原recreateTree外层锁保留，最后删除不重复加同一锁；闪卡导出根关闭错误已接入。公开HTTP/CLI、原docj格式、依赖及打包路径不变，完整原前端和受控浏览器回归见执行计划。

R02总体doing、R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为flashcardArchiveStorage.source/Read/Write仍为void的借用root关闭回调和版本登记收尾；本批完成导出根关闭，不等同长期闪卡storage关闭已完成。其他排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史、其余历史/快照与后台完整退出/切换仍待。

### R02：长期闪卡storage关闭与版本交付（2026-10-31）

本批完成flashcardArchiveStorage.source/Read/Write借用root检查型关闭和版本登记。source返回func()error，复用已验证的幂等关闭组件，借用前后检查捕获owner；关闭后端在借用时固定，不晚读可变字段。Read/Write取得原每文件锁后重新准入，锁持有到root关闭及版本交付；Write仅在本次Source副本上取消内层重复Lock，Authorize仍保留，未改变原路径、MessagePack、牌组/日志与缓存生命周期业务。

关闭/收尾失败不推进版本；失败写若已改变磁盘字节，重试需先重读。纯文件缺失的原缺省保持，缺失和关闭等复合错误不再被加载器忽略；缓存牌组跨请求及索引替换后的保存保持。公开HTTP/CLI、原格式和打包路径不变，完整验证见执行计划。

R02总体doing、R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为共享Source.Read的文件描述符关闭、读取取消/身份及调用者缺失错误契约；其他排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史、其余历史/快照和后台完整退出/切换继续串行。

### R02：共享Source.Read关闭与完整读取（2026-10-31）

本批完成共享indexrepair.Source.Read的文件描述符关闭、分块取消/身份和直接调用者缺失分类。公开Source字段与Read签名不变，读取不调用Authorize或取得writer锁，只读Source仍可使用，避免Rewrite内部/原调用者已持锁时重复加锁。替换os.Root.ReadFile为受控Open/Stat/Read/Close；捕获源文件与父目录身份，在已打开父root内借用文件，核对文件描述符/可见路径/父目录、普通文件类型和size/mtime，并在64KiB分块前后、文件及父root关闭之后检查取消和已观察到的身份变化。

关闭/读取/取消失败不返回部分bytes，初始纯缺失与不完整读取分开，缺失分支已同步共享分类。已有空文件和原读取权限边界保留，完整验证见执行计划。

R02总体doing、R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。其余独立文件/root关闭、历史索引、排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史及后台完整退出/切换继续串行。

### R02：解包闪卡的Context与完整交付（2026-11-01）

本批完成ImportSY解包闪卡读取的Context接入：生产路径改用已捕获ArchiveImportContext.openScratch与owner guard，读取期间、交付前及借用root关闭后复核取消和owner。先捕获storage/riff目录与.deck/.cards两份候选的存在状态、inode、类型、size/mtime，再读取原始字节；任何读取、文件/父root/外层root关闭或最终检查失败均不返回候选map，原错误原因保留。

共享ReadLimited保留每文件32MiB上限、原始字节与空缺省；全部关闭/guard成功后才交付。此处不保证整个ImportSY回滚。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为historyIndexContext.index的独立root关闭与Plan/查询生命周期，其余排序、通知、缓存、writer、属性、广播、私有handler、闪卡map、行历史及后台退出/切换继续串行。

### R02：历史索引读取关闭与队列准入（2026-11-01）

本批完成historyIndexContext.index的独立读取root关闭与队列准入生命周期：保留原historyindex.Plan只读解析和历史字段投影，将候选扫描独立成scan；扫描/借用root关闭/最终Context与owner核验全部成功后，才调用原ReplaceDirectory。关闭失败不再被忽略，扫描与关闭双错误保留；扫描panic先转为失败再执行检查型关闭，不交付候选或发送进度。

保留初始纯缺失、原只读历史投影与已接收队列实际Flush后通知；通知失败不回滚已提交SQL。根/目录身份核验不是完整目录快照。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为historyindex.Plan的目录枚举资源、完整候选扫描身份和取消核验；其他adapter、排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史及后台完整退出/切换继续串行。

### R02：历史目录检查型枚举与扫描清单（2026-11-01）

本批完成historyindex.Plan内部目录枚举资源与候选扫描核验。用受控Open/Stat/分批ReadDir/Close替换fs.WalkDir对os.Root.FS的隐式枚举；每批64项，严格区分正常EOF与复合错误，零进展返回错误。目录打开描述符须匹配已捕获身份，读前后及关闭后检查Context、根/路径身份与元数据，读取和关闭错误汇合，失败不交付部分entries。只借用外层Source.Data，内部目录描述符各关闭一次，不申请Authorize或writer锁。

候选交付前重枚举比较子项，并复核所有节点元数据；保留原顺序、只读格式与字段。此观察清单不是跨进程原子快照。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为ListNotebooksContext的独立root关闭与列表/修复生命周期；其他独立读取和目录枚举adapter、排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史及后台完整退出/切换继续串行。

### R02：笔记本列表关闭与修复边界（2026-11-01）

本批完成ListNotebooksContext外层root关闭与列表/修复调用生命周期：保留原notebooks.Scan/Apply和列表字段，捕获AppConf、data、workspace、主SQL及blocktree依赖。预取消、空/关闭owner与无效根不进入扫描，打开后比较根描述符身份，扫描/修复前后及root关闭后核对Context、owner、DB选择器和所选绝对路径身份。root检查型幂等关闭，列表错误、panic及关闭错误汇合，失败不返回部分或成功列表；关闭后的检查不再使用已关闭root。

保留原只读/修复权限和settingsGate→repairGate顺序，回调复用捕获的依赖，避免重入；后续关闭失败不回滚已执行修复。内部notebooks读取/枚举关闭仍待。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为internal/notebooks的有界元数据读取、Scan枚举及Apply缺失分类/关闭；其他独立读取/枚举adapter、排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史及后台完整退出/切换继续串行。

### R02：笔记本内部读取、枚举和修复错误（2026-11-01）

本批完成internal/notebooks的有界元数据读取、Scan枚举关闭和Apply错误准入。原read复用Source.ReadLimited，保留1MiB每文件上限，接入共享文件/父root关闭、逐块取消、路径/描述符身份与长度检查；路径或超限错误仍可识别ErrPath，底层原因保留。Scan/Apply使用IsSourceNotExist，仅真正初始纯缺失才能进入缺省/补建分支，已选定读取不完整或复合关闭错误不会触发补建或损坏清理。

目录枚举显式关闭，错误不交付Plan；修复读/写/关闭失败保留原原因，完整保存后才afterCreate。源部分副作用不回滚。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为云附件uploadCloudAssets/cloudUploadFile的root与延迟文件读取生命周期；其他独立读取/目录枚举adapter、排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史及后台完整退出/切换继续串行。

### R02：云附件读取关闭与部分上传边界（2026-11-01）

本批完成uploadCloudAssets外层root与cloudUploadFile延迟读取生命周期。捕获请求Context、AppConf/data/workspace、根路径身份并在打开前后及检查型关闭后核验，关闭的账户或workspace owner不准继续。外层root幂等关闭，所有失败保留已确认上传计数和原错误；最终检查不使用已关闭root。

延迟文件读取前后和关闭后检查身份/取消，超限仍不打开payload；失败不发起该文件上传，已完成的远端部分效果不回滚。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为archiveExportContext.documentPaths的独立目录枚举关闭与候选交付；其他读取/目录枚举adapter、排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史及后台完整退出/切换继续串行。

### R02：归档导出路径与共享目录读取（2026-11-01）

本批完成archiveExportContext.documentPaths的检查型目录枚举与候选交付，并新增共享indexrepair.Source.ReadDir。Source字段和Read接口不变，ReadDir仅借用外层root、不取Authorize或writer锁；捕获选中目录/借用root/读取描述符身份，128项分批枚举并按原字典序返回。读前后、文件与借用root关闭后检查Context、目录路径及inode/size/mtime；读取、Stat、两层关闭、混合EOF/缺失和无进展错误汇合，不返回部分entries。初始纯缺失保留原错误，选定读取后的失败带原ErrSourceReadIncomplete，不得当成可选空目录。

归档路径候选在全部关闭与最终guard成功后交付；保留原顺序/过滤/源tmp，不宣称全仓原子快照。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为ArchiveImportContext.walkScratch及其AV候选/目录改名/storage预检调用者；其他独立读取/枚举、排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史及后台完整退出/切换继续串行。

### R02：ImportSY scratch受检遍历（2026-11-01）

本批完成ArchiveImportContext.walkScratch的受检目录枚举。原根访问、回调、字典序深度优先、目录与文件上的fs.SkipDir及fs.SkipAll语义保持；链接仍在业务回调前拒绝。每层目录改用共享Source.ReadDir，继承选中目录/描述符身份、128项分批读取及文件和借用root检查型关闭；枚举前后复核请求Context和捕获owner。初始真正缺失仍保留原错误，已选目录读取/关闭失败带ErrSourceReadIncomplete并停止遍历。

三个调用者的原准入边界已核对并通过故障回归：remapScratchAVs完成全部AV候选收集后才生成ID、改写和改名；renameScratchDirectories完成全部扫描后仍按最深目录优先改名；removeScratchStorage完成整个storage树校验后才RemoveAll。模拟后续目录读取/关闭不完整时，AV原字节、原目录和storage均保留，不把复合缺失当成无数据。外层openScratch的私有Source、Authorize和幂等检查型关闭不变。

这仍是顺序业务：一旦后续写/改名开始，晚期失败可能保留成功前缀；未增加整个ImportSY回滚、原子目录快照、内容哈希或跨进程CAS。prepare文档候选、mountArchive、mergeArchiveDirectory和资源发布等独立fs.WalkDir仍未迁移，不能据此机械标完成。公开UI、HTTP/CLI、docj格式、依赖和打包布局未改。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为ArchiveImportContext.prepare的docj候选扫描，复用本批受检walk后再进入文档读取/ID映射；mount/merge/资源发布遍历继续独立审查。完整验证与证据见AnonyPower执行计划的`wikiman-r02-import-scratch-walk/results.json`。

### R02：ImportSY prepare候选清单（2026-11-01）

本批完成ArchiveImportContext.prepare的docj候选扫描与读取准入。prepare复用walkScratch的受检字典序遍历，不再直接使用fs.WalkDir；扫描时为所有目录和docj候选捕获类型、inode、size/mtime，候选须为普通文件。完整扫描清单复核成功后才逐个读取；每个候选在读取前后再次核对身份，全部解析、ID映射完成并最终复核整份清单后才交付prepared结果。

故障回归覆盖后续目录读取不完整、扫描期间候选替换、读取后候选替换及第二文档部分读取；失败均不交付部分prepared结果，不把复合缺失当初始无数据，私有scratch仍检查型关闭并清理，输入ZIP保持不变。原docj过滤、字典序、路径与ID映射、公开UI/HTTP/CLI、格式和打包布局未改。

本清单是prepare调用内的元数据观察，不是内容哈希、跨进程CAS或原子快照；最终复核后仍有系统调用间隙，也不提供整个ImportSY回滚。R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为mountArchive的文档候选扫描；mergeArchiveDirectory与资源发布遍历继续独立审查。完整验证与证据见AnonyPower执行计划的`wikiman-r02-import-prepare-scan/results.json`。

### R02：ImportSY mount候选清单（2026-11-01）

本批完成ArchiveImportContext.mountArchive的文档候选扫描准入。原独立fs.WalkDir改为walkScratch受检遍历；mount局部清单捕获所有已扫描目录及docj普通文件的类型、inode、size/mtime，保留隐藏目录整棵跳过、字典序与原路径投影。完整扫描后、merge前及merge返回后复核清单，全部成功才进入原Load、registry与主索引队列。

故障回归覆盖后续目录读取不完整、扫描期间候选替换和merge返回后候选替换：前两者不进入merge，后者允许原顺序merge副作用但不准入索引。既有晚期scratch关闭失败语义保持，已准入索引仍Flush后返回关闭错误；不宣称回滚。原缺失父文档修复、隐藏文档只复制不索引、根/嵌套目标、索引失败排空及成功通知顺序均通过。

本清单不是内容哈希、跨进程CAS、原子快照或整个ImportSY回滚；mergeArchiveDirectory本身仍使用独立遍历，其顺序写入和部分副作用尚未完成受检改造。R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为mergeArchiveDirectory的源枚举、读取和目标发布准入，资源发布遍历继续独立审查。完整验证与证据见AnonyPower执行计划的`wikiman-r02-import-mount-scan/results.json`。

### R02：ImportSY目录合并两阶段发布（2026-11-01）

本批完成mergeArchiveDirectory的受检源枚举与发布准入。第一阶段复用walkScratch完整扫描全部目录和普通文件，记录类型、inode、size/mtime并复核整份清单；扫描、目录关闭或身份失败时尚未打开目标写root，不产生目标目录或文件。第二阶段按原字典序逐项复核源、创建并检查目标目录、受控读取文件、比较目标现状并通过原Rewrite CAS覆盖；全部发布后再次复核源清单。

故障回归覆盖后续目录读取不完整、扫描中早期文件替换、读取后替换、后续文件部分读取和最终清单发现早期源替换。前三类发布前失败保持目标为空；进入发布后的后续失败保留已成功前缀，符合原顺序副作用。既有目标链接/目录/权限拒绝、外部版本保护、相同内容跳过、空目录、关闭错误及资源/数据/AV调用链保持。

该两阶段模型不是全量预读、内容哈希、跨进程CAS或多文件事务；目标晚失败、源晚变化及关闭失败仍可能在已有成功写入后返回错误，不承诺回滚。R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为publishArchiveResources的资源目录发现、完整扫描和成功合并后删除准入；sanitizeArchiveEmojis的递归枚举继续独立核对。完整验证与证据见AnonyPower执行计划的`wikiman-r02-import-merge-directory/results.json`。

### R02：ImportSY资源发现与删除准入（2026-11-01）

本批完成publishArchiveResources的资源目录发现和成功合并后删除准入。原fs.WalkDir改为walkScratch受检遍历；资源局部清单记录所有已遍历目录及精确命名的assets/emojis候选，保持字典序和候选目录SkipDir。完整发现及目录身份复核成功后才逐候选调用已加固merge，merge返回、owner guard和候选目录身份再次成功后才RemoveAll。

故障回归覆盖后续目录读取不完整、扫描期间候选替换、merge失败、merge后候选目录替换和RemoveAll失败。发现失败零merge/零删除；merge或身份失败保留对应scratch；删除失败返回错误且保留本测试未删除的scratch，已合并目标不回滚。原精确目录选择、嵌套/空目录、资源覆盖、链接/权限拒绝、部分合并和关闭错误保持。

候选目录身份复核不是子树内容哈希，merge返回到删除仍有系统调用间隙；RemoveAll可能部分删除并返回首错，不宣称原子删除或整批回滚。R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为sanitizeArchiveEmojis递归目录枚举、改名清单与碰撞/部分改名边界；其他R02依赖继续串行。完整验证与证据见AnonyPower执行计划的`wikiman-r02-import-resource-discovery/results.json`。

### R02：ImportSY emoji逐层改名准入（2026-11-01）

本批完成sanitizeArchiveEmojis递归目录枚举与改名准入。每层先通过Source.ReadDir完成受检枚举，再捕获全部直接子项的类型、inode、size/mtime，计算原名到过滤名的完整计划并预检同层清洗名碰撞及既有目标；该层计划完整成功前不执行任何改名。

执行仍保持字典序与原先的顺序副作用：逐项复核身份后改名，目录使用改名后的路径递归。枚举不完整、链接/类型变化或碰撞使当前层零改名；进入执行后的后续rename或更深层失败会保留已成功前缀，不做整棵回滚。身份检查不是内容哈希或跨进程CAS。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为extractArchive解压根目录的直接ReadDir与单顶层目录准入核对。完整验证与证据见AnonyPower执行计划的`wikiman-r02-import-emoji-sanitize/results.json`。

### R02：ImportSY解压根目录选择准入（2026-11-01）

本批完成extractArchive解压根目录的受检枚举。原直接fs.ReadDir改为Source.ReadDir，完整读取、关闭、取消及根身份检查成功后才判断唯一顶层目录；选中目录随后捕获类型、inode、size/mtime并在返回前再次复核。

读取不完整或选中目录被替换均返回原错误链并释放本次owned scratch，输入ZIP保持不变。原单顶层目录格式、文档归档拒绝data备份、data archive兼容、显示根名称和后续prepare语义保持；目录身份不是子树内容快照或跨进程CAS。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。ImportSY生产代码中的直接fs.ReadDir/fs.WalkDir已清零；下一项为Markdown导入源扫描的直接fs.WalkDir受检迁移。完整验证与证据见AnonyPower执行计划的`wikiman-r02-import-extract-root/results.json`。

### R02：Markdown本地源受检清单（2026-11-02）

本批完成markdown_import_source_context源目录扫描迁移。原fs.WalkDir改为共享walkScratchWithIO，逐目录通过Source.ReadDir完成分批读取、EOF、关闭、取消和目录身份检查；捕获根、目录与普通文件的类型、inode、size/mtime，完整扫描后复核整份清单才交付源对象。

后续目录读取不完整或扫描后期替换早期文件时不返回部分inventory，失败root按捕获的关闭能力只释放一次。原目录/单文件选择、根显示名、隐藏项、字典序、Markdown扩展过滤、source read及后续资产处理规则保持；清单不是内容哈希或跨进程CAS。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。Markdown源入口直接fs.WalkDir已移除；下一项为单文件导入附件目录复制的直接fs.WalkDir受检迁移。完整验证与证据见AnonyPower执行计划的`wikiman-r02-markdown-source-walk/results.json`。

### R02：Markdown单文件附件目录受检复制（2026-11-02）

本批完成markdown_import_assets_context.copyDirectory单文件分支的目录扫描迁移。原直接fs.WalkDir改为共享walkScratchWithIO；每层经Source.ReadDir完成分批读取、EOF、关闭、取消和目录身份检查，回调捕获目录与普通文件的类型、inode、size/mtime并拒绝链接、特殊文件及类型漂移。

整份清单扫描及逐项复核成功后才创建目标附件目录，随后仍按字典序顺序创建目录和复制文件。后续目录读取不完整或扫描后期替换已捕获文件均在发布前失败，目标目录不存在；原空目录、相对名称、权限/mtime和成功前缀语义保持，进入复制后的晚失败不做递归回滚。

该清单不是内容哈希或跨进程CAS，也不承诺多文件事务。R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。Markdown导入生产代码的直接fs.ReadDir/fs.WalkDir调用已清零；下一项为archive_export_resources.go的归档导出资源枚举受检迁移。完整验证与证据见AnonyPower执行计划的`wikiman-r02-markdown-asset-walk/results.json`。

### R02：归档导出资源受检清单（2026-11-02）

本批完成archive_export_resources的两处直接fs.ReadDir迁移。资源索引按笔记本顺序受检扫描，捕获目录与文件的类型、inode、size/mtime；全部笔记本及全局assets清单再次复核后才构造索引，保持最后笔记本匹配、全局资源优先、隐藏/保留项过滤和嵌套assets规则。

选中的目录资源在目标创建前完成独立受检清单与整份复核，随后按原字典序顺序创建空目录、复制文件并逆序恢复目录mtime。索引或复制枚举不完整、扫描后期替换早期文件均在目标目录创建前失败；发布开始后的晚读取/写入失败仍可保留成功前缀，不做递归回滚。

该身份清单不是内容哈希、跨进程CAS或整批导出事务。R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。archive_export_resources.go生产代码的直接fs.ReadDir已清零；下一项为archive_export_output.go的归档打包WalkDir/ReadDir受检迁移。完整验证与证据见AnonyPower执行计划的`wikiman-r02-archive-export-resource-enumeration/results.json`。

### R02：归档输出受检成员清单（2026-11-02）

本批完成writeOwnedArchive的直接fs.WalkDir/fs.ReadDir迁移。打包先通过Source.ReadDir按字典序建立根、目录与普通文件清单，捕获类型、inode、size/mtime并标记空目录；整份清单复核成功后才创建zip.Writer及ZIP成员。

成员仍按原先的深度优先字典序写入，只为空目录创建目录头，保留可选根名前缀、Deflate、文件权限和mtime。文件打开、流式复制、关闭及写后身份复核保持；最终再复核整份清单。枚举不完整或扫描后期替换早期文件时输出保持零字节，发布期写入/关闭失败仍由私有archive.part和输出owner阻止最终发布。

该清单不预读正文，也不是内容哈希、跨进程CAS或通用io.Writer回滚。R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。archive_export_output.go生产代码的直接WalkDir/ReadDir已清零；下一项为markdown_export_package.go的Markdown导出包资源枚举受检迁移。完整验证与证据见AnonyPower执行计划的`wikiman-r02-archive-output-walk/results.json`。

### R02：Markdown导出包资源枚举准入（2026-11-02）

本批完成markdown_export_package的两处直接fs.ReadDir迁移。文档子项判断使用受检单层读取，捕获并在读取后复核父目录类型、inode、size/mtime；读取不完整不降级为“无子项”，读取返回后目录替换返回ErrSourceChanged。

选中目录资源复用归档导出完整清单，在目标创建前完成全树扫描和整份复核；随后仍按字典序创建空目录、复制普通文件并逆序恢复目录mtime。原全局资源覆盖笔记本资源、注解资源覆盖索引、片段/查询处理、缺失资源跳过及隐藏内容保留不变。预检失败不创建所选目标目录，发布期晚失败仍可保留成功前缀。

该清单不是内容哈希、跨进程CAS或整包事务。R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。markdown_export_package.go生产代码的直接fs.ReadDir已清零；下一项为markdown_export_annotation.go的PDF注解源目录枚举受检迁移。完整验证与证据见AnonyPower执行计划的`wikiman-r02-markdown-export-package-enumeration/results.json`。

### R02：Markdown注解资源枚举准入（2026-11-02）

markdown_export_annotation的笔记本资源搜索现以Source.ReadDir替换直接fs.ReadDir。每个实际访问目录在读取前捕获类型、inode、size/mtime，读取完成后重新复核；不完整读取保留ErrSourceReadIncomplete原因链，读取返回后父目录被替换返回ErrSourceChanged。

搜索仍按目录项字典序递归并在首个后缀匹配时返回，不为已经命中的资源扫描后续无关目录；全局资源优先、笔记本顺序、`.sya`解析、无注解跳过和源只读语义不变。新增确定性故障注入只用于测试，不扩大公开接口。

R02总体仍doing，R03–R10仍todo；公开UI、HTTP/CLI、ZIP格式、帮助与打包布局未变。markdown_export_annotation.go生产代码的直接fs.ReadDir已清零，下一项为html_export_resources_context.go的资源根枚举准入。完整验证与证据见AnonyPower执行计划的`wikiman-r02-markdown-export-annotation-enumeration/results.json`。

### R02：HTML设备资源根枚举准入（2026-11-02）

html_export_resources_context的设备资源根复制现以Source.ReadDir替换直接fs.ReadDir。所选根在枚举前捕获当前路径身份，完整读取后复核类型、inode、size/mtime；枚举成功后才创建目标根。不完整读取保留ErrSourceReadIncomplete原因链，读取返回后根路径被替换返回ErrSourceChanged。

该准入只覆盖顶层发现；每个顶层子项仍复用copyPath的目录清单与普通文件复制，保留字典序、隐藏/空目录、mtime、主题/图标/emoji选择、可选缺失及发布期晚失败成功前缀。不会为未选择设备根建立整树清单。

R02总体仍doing，R03–R10仍todo；公开UI、HTTP/CLI、ZIP格式、帮助与打包布局未变。html_export_resources_context.go生产代码的直接fs.ReadDir已清零，下一项为html_export_tree_context.go的HTML树枚举准入。完整验证与证据见AnonyPower执行计划的`wikiman-r02-html-resource-root-enumeration/results.json`。

### R02：HTML合并树按需枚举准入（2026-11-02）

html_export_tree_context的merge服务现以Source.ReadDir替换直接fs.ReadDir。文档树每次实际请求目录时捕获父目录类型、inode、size/mtime，完整读取后复核；不完整读取保留ErrSourceReadIncomplete原因链，读取返回后目录替换返回ErrSourceChanged，失败不交付部分投影。

枚举仍由doctree按需驱动，不预扫未访问分支；保留reserved/`.tmp`跳过、隐藏文档、原字典序、笔记本/全局排序、mergeMax、引用计数和源只读。私有ReadDir回调仅供确定性故障测试。

R02总体仍doing，R03–R10仍todo；公开UI、HTTP/CLI、ZIP格式、帮助与打包布局未变。html_export*.go生产代码的直接fs.ReadDir/fs.WalkDir已清零，下一项为html_desktop_context.go的目录遍历准入。完整验证与证据见AnonyPower执行计划的`wikiman-r02-html-tree-enumeration/results.json`。

### R02：HTML桌面导出预检完整枚举准入（2026-11-02）

html_desktop_context的目标预检现复用archiveExportContext.resourceManifest，以Source.ReadDir替换直接fs.WalkDir。私有输出根先完成整树枚举与最终身份复核，随后才检查目标index、路径与目录/普通文件碰撞；读取不完整或已读源文件被替换时不会进入复制。

resourceManifest补齐独立根`.`的受检语义：根本身通过os.Root身份快照复核，后代仍逐路径Source.Check，链接与非普通文件继续拒绝。原字典序、输出布局、目标保留、源只读和复制阶段成功前缀边界不变；本批未合并后续copyHTMLDirectory的第二次顶层枚举。

R02总体仍doing，R03–R10仍todo；公开UI、HTTP/CLI、ZIP格式、帮助与打包布局未变。html_desktop_context.go生产代码的直接fs.WalkDir已清零，下一项为html_handoff_context.go的顶层复制枚举准入。完整验证与证据见AnonyPower执行计划的`wikiman-r02-html-desktop-preflight-enumeration/results.json`。

### R02：HTML目录交付完整枚举准入（2026-11-02）

html_handoff_context的copyHTMLDirectory现先通过resourceManifest从独立根`.`完整递归枚举并最终复核全部目录和文件身份，再按原顶层字典序调用copyPath。根目录读取不完整或扫描期间先前文件被替换时，复制不会开始，目标保持为空。

该方案保留原copyPath作为实际发布端口，因此链接/非普通文件拒绝、空目录、mtime、目标覆盖检查和逐项晚失败成功前缀不变；代价是子目录在准入后由copyPath再次扫描。本批不宣称内容哈希、跨进程CAS或整目录回滚。

R02总体仍doing，R03–R10仍todo；公开UI、HTTP/CLI、ZIP格式、帮助与打包布局未变。html_handoff_context.go生产代码的直接fs.ReadDir已清零，下一项为conflict_sort_context.go的目录枚举准入。完整验证与证据见AnonyPower执行计划的`wikiman-r02-html-handoff-enumeration/results.json`。

### R02：冲突排序按需目录枚举准入（2026-11-02）

conflict_sort_context的doctree List/Children现以Source.ReadDir替换直接fs.ReadDir。每个实际请求目录在读取前捕获身份，完整读取后复核类型、inode、size/mtime，并逐项执行Source.Check；不完整读取或回调返回后目录被替换时，不交付部分排序输入。

私有IO端口只用于确定性注入目录读取和通知。失败发生在sort.json Rewrite及filetreeSortChanged之前，保留旧排序文件且不通知；原按需递归、MaxListCount、reserved/tmp过滤、排序模式、引用计数、源只读与成功通知保持。

R02总体仍doing，R03–R10仍todo；公开UI、HTTP/CLI、排序JSON、帮助与打包布局未变。conflict_sort_context.go生产代码的直接fs.ReadDir已清零，下一项为index_fix.go的索引订正遍历准入。完整验证与证据见AnonyPower执行计划的`wikiman-r02-conflict-sort-enumeration/results.json`。

### R02：索引订正完整清单准入（2026-11-02）

index_fix.go的重复块重置与文件树订正现先通过Source.ReadDir按原字典序深度遍历每个笔记本，捕获全部目录和普通文件身份，并在完整扫描后统一复核。读取不完整、目录或已枚举文件被替换时，均在非法文件归档、文档重写或索引差异处理前失败。

清单保留隐藏目录跳过、docj/资源过滤与原遍历顺序；修复仍按笔记本逐本执行，不提供跨全部笔记本事务、内容哈希或跨进程CAS。私有readDir端口只用于确定性故障注入。

R02总体仍doing，R03–R10仍todo；公开UI、HTTP/CLI、索引格式、帮助与打包布局未变。index_fix.go生产代码的直接fs.WalkDir已清零，下一项为incremental_index_context.go的增量索引遍历准入。完整验证与证据见AnonyPower执行计划的`wikiman-r02-index-fix-enumeration/results.json`。

### R02：增量索引目录完整清单准入（2026-11-02）

incremental_index_context的目录路径展开现以入口私有的Source.ReadDir清单替换直接fs.WalkDir。选中目录按原字典序深度遍历，捕获根、全部目录和普通文件身份；完整扫描与最终复核成功后才投影docj路径，因此后段读取不完整或先前候选被替换时不交付部分增量输入。

清单保留隐藏目录参与、文档扩展名过滤、输入去重与原顺序。后续incrementalIndex的remove/upsert/flush顺序及既有部分副作用边界不变；不提供内容哈希、跨进程CAS或索引更新事务。

R02总体仍doing，R03–R10仍todo；model生产代码的直接fs.WalkDir/fs.ReadDir调用已清零，公开UI、HTTP/CLI和索引格式未变。下一项继续审查internal/indexrepair/notebook.go的直接遍历及R02主线剩余门槛。完整验证与证据见AnonyPower执行计划的`wikiman-r02-incremental-index-enumeration/results.json`。

### R02：笔记本文档完整清单准入（2026-11-02）

internal/indexrepair的Source.Documents现以私有Source.ReadDir完整清单替换直接fs.WalkDir。笔记本根、合法ID目录和候选docj文件在字典序深度遍历后统一复核身份；后段读取不完整或先前候选被替换时返回nil文档集，不进入Load、索引队列、Flush或历史初始化。

原隐藏/.siyuan/assets/非法目录跳过、合法docj名称过滤、链接拒绝和顺序保持。该清单只准入文档路径，不预读正文，也不改变后续并发加载、修复写入、队列与历史完成边界。

R02总体仍doing，R03–R10仍todo；internal/indexrepair/notebook.go生产代码的直接fs.WalkDir已清零，公开UI、HTTP/CLI和索引格式未变。下一项审查internal/assetcache/cache.go及R02主线剩余门槛。完整验证与证据见AnonyPower执行计划的`wikiman-r02-notebook-documents-enumeration/results.json`。

### R02：资产缓存两阶段清单准入（2026-11-02）

internal/assetcache的Cache.Load现以私有fs.ReadDir两阶段清单替换直接fs.WalkDir。根、已访问目录和可见普通文件在完整扫描后统一复核；生产文件系统具备系统身份时使用os.SameFile，通用fs.FS回退到类型、大小和mtime。失败时旧资产快照与哈希缓存均保持。

根计入10万项预算、缺失根视为空、字典序递归、隐藏目录/点文件/`.sya`/调用者隐藏过滤、链接与非普通文件拒绝保持。只有完整清单成功后才在原锁内替换assets并清空hashes。

R02总体仍doing，R03–R10仍todo；internal/assetcache/cache.go生产代码的直接fs.WalkDir已清零，公开UI、HTTP/CLI和缓存格式未变。下一项审查internal/sourcetx/project.go的直接遍历。完整验证与证据见AnonyPower执行计划的`wikiman-r02-asset-cache-enumeration/results.json`。

### R02：内容投影全页面清单准入（2026-11-02）

internal/sourcetx的内容投影现先为全部页面数据树建立完整清单并重复确认，再开始documents/blocks/refs事务写入。每个候选docj打开后核对生产文件身份，全部SQL处理结束后再重建并比较清单；目录读取不完整、候选被同大小同mtime替换、扫描期间新增或删除时，候选事务整体回滚且不发布page.db。

缺失页面data目录跳过、隐藏目录整棵忽略、链接拒绝、合法box/docj过滤、字典序和10万文件/512MiB预算保持。该边界不提供跨进程CAS，也不改变公开UI、HTTP/CLI、索引schema或源文件格式。

R02总体仍doing，R03–R10仍todo；internal/sourcetx/project.go生产代码的直接fs.WalkDir已清零，字面未勾选仍为44。下一项审查internal/sourcetx/read.go的直接遍历。完整验证与证据见AnonyPower执行计划的`wikiman-r02-sourcetx-project-enumeration/results.json`。

### R02：源文件读取完整清单准入（2026-11-02）

internal/sourcetx的Files与Document现共用私有受检目录清单器。两次完整清单一致后才读取文件，打开文件时核对生产身份，全部哈希或文档读取结束后再比较最终清单；后段枚举失败、同大小同mtime替换、扫描期间增删均不返回部分文件列表或混合版本文档。

Files继续递归包含隐藏目录内容，Document继续跳过隐藏目录；两者都把根计入10万项预算、拒绝任意所见链接并保持字典序。Files的512MiB总量/单文件64MiB、Document的16MiB限制和缺失根语义保持。

R02总体仍doing，R03–R10仍todo；internal/sourcetx/read.go生产代码的两处直接fs.WalkDir已清零，字面未勾选仍为44。下一项审查internal/sourcetx/transaction.go的直接遍历。完整验证与证据见AnonyPower执行计划的`wikiman-r02-sourcetx-read-enumeration/results.json`。

### R02：事务条目文档稳定清单准入（2026-11-02）

internal/sourcetx的候选事务不再边fs.WalkDir边判断entryDocument。条目目录先生成两次路径、类型、大小、mtime和SameFile身份一致的完整清单，再叠加本次changes并验证目标docj唯一；第二轮枚举失败、同大小同mtime的新inode替换、链接或非普通同名项均在journal和源发布前拒绝。

隐藏目录继续参与唯一性判断，缺失目录可由本次新增文件补足，原事务入口不新增条目数上限。共享清单的上限改为显式选项，Files与Document仍保持10万项预算。该边界仍依赖现有源锁合作，不声称跨进程CAS。

R02总体仍doing，R03–R10仍todo；internal/sourcetx/transaction.go生产代码的直接fs.WalkDir已清零，字面未勾选仍为44。下一项审查internal/sourcetx/recovery.go的journal目录枚举。完整验证与证据见AnonyPower执行计划的`wikiman-r02-sourcetx-transaction-enumeration/results.json`。

### R02：恢复 journal 单层稳定清单准入（2026-11-02）

internal/sourcetx的journalIDs不再以单次fs.ReadDir决定普通读取准入和恢复集合。`.local/journal`现执行两次完整、SameFile身份一致的单层清单，再从稳定快照验证json/commit配对、临时文件、类型、名称和20万子项上限；目录读取中断或同大小同mtime新inode替换不交付部分operation ID。

单层选项保证非法子目录被记录后拒绝而不递归进入。缺失journal目录、字典序、`.wiki-tmp-*`忽略、孤立commit、目录/链接拒绝和后续loadJournal/Recover发布顺序保持。该边界仍不读取journal内容，也不声明跨进程CAS。

R02总体仍doing，R03–R10仍todo；sourcetx业务代码的直接fs.ReadDir/fs.WalkDir已清零，字面未勾选仍为44。下一项审查internal/appearance/owner.go的直接目录遍历。完整验证与证据见AnonyPower执行计划的`wikiman-r02-sourcetx-recovery-enumeration/results.json`。

### R02：外观资源稳定清单与安装预检（2026-11-02）

internal/appearance的catalog与bundle安装现共用package私有os.Root清单。catalog对每个单层目录完成两次路径和生产身份一致的枚举并成功关闭根后才合并；安装对整个bundle完成两次递归清单后才创建目标，提前拒绝链接、非普通文件和预算超限。

顺序复制期间逐文件复核可见、打开和复制后身份，立即传播输入/输出关闭错误，并在结束时再次比较源清单。installed custom、bundled同名覆盖、隐藏过滤、字典序、10万项/512MiB/2MiB限制和晚失败成功前缀保持；不声明原子树交换或跨进程CAS。

R02总体仍doing，R03–R10仍todo；appearance业务代码直接fs.ReadDir/fs.WalkDir已清零，字面未勾选仍44。下一项审查internal/workspace/emoji.go。完整验证与证据见AnonyPower执行计划的`wikiman-r02-appearance-enumeration/results.json`。

### R02：自定义 emoji 稳定清单准入（2026-11-03）

internal/workspace的自定义emoji发现现通过独立目录句柄分批读取根和一层子目录，核对打开目录、路径及条目的生产身份并检查关闭错误。两次完整字典序清单一致后才生成映射并一次替换本workspace的EmojiRegistry；失败保留旧集合。

一层递归、深层目录忽略、隐藏项、非法名/链接拒绝、同键扩展名字典序覆盖、缺失目录和10000键预算保持。清单不读取emoji文件内容，也不声明跨进程CAS。

R02总体仍doing，R03–R10仍todo；emoji.go直接fs.ReadDir已清零，字面未勾选仍44。下一项审查internal/workspace/native_avquery.go。完整验证与证据见AnonyPower执行计划的`wikiman-r02-emoji-enumeration/results.json`。

### R02：Native AV 查询一致清单（2026-11-03）

internal/workspace的AV搜索现以受检单层清单读取`storage/av`。两次完整目录快照一致后，`blocks.msgpack`和候选JSON按清单身份打开、读取和关闭；名称与views共享一次解析结果，查询结束的最终清单一致后才交付结果。

无关项、100000项预算、16MiB限制、updated排序、12项上限、exclude及缺失目录/关联文件空结果保持。失败不返回部分结果，但不声明回滚已完成的派生blocktree或跨进程CAS。

R02总体仍doing，R03–R10仍todo；native_avquery.go直接fs.ReadDir已清零，字面未勾选仍44。下一项审查internal/workspace/native_blocktree.go。完整验证与证据见AnonyPower执行计划的`wikiman-r02-native-avquery-enumeration/results.json`。

### R02：Workspace 只读投影枚举批次（2026-11-03）

本批一次完成块树重建、文档子文件统计和文档树导航三处直接枚举。共享底层以独立目录句柄分批读取，显式检查关闭、路径与条目身份；调用方保留各自过滤和预算。块树建立两次一致的递归清单，解析完成后在替换索引前再确认，失败继续保留旧索引；文档信息与导航使用两次一致的单层快照，导航元数据不再另行重取。

部分读取、关闭失败、两次确认间变化、链接目录和块树最终确认失败均已覆盖。原隐藏/保留项、缺失子目录、`.docj`名称计数、字典序、10万扫描项与100万投影行边界保持；不声明跨进程CAS。

R02总体仍doing，R03–R10仍todo；三份业务文件的直接fs.ReadDir/fs.WalkDir已清零，字面未勾选仍44。workspace剩余4处直接枚举，下一批从native_document_lifecycle.go开始。完整验证与证据见AnonyPower执行计划的`wikiman-r02-workspace-read-enumeration/results.json`。

### R02：文档生命周期与闪卡枚举批次（2026-11-03）

本批一次完成文档生命周期递归子文档、`storage/riff`牌组和笔记本根文档三处直接枚举。生命周期服务使用两次一致的递归清单，并在Prepare/Create完成后、计划交付前对所有已访问根执行最终确认；变化时返回空计划。牌组与笔记本列表使用两次一致的单层快照，传播部分读取和关闭错误，并拒绝有效候选链接。

原缺失子文档根空集合、递归`.docj`投影、10000文档限制、牌组/笔记本过滤与排序保持。目录清单只稳定枚举和元数据，不预读所有文档或牌组内容，也不声明跨进程CAS。

R02总体仍doing，R03–R10仍todo；三份业务文件的直接fs.ReadDir/fs.WalkDir已清零，字面未勾选仍44。workspace生产代码只剩`internal/workspace/read.go`一处直接枚举，作为最终全树读取批次继续处理。完整验证与证据见AnonyPower执行计划的`wikiman-r02-lifecycle-flashcard-enumeration/results.json`。

### R02：Workspace 最终源读取清单（2026-11-03）

`readSource`不再直接fs.WalkDir。目标笔记本先完成两次一致的递归清单，保持隐藏目录整棵跳过、全树10万项预算和目标文档唯一性；候选文件按清单身份打开、分块读取并检查关闭，JSON与文档ID验证通过后再比较最终全树清单。

目录或文件部分读取、文件关闭失败、候选打开前被同元数据新文件替换、读取后源树变化均不交付字节或路径。16MiB上限、未知AST字段原字节保真、嵌套文档路径和可见重复拒绝保持；不声明跨进程CAS。

R02总体仍doing，R03–R10仍todo；workspace业务生产代码的直接fs.ReadDir/fs.WalkDir已清零，剩余ReadDir仅为受检清单器底层端口。字面未勾选仍44，本项此前只记录为下一步。完整验证与证据见AnonyPower执行计划的`wikiman-r02-workspace-final-read/results.json`。

### R02：Legacy 后台生命周期原链路验收（2026-11-03）

task历史生成、cron、assets/emojis watcher、Context事件订阅及legacy初始化现按真实owner生命周期验收。注册失败会关闭已建事件scope、缓存并清空全局owner，允许原入口重试；watcher提供带Context的checked start/close，关闭超时保留owner，活跃回调返回后可再次排空。非Darwin fsnotify实现同步完成目录与Add准入，并等待事件循环退出。

竞态整包、45个conf/internal包、Linux交叉编译、静态检查、三个Go入口、17项集成用例和受控浏览器均通过。R02仍doing；本项真实清单已勾选，字面未勾选由44降至43。证据见AnonyPower执行计划的`wikiman-r02-legacy-lifecycle-acceptance/results.json`。

### R02：导入与源 IO 历史清单归一化（2026-11-03）

重新从原入口核对ImportData、ImportFromLocalPath、普通/ZIP Markdown、HTML图片/粘贴/剪藏、PDF设备/文件控制器及checked source root关闭链。当前生产代码旧`openRepairIO`调用和忽略错误的`defer closeIO()`均为0；这些实现已由后续连续批次完成，本批用当前源码重新执行原入口和故障竞态后，归一化14条仍显示未完成的历史重复项。

复合项中的其余writer/缓存/广播/历史/快照/后台缺口仍拆回TODO，未随已完成前缀一起勾选。R02仍doing，字面未勾选由43降至29。证据见AnonyPower执行计划的`wikiman-r02-import-source-io-normalization/results.json`。

### R02：Task/Job/Context订阅拒绝窗口（2026-11-03）

异步ContextTask出队后若退出、取消或owner拒绝，现会完成取消而非遗留waiter；cron的task/async/status tick使用scheduler Context。LocalBus支持Context自动注销与活跃回调join，workspace索引/SQL进度入口已接入。assets/emojis watcher也在owner替换和发布窗口检查取消，候选初始化失败或取消不会发布半开owner。

专项十轮、相关整包、全仓分层竞态、vet、Linux非Darwin编译、四入口和compat退出通过。该批只推进B01/E01共享生命周期与两个实际订阅入口，不代表其余compat订阅/业务回调完成；R02仍doing。证据见AnonyPower的`wikiman-r02-task-job-subscription-lifecycle/results.json`。

### R02：短生命周期后台任务接续（2026-11-03）

`Doc2Heading`的延迟引用刷新现进入捕获AppConf的repository任务组，等待可取消且关闭可join。用户指南延迟打开和设置后的UI reload由legacy runtime owner托管；SSE订阅计数改为同步锁保护和defer配对清理，不再启动无值计数goroutine。

专项十轮、最终`model/api`全量及全仓分层竞态、vet、Linux编译和四入口构建通过。共享fixture仍在本批路径前返回既有`not_found`，结果为17/18与0/1。该批只推进B01/E01子项，其他后台回调与`MoveLocalShorthands`仍待；R02保持doing。证据见AnonyPower的`wikiman-r02-short-background-lifecycle/results.json`。

### R02：CalDAV/CardDAV 请求与并发工作接续（2026-11-03）

两套DAV backend的22个Context接口现传播请求取消；集合、日历/地址簿和对象的9处手写goroutine/WaitGroup统一为最多8个文件worker的错误任务组，返回前join并传播子错误。失败不清dirty，多vCard拆分按实际新文件登记独立身份且不嵌套放大并发。

专项十轮、最终全量及分层竞态、vet、Linux编译和四入口构建通过；共享fixture保持已知17/18与0/1 `not_found`基线。全局DAV实例/路径归属和多文件回滚未完成，C01/Q01/T01/B01/E01继续保留，R02仍doing。证据见AnonyPower的`wikiman-r02-dav-context/results.json`。

### R02：CalDAV/CardDAV 显式所有者接续（2026-11-03）

服务启动新增`DAVStore`，构造时固定绝对数据根并独立持有Calendars/Contacts；两套生产backend现在必须注入owner。元数据、集合和对象路径均从owner解析，不再随`util.DataDir`变化；默认元数据深拷贝，双根同ID和全局根切换隔离已覆盖。同步修复CardDAV父路径归一化与Calendar查询重复加锁。

专项十轮、全仓竞态、既定vet、Linux编译、四入口/Wiki App构建、18/18非浏览器和完整浏览器1/1通过。此前DAV全局实例/路径归属缺口已关闭；多workspace路由、抗symlink句柄遍历、多文件回滚及R04/R05源事务仍待，C01/Q01/T01/B01/E01继续保留，R02仍doing。证据见AnonyPower的`wikiman-r02-dav-owner/results.json`。

### R02：服务辅助生命周期所有者接续（2026-11-03）

UI-loaded探测现在由legacy task owner持有并可取消。固定端口代理改为跨workspace切换存活的进程级legacy service：启动前冻结host、端口、target与delay，使用可取消延迟、显式listener和`http.Server`，退出时优雅排空、超时强制关闭并join后释放端口。重复配置幂等，冲突替换与监听失败明确。

专项5项十轮、全仓竞态、既定vet、Linux编译、四入口/Wiki App、18/18非浏览器及完整浏览器1/1通过。主HTTP/WebSocket生命周期和其他后台/事件仍待；C01/Q01/T01/B01/E01继续保留，R02仍doing。证据见AnonyPower的`wikiman-r02-server-aux-owner/results.json`。
