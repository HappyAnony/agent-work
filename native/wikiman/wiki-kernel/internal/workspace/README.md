# S03 workspace resource isolation

## Original edit candidates

`Lease.PlanNativeEdits` uses the original editor core and stages all operations
against one private document, database-view and SQL projection. Subsequent
operations see earlier candidates in the same batch. Index renderers receive
detached trees so temporary layout calculations cannot leak into source files.
Only the edit service may pass the candidate, captured file hashes and directory
membership to the durable writer. A commit waits for actual original index
availability before a normal editor acknowledgement.

## Current R03 acceptance — 2026-11-05 (done; 11/11, zero remaining TODO)

Native handlers explicitly opt into pure original read settings and source-bound
notebook/language ports; bare Options{} still supplies no implicit configuration.
The shared wikiops query registry composes 68 original reads under one admitted
store/page identity. Raw/unknown AST bytes remain independent of original parser
indexing. A private main/blocktree candidate is adopted only after complete
projection and final source verification; history/asset-content owners survive.
Missing tables and changed/deleted source are rebuilt, with copied cache results
and real cancellation. Attachment content is rehydrated lazily through the same
joined original converters. Template matching and image/panel/recent-block
projections are shared with actual legacy callers.

The full inventory maps 106 original caller records (90 read mappings and 16
explicit edit/history/device/transport boundaries), with zero unresolved rows.
The AnonyPower execution plan and its `wikiman-r03-completion` evidence directory
track final acceptance. See the [shared contract](../wikiops/README.md#original-readquery-services-r03).
Dated pending notes below do not override the current phase tracker.


## Current R02 acceptance — 2026-11-05

R02 is complete: zero remaining TODOs, 15/15 closure work packages accepted.
The closed semantic inventory reconciles 188 API and 392 non-API baseline rows
against current definitions/callers; the expanded scan covers 3,873 production
functions, with zero unresolved rows. Compatibility foreground requests pin the
admitted owner for real response/cleanup completion, nested operations refuse
replacement, and accepted background/WS work has its own cancellation and drain.
Native dependency checks continue to forbid the legacy runtime namespaces.

Final `GOPROXY=off go test -race -tags fts5 ./...` passes, including default vet
(66 tested packages; model 389.079s). Final binaries/frontend build successfully;
9 integration, 2 real browser and 6 export/package checks pass without skips.
Linux and Windows checks are compilation only. Source/binary fingerprints and
successful logs are archived in AnonyPower's
`docs/development/evidence/wikiman-r02-final-completion/{audit,semantic-ledger,results}.json`.
The execution plan in that repository is the authoritative stage tracker.

All dated batch notes below are historical checkpoints. Their “pending”,
“next” and “R02 doing” statements are superseded by this final acceptance;
they do not represent current R02 TODOs. R03–R10 retain their own gates,
including R04 durable source receipts and R06 full UI transport identity.

## Historical implementation and verification notes

R02 resource inheritance update: `WithNativeIndexes` lazily opens a per-page
`internal/indexdb.Bundle` containing the original main/history/asset/blocktree
schemas, shared with the compat SQL/treenode initializers. This is separate from
the existing byte-span tables below. `WithNativeIAL` similarly lends the original
IAL cache; `WithNativeParser` uses original factories with explicit value options.
All three ports pin the exact lease, hold source-read admission and serialize
access; their callbacks must not retain handles or dispatch unowned work. They
do not expose SQL/paths to HTTP, initialize source data or save configuration.
The index/cache owners open only on explicit first use; the resource close gate
joins their handles/workers after leases/tasks drain. Failure cleanup leaves no
published half-open native bundle. The FTS5 tests cover duplicate initialization,
same-ID A/B indexes/caches, source hashes, cancellation, retained-resource drain
and closed-handle rejection. Original query/transaction wiring and full Conf/job
ownership remain pending; the small projection is not native feature parity.

The next R02 lifecycle batch fixes the **compat** AppConf receiver/path binding,
operation-scoped SQL parser, callback unregistration/drain, task/cron/history
ownership and macOS polling-watcher joins. None of those compat facades imports
model/sql into this core, and none proves full original business A/B isolation.
The complete original `conf.AppData` now supplies classified, detached Context
configuration via Options.Configuration; conf no longer imports old runtime
packages. WithConfiguredParser uses the same option mapping as model.NewLute.
WithNativeTransaction pins explicit original-schema derived DB operations shared
with original SQL's low-level connection/transaction gateways. These are internal
ports, not new HTTP endpoints or source save receipts. Original high-level query,
AppConf business methods and performTx dependencies still require migration.
StartPeriodicTask uses the creation lease; Drain/DrainStore cancel registered task
owners without unpinning work until it actually returns. Independent-store
cancellation, no-source-write configuration and four-index A/B rollback tests
cover these integrations. Original source jobs/watchers/events still need their
own contextual services; do not advance R03 based on these resource ports.

R02 business batch: Options.ConfigurationBusiness and lease notebook/language
methods reuse AppConf's original business service, not global Conf. Four original
reference aggregate queries run through WithNativeReferences on the page DB.
HistoryQueue preserves original FIFO/per-operation transactions/512-row inserts;
both the original SQL facade and Context use it, with copied values, rollback,
retry and cancellable serialized flush admission. QueueNativeHistories/Purge,
FlushNativeHistories and StartNativeHistoryFlush implement actual history-index
work on the creating lease. Resource close drains accepted derived work before
closing its DB. None of these operations writes authoritative history/source or
issues a save receipt. Main/asset queues, other queries/caches/jobs and events
remain unmigrated. model.performTx has private explicit load/stage/finish/parser
dependencies and nonempty Context-backed tests for create/update-time paths;
the remaining operations are denied in non-legacy environments. It is not yet
exposed to the new Server/CLI or converged with source transactions (R04).

R02 classified/queue/event batch: Configuration overlays explicit provider data
with validated persisted store, workspace and device preferences, never creating a
missing config. SaveWorkspacePreferences uses the existing source journal/CAS;
SaveStorePreferences uses the same source path and invalidates every page;
SaveDevicePreferences writes only local device state with CAS and writer locks.
All three manager mutations drain live leases before publication; keep no lease while
requesting a save. The classified document replaces its category, not a patch.
Original settings controllers and scoped notebook repair remain pending. See the [configuration contract](../../conf/README.md).

AssetQueue now shares the actual original index/delete-path/batched algorithms
with the legacy SQL facade. Context owns it and one ContentQueue FIFO containing
update_block_content and nine delete/rename families; typed empty-embed and asset-selection queries are
also shared. NativeAssetContents is an internal SQL service, not a public query
endpoint. The original typed asset limit behavior is preserved. Case sensitivity,
all FTS tables, copied inputs, FIFO/coalescing, rollback/retry and close are owned.
QueueNativeMainMutation supports delete, delete_id, delete_ids, rename,
rename_sub_tree, delete_box, delete_box_refs, delete_refs, delete_assets. Values
and batch slices are copied; delete_refs coalesces by tree ID like the original.
A failed tail is restored without replaying committed prefixes. Chunked parameter
lists preserve quoted IDs and SQLite limits. FlushNativeMainQueue and
StartNativeMainFlush execute this same FIFO, not an independent reordering queue.
QueueNativeTree(ctx, box, doc, "index"|"upsert") now reads actual source and runs
the original block/span/asset/attribute and reference/annotation projections with
an explicit parser and page-bound services. All 14 main families use this FIFO.
The write-side hash diff reads the writing transaction; unchanged blocks retain
ROWIDs. Values (all row families and ignore lines) are copied at admission.
Assets preserve doc/box/page precedence and original Qiniu ETag; reads reject
traversal/symlinks and never select another page. Page data/.wiki/indexignore is
read-only; original ignored-upsert deletion order is retained, not silently fixed.

NativeOCR binds bounded synchronous preparation under the source admission/gate;
NativeContent binds OCR text/AV reads. Disabled OCR must explicitly bind no-op
preparation and its chosen text reader. ConfigureNativeOCR is an explicit alternative
binding the shared original engine and owner-local services (see below), never a
global fallback. A failing provider does not publish a Context; missing/failing callbacks
or renderer panic do not enqueue partial rows. Live callbacks remain pinned even
after the creating lease releases; drain timeouts never force-close their resources.

NativeBlock and internal external-definition lookups use an owner-local bounded
copied cache. Every committed main mutation invalidates it before publishing;
committed trees also clear document/block IAL caches. Failures keep old cache
state. Full blocktree and asset-list/hash services are now owner-bound (below);
the upper reference/CacheRef services are now owner-bound (below); other upper
fulltext/AV/tag queries and dynamic-reference source mutation callers remain TODOs.
Native source parsing now uses ParseJSONWithoutFix, preserving empty/old sources
without synthesized paragraphs or Spec upgrades. HPath reads ancestor titles
through the same confined source root. These are derived reads, not source saves.

QueueNativeReferences and QueueNativeIndexNode take explicit box/doc identities,
read bounded actual source through the pinned data root, and use the configured
original parser and shared reference/content projection. The original same-block
reference deduplication, exact embed recognition, Markdown, annotation paths and
content rules remain. Reference definition metadata now comes only from this owner's
source-derived blocktree registry, never the main FTS or another page/global tree.
Tree/reference admission initializes it when needed; bare registry reads do not
silently scan or repair sources. Missing definitions retain original empty metadata.
Source file paths are refreshed even on byte-cache hits, including nested docs.
Records/content are copied into the same FIFO (separate action coalescing keys),
not queued as mutable AST pointers. update_refs transactionally deletes old path
rows and inserts refs/annotations in original 512-row batches; failure restores
the failed tail, not committed prefixes. No authoritative source is changed.

Options.NativeContent supplies bounded identity-bound OCR-text and AV-content/name
readers under this owner's gate/admission. Readers must not reenter the manager or
fall back to legacy globals; missing/failed readers reject affected content before
queue admission. This provider seam alone does not start OCR or AV background
services. Provider failure closes unopened data and publishes no Context.

RebuildNativeBlockTree gathers a complete confined source snapshot before a single
atomic registry replacement, rejects duplicate IDs/symlinks/budget violations, and
leaves the previous registry intact on failure. Legacy treenode blocktree/fix
facades and Context share every blocktree query/index/upsert/remove/path algorithm.
Physical parent metadata is preserved separately from FTS heading parents. Original
incremental Upsert still expands changed containers and leaves deletions explicit;
source-snapshot replacement deliberately removes absent rows. The registry and
main FTS have separate transactions; do not infer cross-database atomicity.
External unmanaged file changes require explicit rebuild; full watcher coherency
is not claimed. Manager source writes still drain/recreate Contexts.

ReloadNativeAssets/NativeAssets use an owner assetcache.Cache with original
RemoveID/hidden/.sya rules, copied values and atomic bounded scans. Hash reads
revalidate current ETags, including SQL fallback, and never select another page.
StartNativeAssetRefresh is an explicit owned periodic refresh, not the complete
original watcher side-effect pipeline.

ConfigureNativeOCR explicitly probes supplied device executable candidates and
languages, including disabled mode; it never installs a tool or probes on Acquire.
Bare executable names use device PATH; no WIKI_* policy is inferred. It binds the
original shared TSV/text/extension/language rules, reads legacy assets/ocr-texts.json
as seed, and validates a versioned local cache without overwriting corrupt input.
RunNativeOCR holds a resource pin but releases source admission/DB gate during the
actual bounded child process. Input is a private copied image; late source hash
or engine changes discard the result. Per-owner busy state, cancellation/child
Wait and temporary cleanup prevent leaked workers or forced resource close.
Each run recognizes at most eight images; queued block refreshes wait across
batches until eligible image hashes/engine signatures are current. The catalog
retains original page-global assets/ scope, not new notebook-local OCR traversal.
Removed asset text is cleaned; real source nodes feed index_node in the existing
FIFO. StartNativeOCRBackground runs recognition, main flush and local cache flush;
task errors are surfaced/stopped for caller inspection/restart, not silently retried.
FlushNativeOCRCache uses atomic local writes at
.local/workspaces/<pageId>/temp/ocr-texts.json, excluded from source backup/Git;
it produces no source journal/receipt and never overwrites legacy source seed.
Independent processes may replace this reconstructible cache, not authoritative
data. The explicit lifecycle below now wires the OCR branch of the original
startup/watcher/status chain, not the complete legacy handler graph or all R02
background/AV/Sync/attachment business.

CacheNativeRef reads a specific reference ordinal from this owner's actual source,
then uses the same ProjectReference as original SQL CacheRef. It does not replace
a single-node operation with a whole-tree projection/de-duplication. Definition
metadata comes from the owner blocktree, and original type normalization is
restored. ReferenceCache preserves grouping by definition/referring block and
sliding 30-minute expiry, copies all values and has no worker. Context has a total
10240-reference budget; compatibility retains the original unlimited policy.
NativeRefsCacheByDefinition preserves original cache-hit/fallback behavior; main
commit clears owner refs, failures retain old cache; Close clears it permanently.
WithNativeReferences now lends upper queries for children/definition/ref IDs,
definition/ref roots, matching, keywords, recent refs and definition-reference
pairs. Original SQL facades use the same algorithms. ReferenceText/ContainerText
share explicit parser/block reads; NativeRefText never reenters its lease gate.
Large positive ID filters are parameterized/chunked; matching exclusions are
applied before the original title limit. Cyclic block parents fail instead of
infinite recursion. Trusted internal search/ignore SQL fragments remain inherited
policy in Recent/DefinitionPairs/VirtualKeywords, not a new public SQL endpoint.
The next approved A batch adds SearchNativeBlocks (original four search modes,
counts/paging/document-mode raw rows), SearchNativeTags/NativeTagSpans and
SearchNativeAttributeViews/NativeAttributeViewName. Each uses owner DB/config or
confined source, not global model. AV search retains original cap-before-orphans,
ranking and view result shape; it requires explicit notebook business binding.
Source AV JSON/blocks.msgpack and `.wiki/searchignore` are read-only, not repaired
or created. SQL search and generated ignore queries reject non-SELECT and multiple
statements.

SearchNativeBlockView now shares the actual original FullTextSearchBlock
presentation/grouping/reference-count core under one source admission/index gate.
It preserves original Block/RiffCard JSON fields, IAL, highlighting, truncation,
document path simplification and sorting (including original timestamp-field
behavior); root/content-order reads use the owner registry/source, never legacy
globals. Source/query failures return no partial UI result. This does not enable
the unported original session HTTP/WS adapter.

NativeAttributeViewKeys shares four original key selectors (IDs, nonrelation,
rollup destinations and relations), including duplicate requested IDs and source
order. Full key JSON/unknown fields are detached, not rewritten into a reduced
schema; malformed input fails rather than being fixed/saved. BuildNativeTags uses
the shared original hierarchy/count/sort/limit core and returns Exceeded for the
future handler. It flushes only owner main work, not original global transactions;
bounded hierarchy failure is explicit, not truncated data.

ReadNativeAttributeView reads a detached complete AV source object without fixing,
upgrading or saving. NativeAttributeViewPrimaryValues shares original primary
keyword/binding/updated-desc/page rules; registry membership comes from the owner's
complete source snapshot (not FTS), mirror IDs use batched lookup/deduplication.
Valid primary block values retain full source JSON, including unknown fields;
malformed/non-block entries in the primary column fail instead of fabricating text
with a reduced universal formatter. Blank names use explicit owner language 105.
Default page size remains 16, out-of-range returns empty, invalid pages fail.

NativeAttributeViewView shares original block-override/current/first selection,
returns the complete selected view and copied raw filters/sorts (empty arrays when
absent), preserving unknown fields. Missing blocks may use the original fallback;
known source read/validation failures do not. These operations run under one
owner admission/gate and return no partial result on error. They do not render
layouts, execute filters/sorts/calculations or mutate the selected view.
Other layout/cell/calculation/relation business, tag block navigation/mutation and
other queries remain pending. Original frontend layout has not changed.

The subsequent R02 table batch adds `NativeAttributeViewTable`: detached Spec 4
source -> shared original `avrender.Table` -> `avmodel.FilterChecked/Sort/Calc`
-> bounded paging. Legacy AV types alias the same original domain types; old SQL
table/basic-auto/query/manual helpers use this implementation too. Bound rows use
this page's registry and pinned AST IAL; no global source reader/cache is imported.
Missing cells are filled on the candidate only, not saved. Errors return no
partial JSON. The later R02 update below adds Spec 0–4 projections, explicit
duration catalogs and field-level dependency resolution; ambiguous/future formats
and dynamic dependencies still fail, not silently return simpler results.
Raw full-source reads remain separate and retain unknown fields. Typed rendered
projections do not claim to reproduce arbitrary unknown wire extensions.

The private original insert seam now requires assets/reference-task/AV registry/
AV binding dependencies before begin and uses scoped lookup/IAL/load/stage.
Parent/previous/next tests use real page leases and candidate-only effects;
this is not a native AV/asset mutation adapter or authoritative source commit.
Formal save/history/journal/receipt stays behind R04; the audited AV configuration,
settings, cell, options and field receivers are candidate operations only. Other
AV receivers remain denied. The execution plan's latest seventy-two-family audit is authoritative.

The next R02 batch adds `NativeAttributeViewRender` for original gallery/kanban
and grouped table/gallery/kanban candidates. It shares `avrender.Gallery/Kanban`,
`GenerateGroups/SortGroups/PrepareGroup/LabelGroups` with actual old callers.
All four cover modes retain original content/asset rules without fetching remote
URLs. Extensionless image detection reads a bounded header of a confined file;
missing/linked sources fail instead of becoming empty successful covers. Bound
tree/IAL lookups are batched and AST cover mutations are restored, including
autoplay tokens. Date/range/value grouping, state preservation and independent
per-group paging/calc use operation-owned clock/language and detached fields.
Group regeneration does not save AV files. A missing group field in the layout,
unbound special-label language, malformed layouts/ranges or exceeded resource
limits returns an error without partial JSON. Template/relation/rollup fields are
covered by the subsequent batch below; explicit durations, static field dependencies
and old format projections are covered by the latest update below.

Original private `update` now binds reference-cache/dynamic-anchor operations,
reference tasks, removed-node AV effects, AV upsert and heading notification sinks.
All seven dependency families are required before begin. Tests stage candidate
effects against real isolated page leases; they are not native write adapters.
R04 must still unify source commit/history/receipt and delayed publication.

PlanNativeDynamicAnchors shares the actual original seven-round propagation,
container/heading reference expansion, text rules and setters. AST candidates and
their source hashes come from the same read bytes; hints are operation-local and
closed afterwards. Changes are pending only: no live ref/index publication,
notification/save/history/receipt. An AV source effect without an audited binding
fails. Source mutation remains behind the R04 unique-commit gate, not a new API.

Other query families, committed dynamic-anchor mutation and the remaining original
transaction receivers still need migration; source save/receipt is not implied.

StartNativeOCRLifecycle is an explicit asynchronous task (check Task errors or
ocr-ready), with per-owner duplicate-start rejection. It configures OCR, seeds
image-bearing source blocks, then opens the original nonrecursive polling watcher
on an existing assets directory; no source directories/defaults are created.
Callback paths are confined to owner assets, and the original assetflow dispatch
order is shared with the actual macOS compatibility watcher. Native OCR events
invalidate seed/hash text, queue current source image blocks, delete removed asset
content rows and remove only a regular reconstructible local thumbnail file;
symlink/foreign thumbnails are rejected. Asset changes publish action/path and
identity outside the gate. The loop flushes main, asset and local OCR cache work;
failure cancels/join-waits watcher workers and releases the lifecycle flag for
explicit retry. Cancellation also joins the actual OCR child and active
notification callback. No timeout falsely closes resources held by such work.
SubscribeOriginalOCRStatus binds the original escaped status-bar format to an
explicit owner sink. Accepted non-stale results publish outside source/DB gates,
so a sink may read its page; subscriptions can be removed and joined. This is
not a global WS route or source receipt. Runtime import/Acquire stays inert.

Original InitConf no longer runs go InitTesseract; the compat bootstrap registers
InitTesseractContext with its owned boot task group. DeviceOptions takes explicit
environment lookup at that boundary only. Old cron recognition/text flush now
uses the shared Cycle and cancellable actual child execution, rather than an
uncancellable readiness wait. Compatibility globals are still single-workspace
state, not used by Contexts or claimed to be a completed original UI adapter.
Corrupt legacy text load now preserves the file/previous map and releases its
lock. Context OCR notifications do not call global PushStatusBar. Full original
Office/PDF/text attachment extraction, IncSync/cloud scheduling, non-macOS legacy
fsnotify lifetime, other watcher types and remaining WS routing remain TODOs;
the native asset event announces changes but does not silently run those services.

SubscribeIndexEvents subscribes only to this Context's synchronous LocalBus.
History/asset progress and main change/flush messages carry store/page/generation;
they never mean source-save success. Publish runs outside the resource gate while
holding the publishing lease, so callbacks may read the owner without deadlock.
Unsubscribe/Close remove callback references and join real active calls; timeout
does not unpin a live callback. Do not Close a subscription or Drain its manager
from inside that same callback. Close subscriptions explicitly; resource eviction
closes remaining ones. The original index-changed/flushed business now shares
AppData.SetIndexPending; a Context-owned subscription derives state from this
owner's real pending queue without writing preferences. Publishing after each
committed operation reports partial progress even when the remaining tail fails.
SubscribeOriginalSQLProgress binds explicit policy, copied owner languages and a
synchronous message sink to this Context. Five legacy progress formatters share
conf.SQLProgressMessage; original mobile suppression and insert/delete switches
remain. Five families have actual Context producers (history/assets/delete/rename/tree
block batches), including group-wise current/total and original rename/batch hashes.
No boot-details or WS active-page broadcast is used. Tree batch notifications follow
SQL commit, not a synthetic success publisher. Subscriptions
are resource-owned and survive release of the creating request, but are removed
on explicit Close/context eviction. The compat bridge still registers five
groups/49 callbacks; seven SQL callback businesses are now shared (including the
two state methods), other 42 bodies and WS routing remain pending. Scoped setAttrs/foldHeading/unfoldHeading use
explicit dependencies. Unfold has an editor parser, reference query, AV-name
resolver and successful-commit-only synchronous owner reload (no waiter goroutine).
Failed query/render/finish never refreshes UI. It retains original rendering and
fold semantics but the staging seam is not a source-save receipt; update/move/AV
and other transactions remain denied pending audit and R04 source convergence.

S07 concurrency correction: domain reads use `WithReadContext` across the full
logical request, passing its returned context to nested Tree/SourceFile/Acquire.
Nested calls retain the exact admitted store while a writer/drain waits, instead
of failing midway as workspace_closed. New reads wait for the mutation's
publication barrier; genuine draining still rejects new admissions. The scope
expires on callback exit and cannot be reused to acquire a fresh context. A
deterministic held-reader/writer/new-reader test covers completion and expiry.
The original WithRead helper remains for non-nested compatibility/test callbacks;
new domain services must use the context-carrying variant.

The same context now carries the sourcetx shared-lock admission. Tree,
SourceFile, DocumentSource and lease reads reuse that request's validated source
lock rather than re-reading all historical journals for every nested access.
The manager's request count still pins the store, and admitted source borrowers
drain before the physical lock is released. New requests still validate identity
and recovery; no process-global “already checked” flag or lookup-budget increase
is used. Tests cover nested source/lease reads while publication waits, retained
context rejection, and the source-level concurrent-reader/blocked-writer boundary.

`WorkspaceManager.OpenStore(ctx, root)` explicitly registers a canonical source
root and a validated immutable metadata snapshot. Requests only use
`Acquire(ctx, storeId, pageId)`, never an arbitrary path or global current page.
Two real paths advertising the same storeId fail; a symlink alias to the same
real root is idempotent. Controlled Manager.Apply drains store leases and refreshes
metadata. External metadata changes require explicit DrainStore/OpenStore rather
than silently reusing old editor generations.

Each context owns its confined `os.Root` data handle, paths, generation, leases,
configuration reader, mutable document parser, bounded LRU AST cache, SQLite
block/reference projection, cancellable serial index queue and retained tasks.
Initialization is serialized under the manager mutex; there are no duplicate
resource sets for one identity. Notebook `.wiki/conf.json` is read within that
context on each request, retaining extension fields and never saving defaults.

The parser records original byte spans rather than reserializing a reduced AST.
Block reads query the real per-context SQL index then slice original source
bytes; unknown nodes/fields remain intact. References include modern text-mark
attributes and native NodeBlockRef/NodeBlockRefID nodes. This is not a full rich
text semantic validator. Document identity is box/doc scoped; block identity
additionally includes the document. Reusing IDs in another page/store cannot
change resolution. Returned AST/config/reference values are detached.

The cache is bounded to eight documents / 32 MiB per workspace. Every access
reads the source and checks SHA-256, not mtime or notification delivery. Parsing
and SQL publication are serialized; an index transaction commits before its new
cache entry is published. Failed parsing/indexing returns failure, not a stale
successful read. Cache eviction removes the matching SQL documents/blocks/refs.
Each SQLite DB uses a unique private in-memory URI, one keeper connection and
at most one work connection; the keeper protects schema against connection
discard after transaction cancellation. These workspace indexes stay in memory;
S04 provisions `.local` source locks, journal and a separate persistent projection.
`ResourceStats(ctx)` exposes actual cache/index/parse counters for diagnostics.

`Release` is idempotent. Asynchronous owners explicitly `Retain` or `StartTask`
before dispatch. `StartTask` retains a private owner until completion even if the
callback releases its request lease early; it supports cancellation, Wait, panic
isolation and a default budget of eight tasks per workspace. The default budget
is eight workspaces; opening a
ninth can evict the oldest idle context but never an active lease/task. Explicit
`EvictIdle(now)` defaults to five minutes; no timer/worker starts implicitly.
`Drain(ctx)` rejects new acquisitions/tasks and waits for existing owners, closes
workspace and store handles, and leaves no background task. A timeout preserves
live handles and the draining gate; releasing owners and retrying finishes close.
Already acquired requests can finish reading while draining. `DrainStore` gates
and releases only one source; another source remains usable. After successful
release, explicit OpenStore can register a new root/clone, with fresh generations.
Stats exposes store/workspace/lease/task/watcher counts and the draining gate.

`WatchStore(ctx, storeId)` explicitly enables one metadata watcher owned by the
StoreContext; external events invalidate new Acquire until explicitly reopened.
Manager.Apply stops/restarts the watcher around a controlled refresh to avoid
late self-generated invalidation. No watcher starts just because a package was
imported. Store close waits for its watcher goroutine to terminate, not merely
for an OS handle-close request. Body cache freshness never relies on this watcher.

Read bounds: 100,000 visited entries per box, 16 MiB per document, 100,000 AST
nodes, 512 AST levels and 1 MiB notebook config. Traversal IDs, symlink entries, nonregular documents,
ambiguous doc/block IDs, invalid JSON or mismatched document identity are rejected.
The public `read` CLI remains an AST read preview, not a full semantic checker.
S05 also exposes page/content/check/index/recover through shared wikiops. Direct
store-level source requests (without workspace leases) are counted as owners;
mutations and DrainStore/Drain wait for them too. The source pointer is pinned
atomically at admission, not reused across close/re-register races. Writes must
never be called inside WithRead callbacks (no lock upgrade). Old GUI integration
is still S06–S08, not implied by the shared service implementation.
S04 read guards hold shared source leases and reject unfinished journals; the
combined service guards configuration and AST as a single logical request.

## Audit and stage boundary

The new dependency path is `handlers → wikiops → workspace → sourcetx/wikistore/docast`
plus buildinfo/stdlib and the existing flock/fsnotify/sqlite3 dependencies. SQLite's package
init registers its driver only; DB workers/connections start at explicit Acquire
and stop at Close. There are no owned package `init` functions, fatal exits,
global data/config mutation, cron jobs or implicit source bootstrap on this path.
The explicitly invoked server.Run owns a listener and its joined Serve goroutine;
readhttp.Session owns bounded, workspace-tagged WebSocket subscriptions/readers
and closes/joins them. CLI imports neither the server lifecycle nor legacy;
only the CLI/kernel command entries call `os.Exit`. CLI calls services directly,
with no HTTP client. The dependency/AST regression test forbids imports of old
owned packages and unreviewed initialization/exits/worker launch sites.

`internal/legacy` still imports the old model/util/sql/cache/job/server packages.
Those packages still serve the transitional old GUI; they are **not** isolated
by adding a lease around existing calls. This core replaces dependencies rather
than importing their initialization workers. No dummy DB/queue fields stand in
for migrated resources. The audit maps the owners and deliberately unused paths:

| Legacy owner (retained only for old GUI) | New core owner / boundary |
| --- | --- |
| util/working.go, util/workspace_manager.go global paths/switch | StoreContext layout + WorkspaceContext paths/root; no assignment to DataDir |
| model/conf.go Conf/event subscriptions and config saves | Scoped NotebookConfig loader; GUI/cloud/export preferences not opened by read core |
| sql/database.go global DB/history/assetContent and driver hooks | Actual private SQLite blocks/refs DB per context; unused history/FTS/asset-content services are not started |
| treenode/blocktree.go, treenode/node.go globals/fatal errors | Per-context span parser + SQL block index; no implicit AST fixes or process exits |
| cache/ial.go, cache/asset.go, DynamicRefTexts | Bounded per-context raw cache and reference table, invalidated by byte hash |
| sql/queue.go, model/transaction.go init → flushQueue | Per-context SQL queue/commit/rollback plus S04 StoreContext source/session coordinator |
| task/queue.go, job/cron.go background dispatch | Owned StartTask / lease / Wait; cloud/sync/OCR/cron are not booted |
| watcher lifecycle | Explicit StoreContext WatchStore, waited Close; no body watcher required for hash-checked reads |

S03 is complete for the **new core resources and both read adapters**. Removing
the old GUI's callers is still S05–S08, not implied by that status. S04 now provides
session/source locks, journal admission, source-write queue and persistent page
SQLite projection; these are distinct from the in-memory read index here. S05
has wired shared CRUD and portable server routing; S06–S08 connect host/editor. Until those
integrations/migration are verified, use **synthetic portable fixtures only**.
Locks coordinate participating core users, not arbitrary external file writers.
See the [source transaction contract](../sourcetx/README.md) for recovery, receipt
retention, persistent projection publication, resource limits and test evidence.

## Evidence

```sh
go test -race ./internal/workspace ./internal/wikistore ./internal/handlers/... ./internal/wikiops
go vet ./internal/workspace ./internal/handlers/... ./internal/wikiops
```

Tests cover 80 concurrent A/B reads with identical doc/block IDs, distinct stores
reusing page IDs, identity/root alias ambiguity, failure isolation, traversal and
symlink rejection, returned-byte detachment, fixture hashes, task retention,
budget/idle eviction, generation changes and drain timeout/retry. Handler tests
compare CLI and HTTP AST envelopes on a temporary path with spaces/quotes/Unicode;
HTTP accepts no root path parameter and rejects absent/duplicate identities.
Additional tests exercise real SQL reads/refs/config/cache separation, LRU+SQL
eviction, same-mtime body changes, unknown AST preservation, failed indexing
rollback/cache publication, cancelled SQL transaction schema survival, malformed
parser reset, queued cancellation, bounded retained tasks including early Release
and panic, per-store release/reopen and watched metadata admission. Close tests
probe the actual DB and directory handles and wait for watcher Done; they do not
infer resource release only from a counter reaching zero. Native process tests
check help/version/read have no HOME/cwd/source writes and the old server's
isolated startup still works. CGO is now required for CLI reads as well as Server.

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

The original FSRS implementation is retained in [original flashcard core](../flashcard/riff/PROVENANCE.md), with its upstream AGPL license and provenance. Storage and clock are explicit ports; the original application, import/export adapters and API use this same core. Original deck/card/monthly-log MessagePack fields and builtin deck ID remain compatible, verified by bidirectional reads with the pinned upstream implementation. No new UI or public API schema was introduced.

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

### R02 configuration/resource consumer bindings (2026-10-27)

ConfigurationPresentation shares the full original masked/role/sync projection with model.AppConf and actual getConf. RuntimePaths/RuntimePath return canonical page-local data/runtime/conf/temp/history/asset/emoji selections; they never grant filesystem or persistence authority. Acquire replaces JSON workspace/data/conf display paths with registered paths after classified preferences load. Original getConf compatibility single-mode fields remain layered by the existing handler.

CustomEmojis and WithConfiguredParser read the pinned page emoji directory under one source admission/resource gate. Explicit caller emoji additions are copied and request-local; collection/parser maps are detached. Actual document, document-info/position, lifecycle/archive, bookmark, reference, index and AV consumers pass WorkspaceContext through parser construction. No native production ParserOptions(nil) remains. Source deletions refresh on read, invalid/symlink entries fail without read-time repairs, and close discards the owned registry.

NativeBlock uses indexdb.CachedBlocks/BlockCache, shared with original SQL GetBlock/GetBlocks, rather than a second map/query implementation. Cache access is copied and bounded, cancellation/closed owners reject, failed SQL batches do not partially populate, and committed main mutations invalidate only that owner's block/ref caches. Existing actual source/index/cache isolation tests and eight new configuration/path/emoji/SQL tests passed with race checking; full suites and original UI/API/13 integrations passed. Remaining settings/repair controllers, SQL/transaction callers and task/job/watcher lifetimes stay open in R02; no new public handler or source publication is implied.

### R02 setting plans and explicit effects (2026-10-27)

PlanNativeSetting consumes the acquired page's actual merged configuration under one source admission/gate. It calls the same AppData.PrepareSetting normalizer as ten original HTTP handlers and returns detached Data/Value/Effects plus device/store/workspace PreferencePatch Set/Remove deltas. Runtime authority is not a preference patch. Merge deltas against current documents and use owner CAS after live leases drain; the preview does not authorize source/runtime publication or execute compatibility callbacks.

The original adapter now uses explicit SettingPorts and private AppConf persistence. Required ports validate before save; accepted owner operations serialize with cancellable admission. A successful save precedes live configuration and ordered history/cache/parser/SQL/notification effects. Missing bindings never fall back to globals; compatibility effects remain confined to the original selected owner. Failed save/cancellation/read-only/unbound tests preserve memory and notifications. Committed effects retain request values and finish despite HTTP cancellation; post-save effect errors retain the committed config and carry an explicit error marker.

IAL PublishBlock completes publication after draining older buffered writes, under the owning cache lock. The original AV-name environment uses that barrier before notifications; candidate native effects still await formal commit. Regular warming is unchanged. Nine new tests and full race/core vet/API/13 integration checks passed. The unadapted setting, credential, import, repair, SQL/transaction and background caller graphs remain in R02.

### R02 virtual/emoji/export settings (2026-10-27)

Options.SettingServices binds trusted bounded device probes for each acquired identity; callbacks must not reenter the owner/manager. Acquire failure closes unpublished resources, and close clears the service references. PlanNativeSetting calls PrepareSettingWithServices with the actual merged page configuration. Nonempty export tool paths fail without explicit services; invalid selections use that owner's current tool and return warning intent, not an implicit global or process command. Runtime tool paths remain outside preference patches.

NoSave/EffectsOnly distinguish original transient behavior: recent emoji is memory-only for this operation, refresh is effect-only, and empty keyword input only notifies. Such plans return no immediate preference delta; native previews publish neither live memory nor effects. Ordinary compatibility saves may later include in-memory recent emoji. The original include/exclude helpers and docview keyword parser now share textprefs rules; upload filename facades use the same pure code. New tests verify actual dual-owner config/tool selection, failure cleanup, no source hash changes and missing-binding refusal. Full race/vet/API and 13 integrations passed; this does not complete appearance/publish/account/import/repair handlers or formal source publication.

### R02 explicit publishing resources (2026-10-27)

Options.PublishOptions returns explicit listen host, upstream, transport and owned auth/session callbacks for an identity. New creates no listener/worker. StartNativePublish applies only the acquired owner's existing configuration; StopNativePublish disables only its runtime without changing preferences. Missing bindings reject, constructor failures publish no Context, and resource close cancels/joins the service before closing indexes/source. Native publish setting changes still produce classified device-preference candidates rather than committing source.

publishsvc tracks serving and admitted request lifetimes, separates credential replacement from backend IO, reuses ephemeral ports, keeps a working listener on replacement bind failure, and supports Close retry after timeout. Auth ports must replace atomically on Configure and remain bounded/non-reentrant. When supplied, owned Auth.Close runs after requests drain. Compatibility publishing guest JWT/session state is synchronized and scoped to the selected config owner; identical reads keep keys, while credential/enable/owner changes revoke them. This does not migrate cloud login or expose a new CLI/server command.

Real dual-owner fixture reads, loopback authentication, occupied ports, in-flight cancellation, close retry, settings/getter sequencing and original JSON passed along with full race/vet/API/13 integration checks. Independent Context listeners do not establish a single-port routing policy or complete native UI/auth transport assembly. Appearance watchers and remaining original lifecycle graphs stay open in R02.

### R02 appearance resource ownership (2026-10-27)

Options.AppearanceOptions supplies trusted bundle/destination/theme/icon roots and explicit change/error callbacks per identity. Acquisition constructs the optional owner without IO or workers and cleans unpublished resources on failure. Missing bindings reject; supplying a competing appearance normalizer is ambiguous and rejected. Resource teardown closes appearance watchers before indexes/source.

NativeAppearance returns a detached merged catalog without copying files or starting a watcher. StartNativeAppearance installs only explicitly bound device resources, normalizes runtime configuration and owns the watcher; StopNativeAppearance leaves preferences/source untouched. PlanNativeSetting("appearance") produces device patch/effect candidates only. Neither method is the R04/R05 preference/source commit adapter.

Original theme/icon enumeration and selected metadata are shared with the compatibility setter/boot. Owner.Apply is explicit, bounded and cancellable; Stop removes watches, closes and joins, Close supports retry after timeout. Callbacks must honor cancellation, remain bounded and not re-enter the owner. Per-owner file-event routing, missing factories, unchanged source bytes and repeated descriptor counts are tested; the original UI/API is preserved. Cloud/import/repair and other unfinished caller graphs keep R02 doing; full native UI/server routing stays R03/R06.

### R02 cloud account Context capabilities (2026-10-27)

CloudAccountOptions binds private device Load/Commit, region-aware Request, Decode/Encode and Now callbacks per store/page identity. New starts no IO or workers; missing/failed factories reject acquisition and clean unpublished resources. Account credentials belong to this private device authority, not shared wiki data, page preference patches or source receipts. The native host must supply storage/codec/network capabilities explicitly; no fallback to the selected legacy Conf is permitted.

NativeCloudLogin preserves region-first/token-only login; NativeCloudLogin2FA preserves the original wire protocol; NativeCloudRefresh/Logout serialize through the account owner, publish only after accepted persistence and synchronize the Context runtime projection. The legacy adapter preserves read-only refresh's memory-only behavior and detached user/title values. Service Close cancels requests and waits with retryable deadlines. Source/credential transaction boundaries remain distinct.

StartNativeCloudRefresh uses the existing lease-owned StartTask, checks binding before creating work and reads the account after its delay under the service gate. Logout cannot be undone by a captured old refresh token; drain cancels and joins the actual task. Real Context tests verify separate credential/region/user state, missing authorities, rejected construction, release/drain and unchanged source bytes. Full native auth routes/CLI commands/UI assembly remain R03/R06; this is not completion of cloud sync, trial/deactivation, announcement/subscription workers or R02 as a whole.

### R02 imported configuration and Notebook repair candidates (2026-10-27)

PlanNativeConfigurationImport uses the acquired owner's merged configuration and the original twelve-group import core. Only provided groups replace values; omitted groups retain their owner data. Required null/invalid input rejects. Runtime paths/device identity/read-only/disabled-feature/Pandoc authority and non-imported credentials remain owned. The result contains detached full configuration plus classified Set/Remove deltas. Planning writes no preference, source, live configuration or receipt; the ordinary R04/R05 publication boundary remains.

PlanNativeNotebookRepairs scans this Context's pinned data root under source admission and the resource gate. It supports existing canonical .wiki or legacy .siyuan metadata without renaming either, rejects competing metadata roots/symlinks and preserves valid raw metadata/extension bytes. Missing configurations produce original closed Untitled defaults plus an unindex intent; corrupt configurations retain exact Before bytes and SHA-256, with a distinct missing marker. Native reads never apply these changes. The future authoritative source transaction must revalidate preimages, retain history/backups, commit before updating indexes and emit its own receipt. The compatibility writer is explicitly separate and cannot substitute for that transaction.

Tests exercise actual same-ID owners with different merged settings and valid/broken/missing source metadata, cancellation, release/drain, detached preference deltas and unchanged source hashes. Original JSON/ZIP import and compatibility two-pass Notebook repairs are tested through the original application. Full native UI/handler routing and authoritative writes remain later stages; R02 remains doing.

### R02 B1 root shared-preference snapshot and restore (2026-10-27)

SnapshotStorePreferences(ctx, storeID, at) captures the versioned root-only component using the registered StoreContext and explicit UTC clock value. PlanRestoreStorePreferences captures the current target file CAS/tree digest under read admission; the snapshot's old graph digest is provenance and cannot replace the file CAS. Both calls are read-only, support nesting under WithReadContext and reject retired/foreign admissions. The component includes exact optional source bytes, never private device preference values.

RestoreStorePreferences verifies the plan before lifecycle mutation, requires leases and read admissions to be released, then uses the existing store-wide drain and authoritative source Writer. Self-upgrading the current read admission fails with store_busy rather than waiting for itself. All page Contexts are rebuilt on reacquisition, including shared-only changes and absent-file inheritance. Page preferences/device configuration/document bodies and unrelated stores remain intact. No-op absence restoration reports no source commit; interrupted restore blocks capture/reads until Recover. Reusing a rolled-back operation returns its original uncommitted receipt; retry needs a new operation ID.

Actual dual-page Context tests verify restored values, generations, retained page/device overlays, cancellation, stale CAS, no-op, retired admission refusal and journal recovery. Snapshot data is detached, content-verified and scoped root-store-preferences. S09 must still compose the full whitelist archive/manifest/sealing and new-directory reconstruction; S11 must still supply protection snapshots and selective/in-place restore orchestration. These internal methods add no public native/legacy handler or UI flow.

### R02 owned cloud business tasks and path-based upload (2026-10-27)

NativeCloudStartFreeTrial, NativeCloudDeactivate, NativeCloudUploadToken and NativeCloudUploadAssets bind the acquired Context's optional CloudAccountOptions. The existing account Request/clock/device authority and optional Upload transport remain trusted capabilities, never JSON-created permissions. No public native route/CLI command or role assembly is added. Trial/deactivation keep the original local state unchanged; token cache/credentials remain private memory and produce no source/preference receipt.

Network work is registered through StartTask and retains its creation lease. Drain cancels and joins actual callbacks; a canceled caller wait does not falsely release a still-running task. Before upload, explicit asset paths are validated and frozen from this page's pinned data root; paths/link escapes and budgets reject before cloud access. The native budget is 128 paths/64 MiB of frozen content, with metadata only for files over the original 10 MiB maximum. Resolve/Open then consume detached bytes, and this operation's source admission/resource gate is released before networking. An outer caller-held read admission remains the caller's responsibility.

The shared account gate covers successful upload-token acquisition and the complete batch. Per-owner/account/region/entitlement cache isolation, TTL and 401/logout invalidation prevent credential mixing. Both upload-assets and export-liandi preserve 3/10 MiB limits, resolved deduplication and original multipart business metadata. Partial remote completion is returned honestly, not published as a source transaction. Tests verify different page bytes/regions, no source writes, an available exclusive source lock during transport, task cancellation/drain and missing/unsafe bindings. Automatic document/AV/embed asset extraction, full forum article/source-write Context migration and complete handler/UI assembly remain separate work.

### R02 activation handlers on owned cloud tasks (2026-10-27)

NativeCloudCheckActivationcode and NativeCloudUseActivationcode use runNativeCloudBusiness and the existing cloudaccount capability. Missing capability refuses; no Conf or implicit device network fallback is used. Methods pin creation identity until the actual request exits, honor caller cancellation and drain cancellation/join, and release their own source/resource admission before networking. Callers must also avoid wrapping network IO in a long-lived outer source read admission. Check returns only its original 0/1 and confirmation message; redemption invalidates the private per-owner upload cache even when the remote outcome is uncertain, with no automatic retry, user persistence or source receipt. Original GUI handlers remain compatibility adapters with unchanged middleware/response shapes; public native role/handler assembly remains a later stage.

The original HTTP client retries failures/503 once by default. The compatibility transport now overrides that policy only for the redemption route (SetRetryCount(0)); an actual loopback 503 verifies one send. Injected native Request ports must likewise honor the single-attempt redemption contract. Check and other account routes retain their existing retry policies.

### R02 owned Inbox reads, projection and deletion (2026-10-27)

NativeCloudGetShorthands/GetShorthand/RemoveShorthands reuse runNativeCloudBusiness and the explicitly bound InboxRequest capability. Reads capture a fresh configured parser and emoji state from the pinned page before releasing their own source/resource admission for networking; returned list/detail HTML preserves the original projection and nested pagination. Same remote item/account IDs do not make different owner credentials, regions or parsers interchangeable. Missing capability refuses with no Conf fallback. All three operations support caller cancellation and drain cancellation/join, with no source receipt or local account persistence; callers must also avoid retaining an outer source read admission around network IO. Removal ports must not retry automatically; a canceled removal can already have succeeded remotely. Native route/role assembly and cross-endpoint cloud-to-document orchestration remain later work.

Removal snapshots preserve nil versus empty input slices as well as order and duplicates. Synthetic source digests and task counts cover all three methods; original GUI regression excludes only its independently updated dataIndexState runtime flag, while isolated model tests compare full configuration bytes.

### R02 reminder candidates, cloud calls and owned expiry timers (2026-10-28)

PlanNativeBlockReminder prepares the original date/content/IAL/notification intent from this page's detached tree, contextual static-content services and language; the result includes source hashes and changes no source/cache authority. It is a read-only candidate with no remote operation or receipt. NativeCloudSetBlockReminder uses the existing pinned cloud task and explicit ReminderRequest capability, releasing its own source/resource admission before networking; it is the low-level authenticated cloud port, not native publication of the candidate. Trusted handler permission/subscription assembly and authoritative source commit remain R03/R04; callers must not hold an outer source read admission around network IO.

StartNativeSubscriptionReminder registers an actual Lease task with original 7/30-second timers, paid/trial cutoffs and owner-private once-only state. Account identity/entitlement revalidation occurs at delivery; cancellation/drain joins both timers and cloud callbacks. Notification is a bounded, context-honoring explicit device callback, never global Conf or a detached task queue. Same block IDs do not share content/credentials/region across owners. No receipt, persisted expiry flag or account data is written to source. Original compatibility UI and runtime queue execution remain the GUI path until later source publication work.

### R02 owned anonymous announcements and device-wide seen ledger (2026-10-28)

StartNativeAnnouncements uses the existing Lease task owner and an explicitly injected cloudaccount.Options.Announcements capability; missing capability refuses. Its lookup ends its own source/resource admission before the one-minute/default-host delay, anonymous GET, ledger locking and notification. The task retains creation identity until the real callback exits; drain cancels and joins delay/fetch/notification separately. Default region access goes through the account snapshot gate rather than unsynchronized Load, so concurrent login/region commits cannot race the announcement reader. Notifications and device roots remain trusted host ports with no global Conf fallback.

A FileStore may be shared explicitly across contexts, or separate instances may address the same trusted device home; the advisory file lock and fresh merge preserve IDs without duplicate display. Legacy semantics deliberately record all regions before showing only the current one, so an ID recorded by one context is also seen by another region on that device. Owners retain separate region-keyed network caches. Markers live only in the legacy private device ledger, not a page directory, source receipt or backup preference. Cancellation after marking may suppress display; no exactly-once/rollback claim is made. Link/corrupt/failed markers refuse safely, and network/notification callbacks must honor ctx and avoid reentry into the same announcement owner. Public handler/role assembly remains later work.

### R02 AI tasks, private history and actual source extraction (2026-10-28)

NativeAIChat/NativeAIAction use an explicitly configured aiflow Service and the existing Lease task owner. AIOptions.Open and Event are required trusted capabilities; absent Load defaults to a short resource-gated snapshot of this owner's already-classified AI settings and Markdown configuration. Construction starts no network/client worker, reads no environment and imports no global Conf. A failed provider binding is cleaned up before a workspace is published. API secrets remain device authority, never supplied by a request/wiki source JSON.

Actions read this page's detached original trees/registry under their own source admission and export the original document children/repeated blocks/Markdown separators. The immutable prompt is ready before provider IO, which holds neither that source lock nor the resource gate; caller-held outer admissions must also end. A bounded per-owner conversation is serialized, clears on credential/provider identity changes, and does not receive failed/canceled turns or action results. Clear-context preserves the original plain/action distinction. Clients/cleanup callbacks finish before task ownership ends, and drain cancels/joins direct and low-level cloud requests. NativeCloudChatGPT separately retains original cookie/region/user-role payload and response semantics; no automatic fallback or combined source write is introduced. No source receipt or new public CLI/native HTTP command is published.

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

### R02：Markdown导出准备端口（2026-10-30）


本批完成批量Markdown/Pandoc族与整本Markdown的入口Context、Export值快照和选中文档/引用树准备：11类文档转换HTTP调用及笔记本入口传Request.Context，model保留可选Context兼容签名；空选集直接返回原空字段。选集按每个ID所属笔记本枚举子文档，修正跨笔记本批量选择误用首本枚举器的问题。树与块/链接/AV引用准备复用归档只读源端口和原递归算法，首读字节/局部树缓存与取消/路径身份检查保持；原SQL子块引用计数改由捕获主库查询并传播错误。顶层标题/YFM/引用/嵌入/标注/tag/inlineMemo选项来自捕获配置，准备失败在进入旧输出阶段前返回。

边界：仅完成入口、配置与准备阶段；深层exportTree、动态引用/嵌入/AV/标注、NewLute及其他配置/源读取仍含全局依赖或旧修复行为，不能据此认定整个导出只读或隔离。资源复制、global Pandoc调用、按标题复用输出目录/ZIP及其关闭/清理错误传播仍待；单文档正文ExportMarkdownContent、HTML/PDF等其他准备控制器尚未迁移。源.docj格式不变，Markdown仅为既有导出格式。HTTP/CLI参数/返回形状、帮助/导航和资源打包布局不变，无需机械修改；R02仍doing，R03–R10 todo，未启用Native正式源写、无R04持久receipt/回滚保证。既有17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：Markdown输出与资源归属（2026-10-30）


本批接通批量Markdown/Pandoc族和整本Markdown的资源/转换/输出阶段。每次使用捕获temp下的独占工作目录，复用归档输出身份核对、关闭并同步ZIP后独占发布、归属清理和失败抑制URL；下载文件名改为标题+唯一ID+格式扩展+.zip。Markdown ZIP仍无外层包装目录，保留文档层级、不同内容重名追加ID、Deflate及空目录行为。旧标题目录和历史ZIP保留，不再参与新导出或被覆盖。任何单文档复制/转换/打包失败结束本次导出，避免返回部分成功ZIP；HTTP仍沿用原空zip失败回退和原name行为。

资源索引改为捕获Notebook顺序及Data root读取，保留后Notebook覆盖、全局assets优先和缺失资源略过；目录资源递归复制包含隐藏项/空目录，目标为各文档相对资源目录。复制沿用有取消检查的流复制与本次scratch替换；链接/越界/权限等真实错误返回，不再吞掉。空父文档的子目录判定改为捕获root读取；资源收集使用显式AV读取端口。源root关闭错误也抑制下载URL并触发输出回收。

Pandoc执行不再调用旧util.Pandoc：入口捕获已选择的设备工具路径及容器能力，转换时使用显式命令和请求Context，输入文件限于本次私有工作目录；保留原from/to/resource-path/-s/-o参数，取消可终止子进程并限制管道等待。不需要转换时直接写本次payload文件；成功必须实际产生普通文件并完成同步/关闭，输出与清理错误返回。工具stdout/stderr不归档正文。设备工具初始化/安装/运行时选择仍是原全局兼容桥，尚未宣称完整Workspace设备能力装配；本批不安装Pandoc、不新增外部依赖。

边界：深层exportTree、动态引用/嵌入/AV/标注、NewLute及其余配置/源读取仍含全局依赖或旧修复行为；整个导出尚不能宣称只读、全源一致性快照或完整隔离。尚待单文档正文ExportMarkdownContent、HTML/PDF等其他控制器和R02其他副作用/后台链路。本批正常流/错误流的临时文件归属不等于R04持久receipt、进程骤停恢复或恶意跨进程改名CAS；极端身份/权限/句柄错误仍可能有已报告残留。源.docj格式不变，Native正式源写未启用；R02仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：Markdown引用渲染端口（2026-10-30）


本批接通批量Markdown/Pandoc族及整本Markdown的块引用锚文本、mode4路径/锚点定位和blockLink2Ref递归。复用原导出函数与遍历/转换算法，通过可选的单次渲染适配器提供捕获registry、主SQL、只读树准备及错误返回；没有另写一套渲染UI。无适配器的HTML/PDF/单正文旧调用继续原路径。每个渲染适配器保留首个错误，SQL/registry/源读取/缓存身份失败不再当成缺失引用或回退进程全局资源；拥有Context的Markdown入口将失败传至外层，不发布ZIP。

入口在同一设置快照中捕获Editor的锚文本最大长度和ParserOptions，复制emoji映射；exportTree入口和Markdown正文渲染器使用捕获的Editor parser，动态锚文本使用捕获配置的Index parser及原indexdb.ReferenceText算法。保留静态锚文本、缺失SQL定义的原“block not found”文本、search标记清理、HTML反转义、截断与左右装饰，保留mode2协议链接/mode3文本/mode4相对文件链接和块哈希规则。递归保留原4096深度边界、标题子块遍历及本次树缓存；源只读端口保留首读字节/旧Spec兼容，验证rootID/Box/Path一致性。

边界：原渲染器会修改本次候选树，本批不宣称候选不可变或整个导出全源只读/一致性快照。resolveEmbedR及其内部查询/渲染、文件标注、AV表格/过滤/汇总、标题getBlock、widget/CSS和其他parser构造/配置仍有全局依赖；singleFile脚注定义收集/解析及HTML/PDF/单正文调用尚未完整接入。本批接入的批量路径使用singleFile=false，不据此宣称单文件脚注完成。设备工具初始化兼容桥及R02其他导入/镜像writer/缓存/广播/历史/后台仍待。源.docj格式不变、Native正式源写未启用，无R04持久receipt/回滚或恶意跨进程改名CAS保证。R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：Markdown嵌入读取端口（2026-10-30）


本批完成批量Markdown/Pandoc族及整本Markdown的resolveEmbedR嵌入渲染Context接入。通过既有单次渲染适配器传递捕获的主SQL、registry/源树读取、Search.Limit、Editor.HeadingEmbedMode和parser；继续使用原解析查询/选集顺序、原文/引述、标题0/1/2与custom-heading-mode覆盖、折叠子标题过滤、层级调整/六级上限、属性转移及resolved/七层深度边界。未提供适配器的旧HTML/PDF/单正文调用继续原兼容路径。

SelectReadBlockRows复用原SelectBlockRows算法，并在输入及实际执行的重写SQL上验证单条只读SELECT；保留原分页、显式limit及CTE等原raw分支，不在此重写其历史分页语义。SQL/registry/源读取、命中但缺节点、缺脚本及格式化错误保留为首个渲染错误，返回拥有Context的入口并抑制ZIP，停止把真实故障降为空嵌入。只读校验拒绝写语句/多语句；本批不新增数据库写权限。

嵌入会调整标题或格式化节点，因此每次使用都从已准入的首读.docj字节重新ParseJSONWithoutFix，复制定位信息，旧Spec仅在候选内转换。候选独立，避免重复嵌入累积改变层级或污染后续渲染；源字节、未知字段和外部后续版本不会被回写。此机制是请求内首读文档快照，不是SQL与整个源仓同时一致性快照；外层源root关闭返回值审计仍按原待办保留。

边界：renderBlockMarkdownR及单文件脚注定义中的另一条嵌入查询/渲染调用链尚未接入；本批批量路径singleFile=false。AV表格/过滤/排序/汇总、文件标注、标题getBlock、widget/CSS/其他parser、设备工具初始化兼容桥及其他导出控制器和R02其他副作用/历史/后台仍待；不能宣称整个导出全源只读或完整隔离。源.docj格式不变，Native正式源写未启用，无R04持久receipt/回滚/进程骤停恢复或恶意跨进程改名CAS保证。R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：Markdown标注与资产归属（2026-10-30）


本批完成批量Markdown/Pandoc族及整本Markdown的文件标注读取和链接转换Context接入。沿用原两种链接AST/页码展示：mode0为“文件名 - p页码 - 锚文本”，其余模式仅锚文本，跳转仍为资产路径#page=N。正常原生“文件名-22位节点ID.扩展名”保持原展示名，短/非标准文件名安全保留，不再做可能越界的切片。未传适配器的旧HTML/PDF/单正文调用仍走原兼容读取分支，只有链接AST构造提取为共享纯函数。

标注资产查找使用捕获的Data root和Notebook顺序：全局文件优先，否则按各Notebook词法遍历的首个后缀匹配，保持原GetAssetAbsPath的选择顺序。源和.sya使用root相对检查，拒绝链接/越界/非普通文件，固定路径的进程内锁保护侧车读取，关闭错误合并返回。请求内缓存整份首次成功读取的.sya记录，不同annotation ID不会混读后续文件版本；返回缓存前仍检查取消/路径归属。只校验实际引用的记录，其他扩展字段保留在源中，不要求全部记录同构。

.sya读取/JSON/缺记录/空pages/缺index/类型错误/负数/非整数/溢出等均明确失败，不再靠强制断言或把故障当成可忽略引用；页码仍由0基index加1。首错返回拥有Context的Markdown入口，失败抑制ZIP并沿用私有输出清理。Markdown资源复制去掉导航fragment后定位文件，修复#page=造成PDF漏包；标注生成的链接按实际选中资产复制，避免标注按首Notebook读取而普通资源索引按末Notebook覆盖造成不一致。未被标注选中的普通资源保留原索引规则；不额外复制.sya到Markdown ZIP。

边界：标注缓存只是请求内首读侧车快照，不等于PDF、文档、SQL与全仓原子一致性快照。AV表格/过滤/排序/汇总、标题getBlock、widget/CSS/其他parser、renderBlockMarkdownR/单文件脚注、设备工具初始化和其他导出控制器/R02其他副作用/历史/后台仍待；不能宣称整个导出只读或完整隔离。旧HTML/PDF/单正文标注的全局读取及异常结构处理仍需迁移。源.docj/.sya格式及未知字段不改，Native正式源写未启用，无R04持久receipt/回滚/进程骤停恢复或恶意跨进程改名CAS保证。R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：Markdown文档根投影（2026-10-30）


本批完成批量Markdown/Pandoc族及整本Markdown的文档标题投影，以及挂件Markdown/打包资源收集解析器的Context接入。标题不再走getBlock→全局SQL投影parser/fromSQLBlock→全局Search配置的兼容入口，改为捕获候选身份检查、ProjectDocumentBlock原根节点分支及原blockview.FromRow规则。Search.CaseSensitive与parser使用入口快照；保留原HTML实体处理、零宽清理、5120字符UI截断及IAL转为标题属性的逻辑。标题开关和插入一级标题的原算法不变。

ProjectDocumentBlock只对独立根节点/IAL副本调用原buildBlockFromNode的Document分支，不遍历正文、查询SQL/registry、读取资产/AV或启动OCR。复用原投影而非重新拼写标题语义，避免NodeHash排序改变调用者IAL；返回的Block/IAL独立。上下文、树/根身份、节点类型、IAL结构和投影失败返回错误，首错抑制ZIP，并在标题阶段前后停止后续渲染。此处仅消除标题读取自身的隐式依赖，外层导出仍会使用捕获SQL/registry。

挂件data-export-md仍先HTML反转义、启用YFM并替换为原解析节点；使用与原util.NewLute同类型的nativeparser.NewLute，配置改为捕获Markdown值，不替换成Editor parser。打包前重新解析正文收集资源也用同一捕获值。未提供适配器的HTML/PDF/单正文等旧调用仍保留getBlock及util parser兼容路径，不据此宣称其已上下文化。

边界：AV表格/过滤/排序/汇总、CSS主题资源、聚焦导出的标题级别辅助读取、renderBlockMarkdownR/单文件脚注、设备工具初始化、其他导出控制器及R02其他IO副作用/历史/后台仍待。整个导出尚不能宣称全源只读或完整隔离；本批根投影无正文/SQL依赖不等于整个导出无这些依赖。源.docj格式和未知字段不变，Native正式源写未启用，无R04持久receipt/回滚/进程骤停恢复或恶意跨进程改名CAS保证。R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：脚注中层读取端口（2026-10-30）


本批完成collectFootnotesDefs/collectFootnotesDefs0、resolveFootnotesDefs与renderBlockMarkdownR的可选Context中层适配，复用已有捕获registry/主SQL、只读查询、解析配置与独立首读候选端口。保持原脚注编号/去重/本页锚点排除、静态锚文本及最大长度、标题/文档/列表节点展开、同文档段落64字符缩略、HTML/换行解码、rendered集合循环去重及收集器4096深度边界。没有另写脚注渲染算法。

脚注定义保留原loadTreeWithCache的“请求ID”缓存规则：请求块ID未命中时，不复用已经被blockLink2Ref转换的根ID候选，而从已准入的源字节创建独立树，并缓存到该块ID下。这样保持原链接转换时机；根或块缓存均校验定位身份。递归Markdown每次源加载同样借用独立首读候选，避免移动节点污染下一次读取。源、registry、查询、缺节点/脚本、取消和路径切换错误保留首错；拥有适配器的失败不返回部分脚注块或部分节点列表。

重要状态：批量ZIP仍使用singleFile=false；单正文/标准Markdown HTTP控制器仍使用原无适配器兼容路径。本批验收的是中层显式端口和拥有Context的singleFile内部链路，并未宣称这些HTTP入口已经完成迁移。控制器的聚焦准备、CSS、订阅/云资源等依赖仍须后续统一装配；不能为了接通脚注而在未满足依赖时回退全局读取或丢弃原请求选项。原无适配器路径保留旧读取/错误兼容行为。

边界：AV表格/过滤/排序/汇总、CSS主题资源、聚焦辅助、设备工具初始化、其他导出控制器和R02其余IO副作用/历史/后台仍待。候选树仍会被原渲染算法修改，首读文档候选不是SQL与全仓同时一致性快照；不宣称整个导出只读/完整隔离。源.docj及未知字段不改，Native正式源写未启用，无R04持久receipt/回滚/骤停恢复或恶意跨进程改名CAS保证。R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：聚焦候选归属（2026-10-30）


本批完成聚焦准备与标题级别调整的Context中层端口。selectExportTree提取原prepareExportTree的选择算法，保留整篇、标题及其后代、单块的范围和末尾原占位节点；旧入口仍使用原全局loader/parser与原临时根行为。新prepareFocusedExport使用捕获registry/独立首读候选/parser，验证命中节点后再执行共享选择逻辑，错误返回而不回退全局。

拥有Context的候选将Tree.ID和临时Root.ID统一为源根ID，并补齐Root的Box/Path，避免新读者把临时根当作源根缺失；源文件ID不变。只登记由本次准备端口生成的候选，记录根指针和定位信息；任意同ID克隆、根替换或ID/Box/Path/HPath/类型变化不能凭字符串相同进入标题投影。登记的候选可以使用已有的标题端口，输出属性仍独立；调用者拿到的定位结构也是副本。

引用/脚注读取现在在拥有Context时保留请求块ID，以识别聚焦范围外的同文档定义。范围内读取复用聚焦候选；范围外读取独立完整源候选，不覆盖当前聚焦范围。标题级别调整共享原算法，读取捕获的定位及本次渲染实际使用的addTitle参数：无标题时首标题提升一级、有文档标题时提升二级、整篇不调整、保持原直接标题/层级传播与1–6边界。拥有Context的空树安全不处理；旧入口继续原配置路径。

状态边界：单正文/标准Markdown HTTP入口仍未装配这些端口，CSS/订阅云资源和完整控制器准备仍待；本批验收的是拥有Context的准备/内部聚焦渲染及原HTTP范围/级别兼容。AV表格/过滤/排序/汇总、设备工具初始化、其他导出/旧标注兼容分支和R02其余副作用/历史/后台仍待；不宣称整个导出只读或完全隔离。候选会被原渲染算法修改，请求首读源候选不是SQL与全仓原子快照。源.docj和未知字段不变，Native正式源写未启用，无R04持久receipt/回滚/骤停恢复或恶意跨进程改名CAS保证。R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：导出主题读取归属（2026-10-30）


本批完成Markdown渲染的CSS变量Context端口。捕获导出所属AppConf的外观模式/主题选择，以及入口当时的设备主题资源目录；只复制标量，不保留可变外观配置指针。主题资源仍是设备资源，不纳入wiki源数据。拥有Context的内部渲染通过显式theme端口读取，不在替换时访问当前Conf或ThemesPath；批量ZIP原有fillCSSVar=false行为不变。

提取并复用原主题解析和IAL替换算法，保留daylight/midnight基础主题与自定义主题拼接顺序、原粗略模式选择器优先级、同优先级后者覆盖、CSS解析器双连字符兼容、嵌套变量解析/循环与深度限制及未命中var()回退。缺默认CSS仍允许选中主题继续，缺选中CSS仍得到空映射；其他实际IO错误在拥有Context的路径上传播，渲染失败不返回部分正文。

拥有Context的读取验证绝对资源根与单级主题名，借用os.Root和既有Source只读端口，拒绝穿越与根/目录/文件符号链接，返回关闭错误；首用后缓存本次解析值，不重复追随主题文件变化。验证候选归属和Context存活，缺配置/缺端口/取消/关闭不回退全局。旧无端口入口仍保留原日志、路径/符号链接和缺文件兼容行为；严格读取策略只适用于新端口，尚未成为HTTP主题导出的新默认。单根文件检查不构成恶意跨进程改名CAS，跨主题文件读取也不是原子快照。

边界：单正文/标准Markdown HTTP入口仍未装配已有聚焦、脚注、标题和CSS端口；原UI保留，当前验收是内部Context端口及原入口兼容回归。AV表格/过滤/排序/汇总、订阅云资源和完整控制器依赖、设备工具初始化、其他导出/旧标注与其余R02副作用/历史/后台仍待。源.docj不改格式、不写回，Native正式源写未启用，无R04持久receipt/回滚/骤停恢复。R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：Markdown AV只读渲染源（2026-10-30）


本批将exportTree内的AV表格转换接入显式attributeTable端口。无端口时仍执行原文件存在检查、ParseAttributeView/getAttrViewTable/GetFurtherCollections/Filter/Sort和原日志/跳过规则；拥有Context时使用本次导出的只读AV scope，原表格AST生成和所有字段显示代码保持共用。表格/卡片/看板均遵循原表格投影：表格保留列隐藏/换行属性，卡片与看板按原ID-only字段投影；保留视图过滤、排序及不受UI分页限制的导出行集合。

新scope复用已完成的commitAVContext与avrender，而非另写渲染器。两个可选只读源端口复用导出已捕获的AV原字节和独立完整文档候选；默认事务/查询scope不设置这些端口，原行为不变。AV相关读取/模板SQL/块属性使用所属Context的reader、parser、SQL、语言与时钟，旧格式仅在内存升级；首读原字节与依赖缓存不因后续源变化被重定向。每次表格转换重建可变候选图，防止上一次自动填值/布局投影/排序污染下一次输出，也不修改归档准备保存的原候选。临时表格缓存不冒充卡片/看板布局缓存。

只读scope禁止save，源端口禁止借给可写scope；缺失AV仍作为悬空块跳过。已有文件坏JSON、身份错配、非法布局/字段、符号链接、依赖失败、取消或所属Context关闭均向上返回，不回退全局，不发布部分Markdown/ZIP，不覆盖既有产物并清理临时工作目录。原单正文/标准Markdown HTTP入口仍未装配Context端口；本批实际装配的是原有带Context的批量Markdown路径及内部渲染。CSV/HTML等其他导出控制器仍有单独迁移工作，不能以本批推断全导出已隔离。

边界：本次读取仍非SQL/文件/全仓原子快照，不新增跨进程CAS或R04持久receipt/回滚/骤停恢复；既有借用root关闭返回值等独立待办保留。源.docj/AV JSON格式不变、正式源写未启用、原UI保留。R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。后续优先补订阅云资源与完整控制器依赖，迁移单正文/标准Markdown HTTP入口，再处理其他导出、设备工具、历史/副作用/后台收尾；复杂跨库递归/循环和其他控制器需各自验收。

### R02：Markdown HTTP请求归属（2026-10-30）


本批实际装配单正文exportMdContent与标准Markdown copyStdMarkdown的Context调用链，覆盖上一批“HTTP入口仍待”的状态。新增错误返回的ExportMarkdownContentContext/ExportStdMarkdownContext与类型化选项，HTTP传递Request.Context；省略refMode/embedMode时由所属AppConf导出快照决定，显式0仍保留，处理器不再为这些默认值读取model.Conf。原公共模型函数保留签名并封装新入口，失败返回原空字符串兼容值；成功响应仍分别为hPath/content对象与字符串，不新增UI或HTTP字段。

入口用已有独立聚焦准备、稳定源根身份、引用/脚注/嵌入/标题/CSS/AV端口执行原Markdown算法。保留整篇/单块/标题后代范围、标题级别、YFM（单正文默认true、标准入口无YFM）、资源空格替换/imgTag参数与原字段格式。标准入口refMode4仍按原遍历顺序收集存在的定义ID并去重，随后复用原脚注/锚点逻辑。单正文不会无意启用订阅云资源前缀。

订阅状态使用既有AppData.IsSubscriber规则（终身或正期限且状态0），只捕获当前所属配置的区域与用户ID形成云资源基址，不保留User指针或token。GetCloudAssetsServerForRegion提取原区域选择算法，原全局helper仍兼容；标准入口把捕获的基址交给原renderer，后续全局区域/账户变化不重定向本次导出。此过程只读本地配置，不发云端请求。

Context/配置/依赖错误向上传递且不返回部分hPath/正文；HTTP异常返回code=-1和空data，合法但缺失块保持原成功空结果兼容。请求取消会在配置捕获/源读取/渲染守卫处停止，不再从处理器降级为Background；未改动的旧直接调用wrapper仍显式使用Background。源.docj/AV JSON不改格式、不写回，Native正式源写未启用；本批仍是现有兼容runtime的Context装配，不代表Native多workspace路由或R04原子提交/receipt/恢复已经启用。请求首读和配置捕获不等于SQL/文件/全仓原子快照。

R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。单正文与标准Markdown的这部分HTTP装配不再整体列todo；后续转向CSV/HTML/PDF等其他导出控制器、设备工具与旧标注兼容分支，继续其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限及后台完整退出。真实云账户/外部网络和复杂跨库循环需单独验收，不能由合成订阅测试推断已验证。

### R02：CSV只读来源与发布子目录（2026-10-30）


本批完成AV CSV导出控制器的Context接入。HTTP exportAttributeView传递Request.Context到新增ExportAv2CSVContext；原ExportAv2CSV签名保留，封装Background兼容调用。复用已验证的导出Context和只读AV表格能力，节点从所属registry/文档首读候选解析，校验选中块确为请求的AV，按该块保存的viewID选择视图；不再在控制器中调用全局getNodeByBlockID、ParseAttributeView或SQL渲染/过滤/排序。

复用原表格/卡片/看板投影和完整过滤排序，CSV仍导出所有匹配行及全部列（含隐藏列），不跟随UI分页。writeAttributeViewCSV提取原字段显示代码：UTF-8 BOM、标准CSV转义、日期/创建/更新时间、图片/文件链接、其他资源、逐行行号与Value.String回退保持；日期等修改仅作用于可变候选。数据库名使用本次捕获的未命名语言值，源.docj/AV JSON不改格式、不写回。

输出复用archiveExportOutput的独立工作目录、关闭/同步后打包、完整ZIP才发布及清理。增加可选的受约束发布子目录，默认归档/Markdown行为不变；CSV保留/export/csv/下载路径、.db.zip后缀及单个平铺“数据库名.csv”成员，外部ZIP文件名增加独立ID避免同名覆盖，不再清理旧同名文件夹。子目录拒绝符号链接并校验目录身份，已发布输出不能重定向子目录；碰撞、目录替换或外来文件替换不覆盖/误删无归属文件。单文件发布不是全仓原子快照，也没有恶意跨进程改名CAS或R04持久恢复保证。

补齐CSV Flush/Error、短写、文件Sync/Close、打包/最终关闭错误返回，以及逐行取消检查；失败不返回下载URL，既有HTTP code=1、closeTimeout=7000错误结构保持。缺源、坏源、块/AV不匹配、非法ID、源/目标符号链接与Context关闭均失败，不回退全局。此为现有兼容runtime的Context装配，不能视作Native多workspace路由或正式源写已启用。

R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。CSV控制器不再整体列todo；下一步为HTML/PDF等其他导出控制器、设备工具/旧标注依赖，之后处理其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限和后台退出切换。真实Excel、复杂跨库递归、外部Pandoc/PDF设备能力需独立验收，不能由CSV合成样例推断。

### R02：HTML预览源与派生树归属（2026-10-30）


本批完成/api/export/preview的HTML片段预览Context装配。HTTP传递Request.Context到PreviewContext；原Preview签名保留Background兼容wrapper。源候选由既有独立聚焦准备和稳定源身份端口产生，原exportTree使用本次导出的引用/动态文本/嵌入查询/脚注/标题/AV端口和捕获配置，不读取当前全局Export/Appearance。预览仍按原行为固定#标签包裹、过滤隐藏列、处理标题级别和完整引用内容；AV卡片/看板保留原ID-only表格投影规则。

renderHTMLPreview保留原内存转换流水线：强制行内语法、启用脚注、添加块IAL、调整聚焦标题、删除超级块外侧IAL、为脚注后括号文本插入零宽空格，然后Markdown格式化/重新解析/ProtylePreview，关闭远程图片标记。该格式化步骤直接调用原lute.FormatNodeSync并返回错误，避免通过原FormatNode的fatal日志路径退出；全库FormatNode未修改。

CSS变量在这次格式化派生的局部树上应用本次捕获的主题值，复用原主题合并/解析/回退算法及受约束的主题资源读取，不放宽通用源树主题端口的身份检查。主题首读缓存和配置标量保持独立；后续全局配置或主题文件变化不重定向同一次预览。源码候选与派生树均不写回.docj/AV JSON，不创建导出产物或临时打包目录。

HTTP成功仍返回html/fillCSSVar字段，原User-Agent判断保持：Chrome等支持环境保留变量，Safari等需替换的环境读取主题值。合法但缺失块仍返回空html和原fillCSSVar值；非缺块的Context/配置/依赖错误返回code=-1且无部分data。取消在准备、遍历、格式化前后和最终渲染后检查，不降级为全局服务。此为现有兼容runtime的Context端口，Native多workspace路由和正式源写仍未启用；请求首读与配置捕获不是全仓原子快照，无新增R04持久receipt/回滚/骤停恢复保证。

R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。/api/export/exportPreviewHTML的PDF/图片预渲染、完整HTML/MarkdownHTML文件导出、合并子文档、设备字体/脚本/图标资源复制及PDF/DOCX工具链仍待独立迁移。下一步从这些文档准备和资源依赖继续，之后推进其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限与后台退出切换；HTML片段预览不再整体列todo。

### R02：导出目录与子文档源归属（2026-10-30）


本批完成HTML/PDF/DOCX共用的文档准备与子文档合并Context中层端口。prepareHTMLDocument复用已有聚焦准备，返回本次独立树/选中节点/定位；merge开启时由显式guard/list/load服务完成目录枚举和子文档读取，缺端口或错误不回退全局，失败不向调用者交付部分树/节点/定位。完整文件导出控制器尚未装配该端口，设备资源/工具依赖仍待；这与上一批已完成的/api/export/preview片段预览是不同范围。

原mergeSubDocs/buildBlockChildren/walkBlock/loadTreeNodes保留无端口兼容路径，新增可选Context服务，复用原插入与标题算法：按目录排序后的深度优先顺序展开，逐文档合成标题、标题ID沿用源文档ID、深度超过6时标题限制为6，不调整原正文标题；保留空父文档插入点及末尾空段落规则。修复walkBlock对loadTreeNodes错误漏赋给返回值的问题，旧路径也不再吞掉这一加载错误；上层完整文件控制器的错误/产物处理仍需各自迁移验收。

拥有Context的目录服务复用doctree.List的隐藏/笔记本优先排序/全局排序/自定义sort/每目录MaxListCount/引用计数/子文档数规则，捕获FileTree标量；笔记本配置、排序文件和计数从所属源与SQL读取。展示用的人类化时间字符串不参与此内部列表，仅原元数据排序字段用于合并。缺配置文件使用原默认值，不创建配置；坏配置/坏文档直接错误，不搬移损坏文档，也不清理过期.tmp。目录、文件和元数据路径限制在选定笔记本，拒绝穿越/符号链接与关闭后的读取，受借用root关闭错误约束。

统一抽取exportTreeAt，从本次已记录原字节重新解码独立候选；嵌入引用继续先做原registry定位核验，目录发现的未入索引子文档可按受约束路径读取，不要求先写索引。Properties身份与原根身份校验，旧Spec适配仅在候选中进行；排序元数据使用原字节与源属性，不复用全局文档属性缓存。合并后的父根保留既有来源绑定，标题投影可用，子文档候选的拼接不修改保存的原树或.docj/未知字段。目录/配置/SQL/文档首读并非全仓原子快照，无跨进程改名CAS或R04持久receipt/回滚/骤停恢复保证；Native正式源写/多workspace路由未因此启用。

R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。文档合并中层不再整体列todo；下一步补HTML/MarkdownHTML文件及PDF/图片预渲染的脚本、字体、主题、图标、表情、附件和输出能力，再实际装配文档准备端口。之后继续其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限和后台退出切换。

### R02：导出设备根与私有资源目标（2026-10-30）


本批完成HTML/MarkdownHTML及PDF附件阶段共用的资源复制Context中层端口。copyHTMLResources只接受本次已登记的源候选与本次archiveExportOutput私有payload，拒绝外来克隆/输出；附件继续复用已有显式AV读取、资源索引/全局优先及标注选中文件映射、URL查询/片段处理、rtfd目录/隐藏文件/空目录复制。static=false只复制附件，沿用PDF等无需打包程序静态目录的分支；static=true增加原HTML/MarkdownHTML的程序资源和表情。端口不发布ZIP，由后续完整控制器在全部步骤成功后发布。

捕获所属Appearance模式/主题名/图标名、WorkingDir与AppearancePath设备根，以及原生emoji静态表；配置标量与目录选择不跟随之后的全局变化。保留原主题根别名支持：显式配置的设备根/选定主题或图标根在捕获时解析为物理目录，记录目录身份，复制前重新校验；这仅授权选定根，不代表递归跟随内部链接。主题/图标名单级路径校验，目录内部文件/目录及目标链接仍由Source约束拒绝，已被替换的根失败。捕获仅冻结选择/目录身份，不冻结资源文件字节，不构成跨文件或SQL/文件原子快照。

复制布局保留stage/build/export、stage/protyle（含字体）、选中主题全目录、当前图标icon.js与自定义图标的material后备，图标其他文件不额外复制。空图标/ant/material遵循原后备规则；缺失的可选图标/表情保持省略，真实IO/权限/越界错误向上传递。表情路径在所属data/emojis内，保留原文档图标/NodeEmojiImg及去重规则，支持原生中文文件名和编码URL；坏引号或编码穿越返回错误，避免原字符串截取可能的panic。原生/远程/API图标不当作本地表情读取，不新增网络请求。

资源流复用既有有界复制、源/目标检查、文件模式/时间及私有临时文件替换，借用设备/数据root的关闭错误加入返回值。必需程序目录/主题缺失失败，部分资源仅留在私有工作目录，失败不发布；Wiki源.docj/附件/表情及设备资源均不写回。旧无端口ExportHTML/ExportMarkdownHTML控制器保持原路径，新端口与其在合成资源夹具下做布局/字节对照，不能据此宣称完整HTTP控制器已隔离或真实发布资源已验收。

R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。资源复制中层不再整体列todo；下一步为完整HTML/MarkdownHTML及PDF/图片预渲染的原渲染/输出/错误返回与设备工具依赖，实际装配已有文档准备、合并和资源端口。之后继续其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限和后台退出切换。Native正式源写/多workspace路由、R04持久receipt/原子恢复、真实PDF/DOCX工具及实际应用资源全量发布不在本批新增验收范围。

### R02：所属预渲染读取与原生DOM（2026-10-30）


本批将原/api/export/exportPreviewHTML实际控制器接入请求Context及所属导出读取端口，保留id/image/keepFold/merge参数和id/name/content/attrs/type响应字段。该接口为PDF/图片提供预渲染DOM，不保存文件；image仍沿用原PDF式DOM处理，不新增图片渲染行为。不存在的有效块保留空结果，取消、依赖失败及坏iframe引号返回错误而非部分成功内容。

复用前批所属文档准备/合并，串联原PDF目录锚点、exportTree引用/AV渲染、标题层级调整、iframe转链接及ProtyleExportRenderer。直接位于blockquote下的标题沿用原规则不生成目录锚点，keepFold/合并/焦点范围及原始HTML组件保持原行为。attrs/type在iframe转换后读取，与原接口一致；不把转换前的NodeIFrame类型返回给原UI。捕获本机链接前缀，模型内重写本机/#锚点，控制器不再在请求末尾读取全局端口；配置/源树/解析器/查询依赖继续通过已绑定Context端口读取。

目录标记、iframe转换和最终HTML renderer抽为原文件导出及新Context共用函数，原无端口文件入口仍保留兼容包装。新端口循环与渲染前后检查Context，失败不返回部分DOM；缺少iframe src保留原文本回退，src引号不闭合改为明确错误，避免原字符串截取panic。该只读路径不写回.docj/AV/附件，不创建导出文件或调用设备工具。捕获不构成跨文件/SQL原子快照。

R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。PDF/图片预渲染HTTP装配不再整体列todo；完整HTML/MarkdownHTML文件导出仍需串联已有文档/资源/私有输出端口并传播错误，ProcessPDF后处理及Pandoc等设备工具仍待。其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限和后台退出切换继续后续串行推进。此处仍为原兼容运行时接入，不代表Native正式源写、多workspace路由或R04持久receipt/原子恢复已完成。

### R02：所属文件渲染与输出消费生命周期（2026-10-30）


本批完成HTML/MarkdownHTML文件导出的Context模型装配端口withHTMLFile：按原分支选择所属文档/合并/焦点、HTML或MarkdownHTML renderer，再复制原静态设备资源/主题/图标/表情与所属附件到本次私有payload，最后调用受生命周期约束的消费者。PDF预备分支仅复制附件，MarkdownHTML/Word预备分支保留原静态布局。调用者不能在此传入任意savePath；借用输出仅在消费者内存活，端口统一关闭与清理。

新增renderMarkdownHTMLDocument，将原exportTree引用/AV配置、文档名、可选Word iframe转换及MarkdownHTML renderer接入所属Context。MarkdownHTML沿用原ProtylePreview，Word预备DOM沿用ProtyleExportDocxRenderer；不套用ExportHTML的标题层级调整。原表情/emojis相对路径、列表起始值/首条编号及行内code后零宽空格处理抽为共享函数，原ExportMarkdownHTML亦调用该函数，避免分叉；其文件资源/保存路径控制器仍为旧分支。新路径读取捕获配置和解析器，检查树归属，查询/循环/渲染失败不向消费者交付部分内容。

私有输出复用已有archiveExportOutput：渲染或资源复制失败不调用消费者；消费者失败、取消、panic或最终Context检查失败时清理本次工作目录和已发布ZIP；只有消费者正常返回且最终检查成功，才允许保留它已发布的ZIP。正常仅消费DOM/资源而未发布时也清理私有目录；关闭错误加入返回值，借用输出结束后拒绝再次使用。此处未新增跨请求receipt或原子快照承诺。

原UI的文件导出仍为后端准备资源/DOM、前端补齐HTML，再由浏览器exportBrowserHTML打包或桌面前端写index.html的两阶段流程。withHTMLFile是完整模型装配端口，尚未装配exportHTML/exportMdHTML/exportBrowserHTML的跨请求目录交接、桌面保存路径授权及完整错误返回；不宣称三个HTTP控制器已经隔离。PDF后处理/实际Pandoc调用、真正Word文件生成和实际应用完整静态资源发布亦未在此新增验收。

R02仍doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。HTML/MarkdownHTML完整模型装配与MarkdownHTML渲染端口不再整体列todo；下一步为保留原UI两阶段协议，接入临时资源目录的交接/归属/关闭、原browser打包回调及桌面savePath控制器，再推进ProcessPDF/设备工具、其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限和后台退出切换。此处仍为原兼容运行时端口，不代表Native正式源写、多workspace路由或R04持久receipt/原子恢复已完成。

### R02：实例所属HTML目录交接（2026-10-30）


本批将exportHTML/exportMdHTML的空savePath分支及exportBrowserHTML实际HTTP控制器接入请求Context，保留原id/name/content/folder和zip响应、前端补齐HTML后回调打包的两阶段UI协议。HTML保留pdf/keepFold/merge语义，MarkdownHTML沿用原控制器不合并的行为；桌面非空savePath仍为旧分支，本批不宣称已隔离。准备阶段调用前批withHTMLFile完成所属渲染/资源复制/私有输出，再将资源复制到所属temp/export内的独立目录，成功关闭和最终检查后才返回folder；失败返回code=-1且无部分成功Data。

每个AppConf拥有私有handoff表，记录temp/export/资源目录的身份和创建时间，不持有跨请求os.Root或请求Context。后续回调重新捕获所属读取端口，只接受该实例登记的单级folder；不存在记录、路径穿越、外来目录、已替换根/目录与内部链接均拒绝，不通过文件系统中“碰巧存在”获取权限。目录名保留html-/htmlmd-前缀但改用唯一ID；一条记录最多消费一次，实例内互斥避免并发回调重复打包。只有目录身份被记录，资源内容不是不可变字节快照。

回调将资源有界复制到本次私有payload，只在私有副本写index.html，再用既有检查后的关闭文件/硬链接ZIP发布；保留原平铺ZIP布局和URL字段，文件名改为唯一值，不覆盖同名已有包。回调结束前关闭借用root、清理交接目录，再决定是否保留本次ZIP；复制/写入/关闭/清理/最终Context失败不返回下载URL且清理本次发布。目录身份不匹配时拒绝删除外来替换，返回错误；不能据此承诺能清理攻击者或其他工具移走的旧目录。

最多32个待回调目录，30分钟有效期：新准备请求惰性清理过期记录，过期回调失败并清理；没有周期扫描后台线程。实例关闭/原repo关闭和legacy服务退出清理已登记目录并注销closer；登记表不序列化，进程重启后旧folder不能继续回调。崩溃遗留临时目录不自动认领，持久receipt/跨进程恢复仍属R04。写入权限仅显式授予本次临时输出，目录根逐成员复制，不放宽Source.Check对根路径/链接的约束，不授予Wiki源写权限。

真实导出资源构建发现webpack.export缺少SINGLE条件定义，补为false后原npm run build:export可完成；无UI设计或HTTP参数改动。R02仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变。浏览器两阶段临时目录交接/打包回调不再整体列todo；下一步为桌面非空savePath的路径授权/资源输出/错误传播，以及ProcessPDF/设备工具，再继续其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限和后台退出切换。本批不代表Native多workspace路由/正式源写或R04原子恢复已完成。

本批打包同步：统一wiki-app脚本新增webpack.export构建，输入门禁要求非空protyle-method.js与base.css；相应缺失/空文件测试及IronMan手动构建README同步。本地依赖模式下完整wiki-app资源构建通过，仍保留构建前stage/build快照恢复。此验证不包含Tauri/DMG或实际发布。

### R02：显式桌面目录授权与原生保存（2026-10-30）


本批将exportHTML/exportMdHTML非空savePath桌面分支实际控制器接入请求Context，保留id/name/content响应及原前端生成最终HTML的分工。HTML继续使用pdf/keepFold/merge，MarkdownHTML仍沿用原不合并控制器语义；调用前批withHTMLFile完成所属文档/渲染/资源准备，准备失败时不新建外部目录。桌面保存资源不再调用无端口ExportHTML/ExportMarkdownHTML；旧函数仍供尚未迁移的其他业务兼容。

显式桌面目录授权只覆盖所选输出：要求绝对路径、拒绝上跳/反斜线/文件或最终目录链接；解析并捕获已存在祖先的物理目录及身份，支持macOS /var、/tmp及显式父级别名，缺失子目录经借用root创建。后续检查父级别名、根身份和新建子路径，拒绝父级/根替换及内部链接。输出与所属workspace/data/temp、repo/history/conf及捕获的程序/主题/图标资源根双向重叠时拒绝，包括祖先路径和保守大小写别名；共享文本前缀的路径组件兄弟目录允许。不得把输出权限用来写Wiki源。

私有payload准备后，对既有输出完整预检：所有资源的文件/目录类型、链接及原前端将写入的index.html均先检查，再逐项复制。保留原资源合并/同名资源覆盖和无关文件保留；不会由后端提前覆盖index.html。复制有界、逐文件临时替换、显式临时/所选输出授权，最终检查及借用root关闭错误向上传递；失败返回code=-1且无部分成功Data。输出目录是顺序合并，并非多文件原子提交：外部复制中途失败时，已完成的资源可能保留，不删除既有输出或承诺回滚。

原Save流程补齐失败响应分支，失败不再继续onExport包装/写盘；原最终HTML写入提为nativeHTMLFile小模块，独占创建本目录临时文件、写入/fsync/关闭，再rename替换index目录项，避免直接截断符号链接或硬链接目标。冲突不删除外来临时文件，失败按文件身份清理已创建临时文件并显示错误；原路径/模板/导出后行为保持。后端最后检查到前端最终写入之间仍有跨请求窗口，不宣称阻止其他进程在该窗口重定向整个目录，也不宣称跨文件CAS/事务；最终Native身份协议与R04/R06仍待。资源/HTML字节来源均不写回Wiki源。

原生Electron renderer构建配置补SINGLE=false以编译该原生保存分支，输出验证放在临时目录；统一wiki-app构建和独立原生构建均需验证，无新增依赖/命令/打包布局。R02仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变；桌面HTML/MarkdownHTML资源保存控制器不再整体列todo。下一步为ProcessPDF与设备工具，再继续其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限和后台退出切换。本批不代表Native多workspace路由/正式源写或R04原子恢复已完成。

### R02：PDF后处理文档与所属书签读取（2026-10-31）


本批完成ProcessPDF的所属文档准备及书签Context端口。preparePDFPostprocessDocument先通过所属块索引定位根文档，再加载完整所属文档并按原排序合并子文档；即使传入标题ID，也保留原ProcessPDF加载整个文档的语义，不误用HTML预渲染的焦点裁剪。标题直接位于blockquote下时沿用原排除规则；资产目标经显式AV读取端口收集，供后续附件阶段使用，仅收集不嵌入/复制。候选与文档/索引读取均受原Context归属检查约束，不创建输出或写回docj/AV。

书签生成使用捕获的所属SQL句柄及请求Context读取标题，查询/取消/关闭/候选归属失败向上传递，无全局SQL回退。原processPDFBookmarks与PdfListToCLinks改为共享错误返回算法的兼容包装，仍供原ProcessPDF路径调用；保留页码排序、URL QueryUnescape、重复标题控制字符去重、页面/纵向定位和标题等级树。原兼容包装仍按旧调用方式记录错误，不能据此宣称完整HTTP错误链已迁移。

修复原算法在已有父书签时遇到无PDF链接的子标题会空指针的问题：缺少链接或SQL条目跳过该节点，其后有链接的标题按等级挂接到现有祖先。重复/空标题ID或非法等级拒绝，避免循环书签；nil PDF上下文和错误批注类型返回错误而非panic。生成完整书签计划后才调用pdfcpu.AddBookmarks；该调用只修改调用者持有的内存PDF Context，不输出文件。pdfcpu发生内部修改错误时不承诺回滚内存上下文，未来文件控制器必须只在所有步骤成功后发布。

真实PDF验证使用显式测试配置和私有输出，不初始化用户字体目录或改pdfcpu配置。pdfcpu.AddBookmarks不会立即刷新内存目录缓存，验收按写出并重新读取PDF后的目录进行。原ProcessPDF仍含全局DisableConfigDir/UserFontDir/字体装载、附件链接/嵌入、水印及VersionStr/文件写回依赖，完整HTTP控制器本批未切换到新Context端口；旧路径回归仅证明共享书签算法兼容，不等于其他PDF功能已隔离。

R02仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变；PDF完整文档/标题及书签Context中层不再整体列todo。下一步串行推进附件链接/嵌入、水印/字体和pdfcpu全局配置/版本依赖，再装配带身份校验与失败传播的PDF文件读取/替换和原processPDF实际控制器。后续仍有其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限及后台退出切换。本批不代表Native多workspace路由、正式源写或R04原子恢复完成。

### R02：所属PDF附件读取与有界嵌入（2026-10-31）


本批完成PDF附件链接/嵌入Context中层。pdfPostprocessDocument.assetLinks在所属候选检查后借用data根，复用原附件索引的notebook遍历/全局优先规则；只准读取该文档收集的附件及显式目录附件的成员，拒绝PDF额外插入的未选中本地附件。URL只绑定捕获的本机origin，其他端口/外部链接保留；支持/assets与/export/temp/assets形式，路径只解码一次，查询参数不参与文件定位，片段保留给相对链接，编码的中文/空格/井号/百分号按真实文件名定位，拒绝上跳、反斜线、坏编码与源链接。

保留原两个分支：removeAssets=false把当前本机资源链接改为相对URI；true将源文件流嵌入PDF，并沿用原FileAttachment标注位置、颜色、时间、文件名/UF和FileSpec结构。附件描述改为attached by wiki。新端口直接从受约束os.Root打开文件，经取消/owner/源根身份检查的64 KiB分块reader送入pdfcpu.NewEmbeddedStreamDict，不向库暴露全局绝对路径；单文件64 MiB、单次累计256 MiB，stat预检和增长时读取检查均执行。不创建输出文件，不写回docj/AV/附件。

共享applyPDFAssetLinks供新Context端口与原processPDFLinkEmbedAssets兼容包装使用，PdfListLinks亦共用安全批注扫描。修复原“移除全部批注再重建链接”的行为，只替换Link批注，保留笔记/标注；普通外部URI在新端口中不做额外解码。兼容包装保留旧宽链接分类/外链解码及单项嵌入失败跳过规则，仍用旧绝对路径解析器；因此兼容包装被调用不代表完整ProcessPDF已隔离。新端口的源读取/限额/库错误直接返回，调用者必须丢弃失败的私有PDF；pdfcpu内存修改不是可回滚事务。

缺失资源沿用原规则：无任何可解析文档附件时不处理链接；已有资源触发处理后，缺失文件在嵌入模式省略，相对链接模式保留其相对URI。目录资源成员可读取；目录本身作为嵌入流不支持，新端口返回明确错误，兼容入口继续原单项失败跳过。所有借用文件/root关闭错误向上传递。新端口不读取字体、改全局配置/版本或新增后台任务，资源身份检查不等于跨文件字节一致性快照。

R02仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变。PDF附件链接/嵌入中层不再整体列todo；下一步为水印/字体及pdfcpu全局配置、版本依赖，然后装配PDF文件身份/读取/成功替换/错误链和原processPDF实际控制器。其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/行历史、历史/快照独立源修复权限及后台退出切换继续串行推进。本批不代表完整PDF HTTP已Context化、Native多workspace路由/正式源写或R04原子恢复完成。

兼容修复补充：原接口前置存在性检查未解码文档中的百分号编码地址，会把真实中文/空格附件当成不存在而整段跳过；兼容包装现在仅对文档目标做一次规范化后检查，链接路径仍按原流程解码，不把已经解码的文件名二次解码。该修复由真实WebKit PDF/原processPDF回归发现，已补编码文件名兼容测试。

附件类型补充：嵌入前先stat确认普通文件，再open并复检，避免命名管道等特殊文件在打开时阻塞；新端口报错，兼容入口继续单项跳过。该检查有实际临时FIFO回归，不授予特殊文件读取权限；不宣称跨进程文件类型CAS。

### R02：所属PDF水印与设备端口（2026-10-31）


本批接入PDF水印设置、素材选择和中文字体选择的Context中层。捕获导出设置时一并冻结水印值、描述、文字/图片/PDF模式及默认字体资源身份；保留原“路径不存在即作为普通文字”的规则，后续出现同名文件不改变已捕获模式。空值和禁用水印保持no-op；只在实际启用的非空水印上报告依赖或素材错误，不阻断其他导出。

pdfPostprocessDocument.watermark使用所属文档检查与显式设备端口，复用原pdfcpu文字/图片/PDF解析及AddWatermarks和OnTop=true行为。图片和PDF交给解析器的是内存字节，不再让库自行打开素材绝对路径；素材和字体只读，支持显式文件别名但捕获后必须保持同一物理目标、父目录及文件身份/大小/修改时间。读取前检查普通文件与大小，读取中每64 KiB检查owner/取消/身份，素材32 MiB、字体16 MiB上限；文件和root关闭错误返回，失败的私有PDF须由调用者丢弃，不承诺内存回滚或跨文件一致性快照。

中文文本通过受信宿主提供的字体枚举/安装端口选择原有字体，支持库返回的带附加信息名称；描述参数排序确定化，预装指定字体时无需安装能力，找不到时沿用appearance/fonts/LxgwWenKai-Lite-1.501/LXGWWenKaiLite-Regular.ttf回退。所有端口错误向上传递，不向HTTP/CLI暴露回调；未绑定设备时拒绝执行，不隐式借用兼容进程。原processPDFWatermark包装显式复用共享核心，保留原UI错误通知方式。

边界：compatibilityPDFWatermarkDevice仍使用pdfcpu全局字体缓存、UserFontDir及默认配置；中文/PDF真实库测试在独立子进程及私有字体目录执行，不能据此宣称生产设备执行器已隔离。完整ProcessPDF仍有全局字体目录/配置/版本设置、旧文件读取/替换及错误控制器待改。本批未新增输出发布能力，也未完成完整HTTP多workspace装配。R02保持doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：所属PDF字节任务与一次性设备进程（2026-10-31）


新增internal/pdfdevice的一任务一进程执行器与所属pdfPostprocessDocument.watermarkBytes中层。受信宿主通过Bind捕获绝对可执行文件及别名/文件身份，传入显式字体TTF和版本；调用前后验证执行文件身份。调用者的PDF与字体切片复制后传输，素材/默认字体沿用上一批的身份、有界读取和owner检查。设备失败或owner关闭时不返回任何产物，没有路径发布能力，也不隐式退回原全局状态。

wiki-kernel、wiki-cli、wiki-compat均在正常启动前识别内部--internal-pdf-device入口。父进程通过stdin发送严格JSON字节任务，只含PDF、文字/图片/PDF水印、字体字节和捕获版本，不含知识仓/素材/输出路径；stdout只返回PDF，失败非零及固定错误。子进程使用独立0700工作目录和字体目录，清空继承环境，仅提供私有HOME/TMPDIR及系统必需SystemRoot；pdfcpu默认配置、字体缓存/目录与Wiki producer版本修改均发生在一次性进程内。父进程有60秒上限、取消kill与Wait、有限输出/stderr和临时目录清理，清理错误亦丢弃结果。

输入PDF64 MiB、输出128 MiB、素材32 MiB、单字体16 MiB、字体累计32 MiB/最多8个、JSON传输256 MiB；描述16 KiB、文本1 MiB，拒绝未知字段、额外消息和路径型字体名；显式字体标识限1–128位ASCII字母/数字/下划线/连字符。复用原字体描述选择与水印解析算法，中文判定保留Han/Lm/平假名/片假名/韩文；支持显式预装字体字节及默认中文字体回退，父进程不读取pdfcpu用户字体缓存。库运行错误只返回失败，不发布部分PDF。

范围：本批具备可实际调用的独立设备和所属水印字节入口，尚未将完整书签/附件流水线及原ProcessPDF控制器装配到设备；原兼容接口仍沿用旧配置/版本/文件读写。旧用户字体库存的所属捕获/缓存适配也随完整装配继续推进。不能将独立执行器通过视为完整HTTP多workspace已隔离。后续需完成PDF文件身份/读取/成功替换与错误链，再处理其余R02依赖、历史与后台生命周期。R02仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变。

同步影响：没有新增可分发程序或第三方依赖，内部入口复用现有三个程序；公开CLI命令/参数及帮助、原UI/导航和资源清单保持。新入口已补真实三程序回归和依赖边界审计，打包脚本继续构建原三个入口，无需新增资源项。相对上一批未压缩临时二进制，kernel约30.3→36.3 MiB、CLI约29.6→35.8 MiB、compat约101.1 MiB不变；这不是最终压缩DMG大小。本批不提交/push、不构建DMG或安装。

### R02：所属PDF标题、附件和字体库存装配（2026-10-31）


本批完成所属processPDFBytes字节后处理流水线：从所属SQL捕获所选标题后，先由一次性设备进程预检当前origin的本机附件链接；父进程按原文档附件选择范围和显式目录成员解析索引、读取必要材料，再由第二个一次性进程依次执行原书签、附件相对链接/嵌入、水印及版本/序列化，返回前重新读取并校验PDF。两个阶段共享捕获的输入PDF，错误、取消、owner关闭或借用root/文件关闭失败均不返回产物。字节入口没有文件发布能力，尚未接入原processPDF HTTP控制器。

书签和附件算法迁入internal/pdfdevice供新设备及原兼容包装共同调用；保留缺失嵌套标题跳过、标题解码/重复名标记、附件布局/颜色/时间、普通外链及笔记标注保持等业务规则。标题只从所属文档所选heading的SQL记录捕获；设备不查询旧全局SQL。附件预检只返回当前origin链接标识；父进程沿用notebook遍历/全局附件优先索引，只读取文档附件或显式目录下实际被引用的成员。路径只解码一次，保留URL片段、忽略查询参数，拒绝未选中路径、穿越及源链接；目录引用有无末尾斜杠均可识别。相对链接分支不读取附件内容；嵌入分支保留缺失源省略规则，空文件必须明确标为已读取，未加载材料不可静默嵌入为空文件。目录本身不可作为嵌入流。

旧字体库存按设备资源只读捕获：兼容来源为home/.config/siyuan/fonts，不加入知识仓。捕获目录/普通.gob文件元数据与名称集合，最多8192条；仅在文字水印确实选择库存字体时读取该缓存（包含中文和拉丁文字），捕获后新增字体不会自动混入，替换/变更/源链接/限额失败明确返回。未知字体仍走原默认字体回退；坏的未选中缓存、禁用或未指定库存字体的普通文字水印均不读取这些字体文件的正文。旧gob在子进程私有目录中由原字体加载器解析，不在父进程解码或加载全局缓存；保留库存文件名作为查找key，即使与内部PostScript名不同。显式TTF也在私有安装目录中转为所选key，父进程和原缓存均不写回。

协议新增严格标题/附件清单、附件预检和互斥的TTF/旧缓存字节。单附件64 MiB、附件累计256 MiB、最多4096附件及预检链接，预检响应4 MiB；标题最多8192、单标题16 KiB/累计8 MiB。原PDF64 MiB、输出128 MiB、素材32 MiB、单字体16 MiB/累计32 MiB和每进程60秒不变；为容纳附件base64，JSON传输上限由256提升至640 MiB。字体标识扩展为Unicode字母/数字/下划线开头，其后允许字母/数字/下划线/点/空格/连字符，最多128字符，仍拒绝路径型名称；附件时间必须为可编码的PDF日期。设备只获得相对资源标识及字节，不获得知识仓、素材或最终输出的绝对路径。

R02仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变。完整字节流水线及选中旧字体库存适配不再整体列todo；下一步为PDF输入/输出文件身份、读取、成功替换与完整错误链，以及受信设备绑定和原processPDF控制器实际装配，再推进其余R02源写/历史/后台生命周期。此处的一次性本地设备进程不是CLI的网络server，不代表Native多workspace路由/正式源写或R04原子恢复完成；身份检查也不等于跨文件一致性快照。

同步影响：两README、设计/基线/计划、原兼容/设备/模型及三个真实入口测试已同步；公开CLI命令/帮助、原UI/导航、依赖和打包资源清单不变，无新增分发程序。现有打包脚本继续构建原三个入口。临时未压缩二进制kernel约36.5 MiB、CLI约35.9 MiB、compat约101.2 MiB，不代表最终DMG体积。本批不提交/push、构建DMG或安装。

### R02：所属PDF文件控制器与提交状态（2026-10-31）


本批将原/api/export/processPDF切换到ProcessPDFContext(c.Request.Context())，再由所属文档、独立设备字节流水线及受约束文件控制器完成后处理。原ProcessPDF签名保留为Background兼容包装；HTTP实际入口使用请求取消信号。设备由受信宿主绑定当前可执行文件的内部处理入口，不从请求接收执行路径、不搜索PATH、不回退旧全局处理。原ProcessPDF路径不再直接修改父进程pdfcpu配置、字体目录/缓存或版本；旧私有书签/附件/水印包装只为兼容与回归保留，不再构成实际后处理路径。

目标必须是已存在的普通.pdf文件，绝对路径，不允许上跳、反斜线、叶级符号链接或目录；输入64 MiB、输出128 MiB。显式选中父目录的系统/用户别名解析为物理目录后捕获身份，处理期间复核别名、目录及文件inode/类型/大小/时间/权限。拒绝捕获的workspace/data/temp/repo/history、应用资源目录内的目标，及与水印素材相同inode的文件；这不是未装配Native StoreRoot路由的跨store授权。原文件以不截断的读写权限打开，避免同目录替换绕过只读文件的访问限制。原前端生成raw PDF、选择路径、后处理和完成回调的业务顺序保持，不在本批新增原生UI写桥。

读取与写入按64 KiB检查所属Context/取消及身份；保存首次输入SHA-256，提交前重新读取原文件，既检查元数据也比较摘要，检测恢复mtime后的同尺寸修改。新产物先写到目标同目录随机、排他创建的.wiki-pdf-*.part，校验尺寸、摘要和身份，保留原权限位，文件Sync/Close后再次确认原文件与临时文件，再用同目录Rename替换并同步目录。新文件inode会变化；不承诺保留原owner/ACL/xattr。普通处理、写入、同步、关闭或rename前失败保留原文件；清理只删除仍匹配本次创建inode的临时文件，被替换的外来条目不删除。

新增PDFProcessError，稳定Code用于HTTP，完整原因通过Unwrap保留（包括errors.Is取消及加入清理错误的链）。Rename完成即记committed；此后目录Sync、身份复查、取消或关闭失败不做危险回滚，错误文本明确PDF已更新，HTTP code=-1且data={committed:true}。提交前错误仍code=-1/data=null，成功仍HTTP200/code=0/data=null；参数缺失或类型错误明确返回失败，不再断言panic。committed只说明本次替换发生，不保证其后没有外部修改或已确认耐久。身份/摘要前置检查与单次rename不是对不合作外部进程的跨进程CAS，也不是多文件恢复事务。

实际原接口现在复用上一批规范编码的相对URI：中文/空格及特殊字符以可解析URI保存，而非旧包装的裸字符串。原前端回归用真实file URL解析和读取目标文件验证定位，嵌入分支继续检查原附件字节，外链及笔记保持。该编码表示变化不会改变接口成功envelope；失败现在实际传回，原fetchPost的错误处理阻止误触发成功/附件清理回调。未更改原UI布局、命令帮助、导航、依赖或分发资源清单，无新增程序。

PDF文件控制器、受信设备绑定、完整字节后处理及原HTTP请求Context接入不再整体列todo。R02仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变；继续串行推进其余导入清理/借用root关闭、writer/属性/缓存/广播/私有wrapper/handler/闪卡map调用者/行历史、历史/快照独立源修复权限及后台完整退出切换。本批实际HTTP仍运行在现有兼容服务与owner捕获边界，不代表Native多workspace路由、原生UI桥或R04原子恢复已完成。本批不提交/push、DMG或安装。

### R02：归档导入借用IO的幂等错误关闭（2026-10-31）


本批完成ImportSY文档归档调用链的借用root收尾。ArchiveImportContext.openScratch现在返回可报告错误的幂等关闭函数，独立借用描述符与临时目录owner分离，关闭能力在借用时捕获；借用子目录时父句柄的检查、交接关闭或子目录打开失败均关闭已取得句柄并合并错误。owned归档输入文件的父借用关闭也复用该能力；异常情况下关闭已打开文件，不返回可继续消费的文件。关闭能力属于受信资源后端，未加入HTTP/CLI参数或持久配置。

AV临时映射、文档回写、目录改名、storage移除及prepare阶段均通过errors.Join归还关闭错误；关闭失败时不返回成功样式的ID映射或prepared结果，prepare仍回收本次独占解包目录。发布资源、合并sort、挂载和复制阶段改从同一openScratch入口取得描述符，owned临时目录不再在这些阶段绝对路径重开；外部显式目录保留原普通目录检查与原合并语义。成功/错误/取消时均释放借用资源，不因关闭需要重新选择当前workspace或依赖请求仍然有效。

新增openRepairIOChecked及受信资源关闭后端版本，读/写权限、原文件锁、Remember/Upsert/Written回调不变；读写源的data/workspace root按逆序全部关闭，错误保留、关闭幂等。第二个root打开失败也会归还第一个及其关闭错误。ImportSY合并/排序/挂载使用该检查型接口；loadRepairTree也改用检查型关闭，关闭失败丢弃返回树。挂载仍先排空已接受的固定SQL索引队列，再归还root，晚关闭失败阻止finishArchive调用成功通知。

边界：原顺序发布可能已写入资源、sort或文档；关闭错误的准确上报不等于撤销这些写入，不是R04多文件原子恢复。owner临时目录清理由既有身份校验/幂等close负责，本批不扩张源写权限。为控制串行迁移范围，旧openRepairIO仍保留void关闭适配；当前model生产代码还有27处旧调用，需继续逐项迁移，不能标记所有借用IO均已完成。ImportData完整数据ZIP和ImportFromLocalPath/HTML资产转换等原全局调用链也仍待接入。

R02仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变。ImportSY借用资源关闭与失败收尾不再整体列todo；下一步优先迁移27处旧IO关闭调用，再推进ImportData/本地Markdown导入，以及其余writer/属性/缓存/广播/私有wrapper/handler/闪卡map调用者/行历史、历史/快照独立源修复权限和后台完整退出切换。Native路由/原生UI桥与正式源事务继续按后续阶段验收，不以此提前完成。

同步影响：两README、设计/基线/计划和模型回归同步；公开HTTP/CLI参数、成功envelope、帮助、原UI/导航、依赖、分发程序及资源布局均不变，无需机械修改入口或打包配置。本批不提交/push、DMG或安装，不处理现有其他未提交改动。

### R02：其余借用IO调用的错误完成边界（2026-10-31）

本批迁移上一批记录的27处生产openRepairIO调用，覆盖归档导出选集/资源源读取、AV源/树/封面图片判断、数据库复制占用/预留/创建/绑定、关系及镜像写、冲突排序、增量/笔记本/引用索引、索引订正、同步冲突副本及commitLegacy提交。旧void关闭适配已删除，生产与测试调用统一使用openRepairIOChecked；这仅完成该接口调用集，不表示所有直接os.Root/os.File关闭点或完整R02均已完成。

关闭能力在借用时捕获，为所属indexRepairContext的受信后端，不接受HTTP/CLI参数或全局覆盖。data/workspace root逆序全部关闭，重复关闭返回缓存结果，主错误和关闭错误均保留；显式关闭与defer兜底不会重复拼接同一关闭错误。读取的切片/映射和占用/图片判断在错误时清空或返回false；已完成增量索引的ID结果仅作为错误伴随诊断保留，调用者仍须先检查error。

事务与同步冲突完成通知、引用索引成功状态、重复ID订正刷新和笔记本historyGenerated标记，均在相应root成功关闭后发布；已有队列排空/工作线程等待边界保留，关闭错误不绕开外层订正的固定队列收尾。AV保存/初始化/复制创建在成功关闭后才更新scope版本与缓存，晚关闭失败清除本次条目以允许从实际源重读。关闭失败可能发生于源字节或派生索引已经写入之后，不承诺多文件回滚，也不能把错误伴随ID结果当成成功。

R02总体仍doing，R03–R10 todo；原UI与业务语义优先保留，17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一步串行推进ImportData完整数据ZIP及ImportFromLocalPath/HTML资产转换，再检查仍直接关闭的导出目标root/文件等资源，以及其余writer/属性/缓存/广播/私有wrapper/handler/闪卡map调用者/行历史、历史/快照独立修复权限和后台完整退出切换。Native多workspace路由、原生UI桥和R04正式源事务仍按后续阶段验收。

本批仅调整资源完成/错误传播边界；公开HTTP/CLI、帮助、导航、原UI布局、依赖和打包资源无须变动。两README及设计/基线/计划同步，未提交/push、构建DMG或安装，保留既有其他改动。

### R02：完整数据导入与后台索引准入（2026-10-31）

本批完成完整数据ZIP的ImportData兼容链路。DataImportContext在HTTP解析multipart之前捕获ArchiveImportContext及所属owner、data/workspace/temp、语言、SQL/registry与同步间隔；显式本地ZIP的旧ImportData入口也经该Context执行。锁定与解锁使用同一个捕获owner，等待后再次验证归属/取消/只读/关闭状态；未装配Native跨workspace写路由的边界保持。

上传使用本次独占wiki-data-import目录，解包复用受控ZIP预检和独占wiki-extract目录，拒绝越界/反斜线/符号链接/重复条目及CRC或IO失败。完整数据ZIP与文档归档分别校验：保留单根目录及根下直接docj拒绝规则；data-20时间戳根仅完整数据入口可接受，文档归档入口仍拒绝。保留原合并覆盖、不删除未涉及文件、空目录及原文档/AV ID和元数据，不运行ImportSY的ID重映射或升级。emoji名称清理限定于独占解包源，冲突/改名错误在目标发布前返回；目标通过捕获root与逐文件版本检查合并，目标符号链接不跟随。

借用源与目标root关闭错误、独占上传/解包清理错误均归还；借用句柄和独占目录全部归还成功后，才向捕获owner申请原后台完整索引任务。重建任务的准入与同步计划更新共用owner退出门，失败/取消/拒绝准入不执行同步完成效果。原scheduleFullNotebookReindex也复用同一准入实现，保留原串行task槽、owner任务组、取消及真实等待生命周期。

HTTP importData与importSY共用multipart读取/reader关闭/表单清理助手，仍分别在读取前捕获Context；importData无需notebook/toPath，原HTTP200/code=0/data=null成功封装保留，异常返回code=-1且保留主/清理错误。HTTP reader或multipart的最后关闭清理可能晚于模型层后台任务准入，返回失败不撤销已接受的任务。

边界：完整数据发布仍为逐文件合并，后续错误可留下已写字节，没有跨文件回滚；HTTP成功代表源发布/模型清理完成且后台完整索引任务已接受，并不代表异步索引已经完成，其后错误仍由原owner任务通知。原完整重建算法、UI布局、公开CLI和打包资源保持；不借本批宣称Native多workspace路由或R04正式事务已完成。

R02仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变。ImportData不再整体列todo；下一步单agent串行推进ImportFromLocalPath/本地或ZIP Markdown与HTML资产转换，再检查直接root/文件关闭、其余writer/属性/缓存/广播/私有wrapper/handler/闪卡map调用者/行历史、历史/快照独立修复权限和后台完整退出切换。

两README、设计/基线/计划及测试同步；公开帮助/导航/依赖/资源分发边界未变，无须机械修改。保留其他既有改动，未提交/push、构建DMG或安装。

验证边界：本批完整数据导入专项、完整Go/原前端导入及16项集成通过；受控编辑器浏览器回归两次失败，已确认相关内核/CLI及前端核心包与上一批字节完全相同，尚未修复。该项仍作为验证遗留问题跟踪，不影响本批导入完成项的局部验收，也不能据此宣称R02或发布验收全部通过。

### R02：编辑队列验证归属与导出目标借用root（2026-10-31）

本批先解除上批受控编辑器少尾字符的验证误判。真实键盘/HTTP诊断表明：失败时DOM仍含完整输入，保存队列dirty=true，原测试仅等待源文件12秒便超时；继续等约3331毫秒后dirty=false且完整标记已写入源。该复现证明保存尚在排队，不能将它报告为已确认的数据丢失；不据此推断所有可能编辑问题均不存在。

浏览器回归现在按iframe名称记录独立dirty状态，对对应编辑器在最多30秒内等待保存完成，再执行原精确磁盘内容、源隔离、撤销/重做和原生扩展字段保留断言；原12秒文件断言与240秒整项时限未删除或放宽。新增真实HTTP边界故障场景阻塞最后一个heading保存，验证未完成队列仍dirty、已有文件只含旧前缀且干净兄弟编辑器不能提前确认本编辑器；解除阻塞后仍必须写入完整标记并通过原全部断言。生产Protyle、session、UI布局、输入/保存与撤销算法均未改变。逐键原生快照的串行保存延迟仍存在，可作为性能优化另行评估，不能标为本批已优化。

同时完成归档文档字节、附件/emoji/AV资源及排序投影的3处目标root关闭接入。openExportIO捕获所属archiveExportContext的受信关闭后端，返回独立借用句柄和幂等/缓存错误的关闭函数；不新增HTTP/CLI选项。原输出owner根保持打开，owned输出仍从payload借用，不转回绝对路径重开。所有目标关闭错误与原写入错误合并，资源源/目标同时关闭失败也保留全部原因；取消或归属失效后仍可归还已借用句柄。

晚关闭失败可发生在暂存文档/附件/sort已改变之后，不承诺文件回滚；上层原归档控制流中止后续pack与下载URL交付，并由既有finish(false)清理私有输出。ZIP格式、名称/ID、字节/Spec/opaque字段、资源选择、sort投影和源只读保持。仅完成该归档目标借用root调用集，不代表其余直接文件关闭或全部R02调用已完成。

R02总体仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变。上批编辑尾字符验证误判与这3处导出目标关闭不再整体列todo；下一步单agent串行推进ImportFromLocalPath/普通或ZIP Markdown与HTML资产转换，再处理其余writer/属性/缓存/广播/私有wrapper/handler/闪卡map调用者/行历史、其他直接文件关闭、历史/快照独立修复权限及后台完整退出切换。Native多workspace路由与R04正式源事务仍按后续阶段验收。

两README、设计/基线/计划及测试同步；帮助/导航/CLI参数、生产原UI、依赖与打包资源未变，无需机械修改。保留其他既有改动，未提交/push、构建DMG或安装。

验证收尾：新增5项归档目标关闭回归、专项及完整Go竞态/vet、三入口临时构建、16项集成、原HEAD desktop/single兼容与带最后保存阻塞的完整浏览器编辑回归均通过。浏览器中断运行未算通过，重跑该项完成后才收尾；编辑保存串行延迟未优化，R02仍doing。

### R02：Markdown调用内状态与准备生命周期（2026-10-31）

本批接入ImportFromLocalPath的准备阶段，不将完整Markdown/ZIP导入和HTML资产发布提前标记完成。候选importTrees与searchLinks原为进程全局变量，现移入每次调用独立的markdownImportPreparation；成功/失败/取消均释放本次树和链接表，准备失败的Context保留失败状态，不能换一批树继续当作成功调用复用。十二个并发准备实例可在相同人类路径下使用独立ID和标签配置，不相互解析到对方候选。

准备开始捕获所属ArchiveImportContext、语言及原EditorOptions（Markdown语法、编辑器解析偏好和独立emoji映射）。标准Markdown解析和后续行内合并分别复用原nativeparser.NewStdLute/NewEditor，YAML前言复用doclife.NormalizeCreationTemplate的原业务规则并使用请求Context；保留date生成ID时间、lastmod、title、YAML代码块、缩进代码、禁止自动URL转换及原HTML链接/图片到行内节点和宽高属性转换。纯HTML块转换未执行源文件或附件写入，整个parse前后检查归属/取消。

文档/标题链接索引、相对.md/.markdown链接、wiki链接的动态/静态锚文本、标题锚点、basename回退、未匹配链接保留、标签转换与嵌套行内合并算法保持；只改为读取本次候选和捕获的语法设置。链接与嵌套行内合并的AST遍历逐节点检查取消和所属Context，任一步失败不向原发布循环交付本次候选。标准解析器/HTML解析器内部的单次同步调用仍在返回后检测取消，不承诺强行中断其内部CPU执行。

原ImportFromLocalPath保留三参数调用形式，并增加与既有兼容入口一致的可选请求Context；普通Markdown与ZIP Markdown控制器将Request.Context传入准备阶段。锁定/解锁使用同一个捕获owner，等待和旧事务队列排空后复核归属；候选发布前逐树复核。目录Walk、文档读取和显式附件复制失败现在返回实际错误，避免忽略Walk结果或在错误候选上继续发布；失败后下一次实际单文件导入不会带入前次目录候选。源文件读取、部分资产选择与复制、旧indexWriteTreeIndexQueue、sort及IncSync等发布依赖仍有旧调用，不能以准备层Guard代替完整依赖注入。

边界：本批只完成本地/ZIP Markdown的调用内状态、配置/解析与链接准备及请求取消传递；ZIP multipart入口的共享目录/文件名、解包/上传关闭清理、目标定位、源/目标root IO、SVG/base64资产转换、固定SQL队列/排序/完成通知仍需后续串行接入。原流程可能在准备失败前已复制部分附件，本批无多文件回滚保证，也未授权Native跨workspace写入。生产UI、公开请求字段与成功envelope保持，数据仍写docj，不更改知识仓目录契约。

R02总体仍doing，R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计不变。候选/链接全局残留与准备算法不再整体列todo；下一步先接ZIP上传/解包的独占生命周期与入口前捕获，再推进本地源/目标定位、附件/SVG/base64及写入/固定索引/排序/完成通知；其余writer/属性/缓存/广播/私有wrapper/handler/闪卡map调用者/行历史、其他直接文件关闭、历史/快照修复权限和后台完整退出切换仍待。

两README、设计/基线/计划、模型及HTTP回归同步。帮助/导航/CLI参数、前端代码、依赖、资源清单和打包配置未变，无须机械修改；未提交/push、构建DMG或安装，保留其他既有改动。

验证收尾：新增8项model/1项HTTP测试通过，含12个并发准备命名空间和实际目录读错后再次导入；专项/完整Go竞态、vet、三入口临时构建、16项集成与原HEAD desktop/single兼容通过，新增Markdown导入真实HTTP11项断言保持。受控编辑器本批未修改，产物摘要与上批一致，未重复运行其完整浏览器用例；不将该项算作本批新增通过。准备阶段完成，源/资产/发布和ZIP入口生命周期等仍待。

### R02：ZIP Markdown请求与临时资源归属（2026-10-31）

本批完成ZIP Markdown入口与独占上传/解包生命周期：在解析multipart前捕获请求Context、原解析偏好和owner；上传使用独占0700目录与固定内部文件名archive.zip，解包使用另一个独占root，上传文件名仅经过滤后作为页面根显示名，不参与磁盘路径选择。复用原Markdown导入解析、链接与页面树业务，传入显示根别名，避免把随机临时目录名写进标题或层级。普通本地导入仍保留原三参数兼容入口。

归档解包拆出不限制布局的共享检查层，拒绝路径穿越、绝对路径、反斜杠、链接、特殊文件与重复条目并传播CRC、IO、关闭及清理错误；SY与完整data ZIP继续在外层应用原单根目录/格式规则。ZIP Markdown的模型层等上传文件、输入归档、成员、提取目录和上传目录关闭/清理成功后，才通过捕获owner的完成屏障安排同步；准备/导入/清理失败不执行这次完成步骤，保留多重错误。四个同名上传实例独立持有目录；单个捕获实例不可重复执行。

HTTP共用上传读取/关闭/表单清理辅助层并保持HTTP200/code/data封装，新增的filename参数仅交给Markdown显示名处理。模型完成之后HTTP读句柄关闭或multipart RemoveAll仍可能报错；此时返回失败但不承诺撤销已准入的同步任务或已写源文件。本次无多文件事务/回滚保证，旧本地资产复制和写入过程也可能已部分生效。

边界：本地源/目标root IO与目标定位、附件/SVG/base64转换、旧writer/固定SQL队列/sort及剩余UI通知仍待接入；其余writer/属性/缓存/广播/私有handler/闪卡map/行历史、其他文件关闭及历史/快照修复权限和完整后台退出切换继续串行推进。Native正式多workspace路由未据此启用，R02总体仍doing，R03–R10 todo；既有17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：Markdown正文读取及目标查询归属（2026-10-31）

本批接入普通本地与ZIP共用的Markdown正文读取及目标准备：每次导入打开选定目录的只读os.Root，单文件只登记所选文件，目录通过root.FS扫描登记本次清单；后续遍历、Markdown存在性判断、同名目录/文档判断和正文读取只使用本次root与清单。保留原字典遍历顺序、隐藏文件跳过、目录层级、同名目录与Markdown合并及原解析/引用处理。修正root.FS根节点名为“.”导致被当隐藏目录跳过的问题，传入真实根名称而不改ZIP显示别名。

选择路径的父目录先规范化（兼容macOS /tmp等系统别名），选定节点和扫描到的后代拒绝符号链接及特殊文件。读取前后复核根与文件身份、类型及文件大小/修改时间，传播读取、权限、扫描与关闭错误，复制正文时检查请求取消，入口及各阶段继续验证捕获owner。源root幂等关闭并合并错误，关闭成功后才允许普通导入执行本次同步完成；ZIP仍在外层等待上传/解包资源关闭。目录清单不是跨文件一致性快照，不承诺抵御同inode/同大小/同mtime的全部并发编辑；扫描后新增文件留到下次导入。选定根目录外同名.md不再参与该根的空文档合并判断，避免越过选择边界。

目标准备改用捕获blocktree/配置owner和数据root，检查notebook目录与登记、目标路径、registry查询、源.docj可读性/解析及根ID，不再调用进程级GetBlockTreeRootByPath或在目标缺失时返回成功；借用目标root的关闭失败同样传播。修正目录导入到已有页面下时根HPath遗漏父级的问题，物理层级和显示路径均保留父页面。目标读取检查是准备阶段验证，尚不构成目标版本锁或跨发布阶段CAS。

边界：root限制本批只覆盖目录元数据、Markdown正文和目标准备读取，链接附件复制、SVG/base64与资产目录选择仍有旧绝对路径IO，包含相对链接资产时不能据此宣称完整导入已拒绝全部越界；目标源写入、indexWriteTreeIndexQueue/固定SQL队列、sort及UI通知仍待接入。旧发布忽略写入返回值等问题仍列待办，发生错误仍可能已写部分资源/文档，不提供多文件回滚或持久receipt。Native正式多workspace路由未因此启用；R02总体doing，R03–R10 todo，既有17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：Markdown资产源/目标与流式复制（2026-10-31）

本批完成普通本地/ZIP Markdown导入的链接附件与目录附件读取、资产目录选择和复制Context接入。目录导入只从已有源清单读取附件；单文件导入可在所选文件的捕获父root下借用相对附件，目录附件先做root内扫描/链接/特殊文件检查。所有读取限制在该root内，拒绝越界、反斜杠、链接和扫描后替换；缺失附件仍保留原链接，其他权限/读取错误不再被存在性探测吞掉。沿用原来源路径文件锁，等待后复核归属，复制时逐块检查请求取消与源/目标归属。

捕获data root与关闭器，按原“文档目录assets→笔记本assets→全局assets”顺序选目录，不再依赖全局getAssetsDir/util.DataDir；候选是链接、非目录或权限失败时返回错误。附件使用原文件名过滤与新ID命名，O_EXCL仅新建，流式写入、同步和输入/输出关闭错误合并；校验源与输出身份后才交付文件名并登记本次去重表，保留原文件权限及文件/目录修改时间。普通资源、图片、链接和目录附件共用该复制端口，目录附件保留相对文件名、隐藏子项与空目录，已复制同一来源继续复用原映射。资产root与源root关闭成功后才允许本次同步完成，ZIP仍等待其外层上传/提取清理。

修正原相对Markdown链接分类误读上层绝对路径的问题：仅本次源目录内实际assets路径段下的.md/.markdown按资源处理，上层目录名含assets不再把普通文档链接变成附件，原页面/标题链接转换保留。图片/HTML宽度/链接/页面树原业务继续复用，未替换前端UI，也未把知识源改成Markdown。

失败边界：普通文件复制失败会按本次创建文件的身份尝试清理，关闭/清理多重错误均返回，遇到碰撞或外来替换不会覆盖或删除该文件。输出为顺序写入，不保证流式文件在完成前不可见，也没有跨文件一致性快照/跨进程CAS/多文件回滚。目录附件在后续子项失败时可能保留已复制子项和新目录，不递归删除以免误删外来新增内容；同一次导入此前成功复制的资源也可能留下，失败不执行本次同步完成。

剩余：SVG/base64转换仍调用旧实现（含共享base64临时目录、绝对路径写入和仅日志错误），虽然其资产目录已由本次root选择，但不能认定所有资产写入已上下文化；下一步串行接入这两类转换。原文档writer返回值、目标源写root/固定SQL队列/sort与其余UI通知，以及其他writer/属性/缓存/广播/私有handler/闪卡map/行历史、文件关闭、历史/快照修复权限和后台完整退出切换继续待办。Native正式多workspace路由未启用，R02总体doing、R03–R10 todo，既有17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：图片生成与HTML转换Context（2026-10-31）

本批完成原SVG/base64图片转换后端的Context接入。移除processBase64Img/processHTMLBlockSvgImg及共享temp/base64暂存、旧绝对路径复制/写入；普通本地与ZIP Markdown使用已有捕获资产root，HTML转换捕获同类owner/data root并持有到转换/渲染结束。HTML没有外部源目录，不伪造Markdown源路径；资产发布复用O_EXCL、逐块归属/取消检查、输入/输出关闭和按本次文件身份清理的原Context端口。

保留原PNG解码/编码、JPEG质量100、SVG data URI原字节、大小写不敏感%0A替换、alt命名/过滤/新ID、图片宽高等属性；受支持类型解码/编码或发布失败直接返回，成功发布后才替换节点链接。修正TextMark a节点误写Tokens的问题，改写实际href并保留文字。已解码但不支持的MIME保持原URI不发布；原PRE中SVG首个匹配提取、base64 SVG内联展开、表格文本管线/数学标识和自定义renderer规则保持。原SVG正则不匹配时仍保留原块，不新增XML标准化或SVG消毒策略。

HTML2Markdown保留旧两参数形式并接受可选Context；新增错误返回HTML2TreeWithContext，仓内粘贴和剪藏转换分支传Request.Context并检查错误，不再忽略fallback错误。HTML2Tree旧两返回值包装保留供兼容，失败记录日志/返回nil，仓内业务已改用错误版本，包装本身不再保留旧全局IO。HTML粘贴转换失败仍保留Failed to convert的data，但code改为-1并附实际错误；剪藏转换失败同样返回失败。公开请求字段及成功返回/原UI不变。

边界：解码器和HTML parser内部的同步CPU调用不能强行中断，在调用前后及编码writer/publish阶段检查取消/owner。PNG/JPEG先编码到调用内内存buffer再发布，不创建共享临时文件；生成器失败无输出节点/名称，资源关闭失败不算模型成功，但此前成功生成的其他资产或后续失败前已写文档可能保留，没有多文件回滚、原子整批或持久receipt。转换依赖合法的当前可写兼容owner；Native正式多workspace写路由尚未启用。

剩余：原文档writer错误返回/目标源root、固定SQL队列/sort/其余通知；HTML控制器的util.NewLute构造及粘贴后续本地文件复制、剪藏multipart上传/远程抓取/其他外层全局业务尚待独立接入，不能将本批图片转换等同整个粘贴/剪藏流程上下文化完成。其他writer/属性/缓存/广播/私有handler/闪卡map/行历史、文件关闭、历史/快照修复权限与后台完整退出切换继续单agent串行推进。R02总体doing、R03–R10 todo，既有17设置/72事务/AV60/60/49订阅/42业务体统计不变，知识源仍docj。

### R02：Markdown源写与索引完成屏障（2026-10-31）

本批接入普通本地与ZIP Markdown导入的实际文档writer、块树登记和固定SQL主队列。原indexWriteTreeIndexQueue在写源前登记块树、调用者又忽略其错误，现改为publishDocuments：每批捕获data root、目标notebook目录身份、原Markdown渲染设置/单行保存/大文件阈值及语言，复用filesys.TreeContext原JSON、空段落、Spec/type规范化和Source.Rewrite；实际写入返回值逐级返回，源未写成不登记块树或主索引。

目标与ID路径校验、source链接拒绝、data/notebook根替换和owner/只读校验均在真实writer端执行，保留原目标路径文件锁。导入分配的是新文档ID，已有源文件/索引ID碰撞返回错误，不再静默覆盖无关文档。JSON编码与缓存更新使用捕获的配置和原commit IAL发布器，不再经全局filesys.WriteTree或晚查当前缓存；大文件提示使用捕获阈值/语言。空文档复用原补空段落和写后Upsert hook，完整树投影只登记一次；新增RepairRegistry.Index捕获DB并沿用原块树锁，登记失败可见。

主队列使用捕获main DB的LegacyMainQueue.Index，关闭writer前Flush已准入前缀；后续文档失败或请求取消也使用WithoutCancel排空已准入工作，再关闭源root，错误合并。writer close幂等，不重刷后来的队列。原文件级Rewrite允许单次已准入源发布在临界区结束后才响应取消，但不再准入下一篇；普通/ZIP外层仍在所有writer/资产/源及上传提取资源关闭成功后才计划本次同步。原文档标题/层级/引用/图片与.docj格式保持。

边界：源与派生索引为顺序发布，没有跨文件回滚、持久receipt或跨进程CAS。索引/缓存/关闭失败可能发生在源文件已落盘之后，返回错误、不计划本次同步；此前成功前缀或资源也可能保留，需要原索引重建/恢复流程处理。未把准备期目标检查升级为跨阶段版本锁；既有Source.Rewrite的临时清理细节与其他历史/修复文件关闭仍需后续审计。本批只推进此导入链，未授权Native正式多workspace写入。

剩余：原box.setSort仍有全局路径、仅日志错误和即时排序通知，需下一步接捕获root/版本化持久化和关闭后owner通知；其他通知/原控制器依赖也未据此全部迁移。HTML parser构造、粘贴后续本地复制与剪藏multipart/远程抓取/外层业务，以及其余writer/属性/缓存/广播/私有handler/闪卡map/行历史、其他文件关闭、历史/快照独立修复权限与后台完整退出切换继续单agent串行。R02总体doing、R03–R10 todo，既有17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：Markdown排序持久化与完成通知（2026-10-31）

本批完成普通本地/ZIP Markdown导入的排序文件与排序通知Context接入。保留原hPath字典序、父路径分组、重复路径去重及组内rank生成算法，仅替换box.setSort调用。使用已有捕获资产data root读取notebook/.siyuan/sort.json，保留未涉及的既有项并合并本批rank；缺少sort文件仍跳过、不创建。JSON/null/结构/权限/链接、查询及写入错误均返回，不再仅日志后算成功；更新沿用原目标文件锁和Source.Rewrite的before字节校验/同目录发布，并检查notebook目录身份。

排序通知在写前由捕获blocktree查询准备，保存原filetreeSortChanged所需parentPath/childIDs和rank比较结果，派发时不再通过全局treenode.GetBlockTree查当前页面。保持原消息字段与相同rank的未指定顺序，不在本批重设分组/通知业务规则。排序成功只产生调用内pending completion，不立即广播；候选树/链接清理不会提前丢掉该pending，普通导入等待writer/索引/资产/源root关闭，ZIP还等待上传/提取目录清理，之后才走捕获owner的completeUI通知与同步准入。

所有失败退出会丢弃本批pending；完成函数无论成功或失败都清理pending，ZIP整体退出也清理，避免下次导入复用旧通知。completeUI继续深拷贝通知字节，准入前取消/owner变化拒绝；准入后通知由owner任务组/关闭屏障管理，关闭会取消并等待未发任务，不把已成功操作的通知生命周期绑定到HTTP返回后的客户端断开。仅新增filetree-sort完成事件到原dispatcher，实际广播为保存的payload，不增加前端协议或全局块树回查。

边界：文档/索引和排序仍顺序发布；排序失败或后续关闭/清理失败时，之前源文件、索引或已写排序可能保留，返回错误但不计划本次同步、不发送本批排序通知，无多文件回滚、持久receipt或跨进程CAS。共享Source.Rewrite的临时清理细节继续后续审计。其他调用者的box.setSort/pushFiletreeSortChanged仍为旧兼容路径，本批不认定所有排序入口完成；其他进度/大文件提示与通知业务也未因此全部迁移。

剩余：HTML parser构造、粘贴后续本地文件复制、剪藏multipart上传/远程抓取/外层全局依赖，以及其他writer/属性/缓存/广播/私有handler/闪卡map/行历史、文件关闭、历史/快照独立修复权限和后台完整退出切换继续单agent串行。R02总体doing、R03–R10 todo，Native正式多workspace写入未启用，既有17设置/72事务/AV60/60/49订阅/42业务体统计不变，知识源仍docj。

### R02：HTML粘贴完整转换Context（2026-10-31）

本批完成html2BlockDOM粘贴入口的原parser配置、容器策略、后续本地资源复制和最终DOM渲染Context接入。入口只负责参数/返回，模型在同一次owner设置快照中捕获Markdown与System.Container，使用原nativeparser.NewLute及HTMLTag2TextMark/HTML2MarkdownAttrs配置；同一捕获资产root覆盖HTML图片转换、Markdown中间格式、二次解析、原段落/列表/单格表格规则、资源复制、TextMark/嵌套行内处理与Protyle渲染，关闭成功后才返回DOM。不再晚查util.NewLute、Conf.System.Container或util.DataDir。

保留原file://、绝对路径、Windows前缀及URL解码规则；相对/网络/缺失来源、无法解析的URL和非std容器按原规则保留链接。每个显式本地来源借用自己的文件/目录root，检查链接、文件类型与身份，复用已上下文化的流式资产复制和关闭错误链；权限/读取/写入/关闭失败返回失败且不返回部分DOM。复制仍为每次引用分配新后缀，即使原名称已含ID也不复用；原图片字节、文件权限/时间、目录子项与空目录保留。共享复制端口增加可选可信名称，未提供时Markdown导入沿用原命名规则，未更改知识源docj格式。

成功/失败envelope保持前批约定（失败code=-1/data=Failed to convert），原前端无需改参数。解码器/parser/渲染器单次同步CPU调用只在调用前后检查取消，逐节点和复制writer继续检查owner/取消；发生后续失败时已生成资产可能留下，无多文件回滚/原子整批或持久receipt。转换仍依赖合法的当前可写兼容owner，未启用Native正式多workspace写路由。

剩余：剪藏extensionCopy的parser构造、multipart上传/读取/关闭、远程抓取和其他外层全局业务尚待接入，不能将本批粘贴入口完成等同所有HTML/剪藏入口完成。其他排序/通知调用者、writer/属性/缓存/广播/私有handler/闪卡map/行历史、共享Source.Rewrite临时清理、其他文件关闭、历史/快照修复权限和后台完整退出切换继续串行。R02总体doing、R03–R10 todo，既有17设置/72事务/AV60/60/49订阅/42业务体统计不变。

### R02：剪藏请求与资源生命周期（2026-10-31）

本批完成extensionCopy剪藏入口的parser、multipart上传、资产发布、远程抓取和返回Context接入，接续上一批HTML粘贴。入口捕获原owner/Markdown配置和语言服务后即借用目标data root，再读取表单；同一捕获root持有至原HTML/Markdown后处理完成。逐次读取检查取消/归属，上传Open/Read/Close、目标写入/同步/关闭、目标root关闭及已解析multipart表单RemoveAll错误汇合；成功结果只在关闭与表单清理后交付，失败code=-1且data为空。HTTP层仍拥有请求Body本身的关闭责任，模型只恢复其包装并清理表单临时文件；解析失败时标准库负责其尚未交付的multipart临时文件。

保留Gin原默认32MiB表单内存阈值、dom/notebook/href/clipType字段、现有笔记本assets目录优先/不存在或普通文件时全局assets回退、原URL解码与二次解码、%u兼容、文件名过滤/新ID、MIME/SVG判断和imageView2链接映射；无效URL跳过，缺dom/坏表单明确失败，notebook ID和链接目录校验阻止越界。原上传MIME判断仍需整文件缓冲，未宣称定额内存流式转码。HTML图片转换仍按原规则使用全局assets，上传文件按所选notebook/global目录，未改变相对assets返回。

保留两站点文章raw URL映射、part剪藏绕过远程、缩进代码围栏转换/数学识别、iframe去换行、段首空白/图片alt与title纯文本/TextMark及行内展开与Markdown输出。远程请求使用请求Context和原30秒cloud客户端，关闭自动响应预读后通过受控读取关闭body；普通无body网络失败沿用HTML回退，取消/归属撤销以及响应读/关闭失败不伪装成功。修复req取消时非nil包装但内部HTTP响应nil导致的空指针访问；即使fetch错误带body，也检查关闭并交付失败。不再晚查util.NewLute、util.DataDir或Conf.Language。

归属撤销在外部调用前后和逐块读取检查，原parser/formatter与第三方同步调用不强制抢占；HTTP请求取消传递到网络层。已成功发布的资产可能在后续失败时保留，不承诺多文件回滚、原子整批或持久receipt。仍是合法可写兼容owner，不等同Native正式多workspace路由。R02总体doing，R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体既有统计不变。剩余共享Source.Rewrite临时清理、其他排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史、其他关闭错误及历史/快照修复权限和完整后台退出切换继续串行。

### R02：共享源写入的临时归属和错误收尾（2026-10-31）

本批完成共享indexrepair.Source.Rewrite的失败临时清理、父目录归属、发布前身份/版本与授权检查，接续上批剪藏之后列明的待办。公开Source字段和Rewrite签名不变，临时文件仍为同目录.wiki-repair-随机后缀、独占创建，保留目标原权限、写入→Sync→Close→原子rename以及nil-before只允许新建/非nil-before要求原字节的业务。新增短写检查，文件Write/Sync/Close、失败清理和借用父root关闭错误汇合返回，释放原文件锁后不再有本次资源操作。

父目录通过已打开的root保留到收尾，初次及发布前核对可见父目录身份；父路径被挪走/替换时拒绝发布，清理仍针对已捕获父目录的本次文件，不使用替换后的父路径删除。临时文件用打开描述符Stat记录身份，发布/清理前要求相同普通文件；被其他文件/链接/目录替换则返回归属变化而不删除该替换物。成功rename后不再删除原临时名称，避免误删后来复用该名称的文件；失败清理时正常不存在（含检查之后由别处移除）不新增错误。无法取得初始文件身份或清理权限失败时保留无法安全清理的临时文件并返回错误，不隐瞒为清理成功。

沿用临时输出阶段的WithoutCancel收尾策略：进入前和等待文件锁后取消不启动写入，进入后完成本次发布/清理；发布前再次以该critical Context校验owner权限，独立的只读/关闭/归属撤销仍拒绝发布。上层Authorize若同时检查其捕获请求的取消，也会拒绝发布并完成清理，不绕过该更严格策略。发布前对捕获父root再核对原目标字节/新建占用，已观察到的外部版本变化不覆盖。检查与rename/unlink仍有系统调用间隙，不宣称跨进程CAS、任意外部编辑排他、目录fsync掉电持久性或多文件事务；成功rename后的父root关闭失败会返回错误但不会回滚已发布文件。

原docj格式、UI、HTTP字段/返回、CLI参数和依赖/打包路径未改。共享调用者包含原修复树写入、AV绑定、归档准备与导入/导出资源、普通/ZIP Markdown writer和sort，因此用真实调用链回归，不能仅依据底层单测宣称全链完成。R02总体doing、R03–R10 todo，17设置/72事务/AV60/60/49订阅/42业务体统计保持。其他排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史、其余关闭错误与历史/快照修复权限及后台完整退出/切换仍待。

### R02：历史清理和归档的Context生命周期（2026-10-31）

本批完成Source.RemoveLegacyHistory与Archive的捕获目录/文件身份、授权/取消和资源生命周期接入，接续上批Source.Rewrite之后的待办。公开Source字段及两方法签名不变，原兼容notebook/.siyuan/history路径与corrupted/<stamp>-<随机后缀>/<原路径>保留，未改变docj源格式或原索引订正阶段顺序。model/index_fix.go仍在resetDuplicateBlocksOnFileSys内先清历史，再归档非法文档文件名；其他业务、UI与HTTP/CLI字段未重写。

历史清理先做完整只读清单，拒绝链接、特殊类型、坏目录和未完成扫描；已存在的history普通文件作为未知对象保留并返回错误，不按目录清理。父目录与history借用root保持到收尾，子目录按递归深度借用，检查可见目录和打开目录的身份。清单内存随条目数增长；删除时再次枚举/核对已扫描项，逐文件/空目录检查授权、取消、身份、普通文件size/mtime，只使用Remove，不对原路径盲目RemoveAll。新增/更新/替换条目、目录被挪走/链接替换或权限撤销都会停止；后来加入的未扫描文件不会被递归带走。初始缺失仍成功；执行中取消或错误可能已删除前缀，明确返回错误，不宣称目录删除原子性或回滚。

损坏源归档捕获源父root和普通文件身份，受控读取并检查Read/Close，再用既有Rewrite发布独占备份，保留完整备份路径逐级目录Sync/Close。删除源之前验证备份父目录/同步过的各级目录、备份身份和原字节，并重新读取源、核对相同文件身份与原字节；同字节但不同inode的替换源也保留，源或备份被改动/替换不继续删除。最终删除通过捕获源父root，删除前再检查owner权限。进入备份发布前遵循请求取消，发布成功后的同步/核验/删源保留原WithoutCancel收尾；上层Authorize更严格的捕获请求取消/只读/归属撤销不绕过。

源/备份文件、目录Sync和借用root关闭错误逐级汇合，备份未完成或验证失败保留源。Archive一旦选择输出位置，错误时也返回候选dest供检查；候选不证明文件已存在/完整，成功仍返回原相对路径。既有model调用者只消费error，公共HTTP返回未变。晚期父root关闭错误可能发生在已归档并删除源之后，返回dest与error、不隐瞒已发布状态，也不回滚。检测与rename/unlink之间仍有系统调用间隙，不宣称跨进程CAS、任意外部写入排他或多文件事务。

R02总体doing、R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。本批不等同所有历史/快照业务完成；共享Source.Remove/Rename其余调用者、其他排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史、其余关闭错误与后台完整退出/切换继续串行。

### R02：Source.Remove/Rename归属及调用者（2026-10-31）

本批完成共享Source.Remove/Rename及原索引修复、导入scratch改名、导出sort和闪卡导出清理调用链核对。保留公开Source字段、方法签名和原路径/文档/目录业务；动作前捕获源条目及父目录身份，锁后及动作前复核授权、取消、源身份/普通文件size与mtime，Rename复核目标不存在。借用父root关闭错误与动作错误汇合；动作已完成后的检查/关闭失败明确返回错误，不回滚已删除或移动的文件。

原recreateTree外层锁保留，最后删除不重复加同一锁；闪卡导出根关闭错误已接入。公开HTTP/CLI、原docj格式、依赖及打包路径不变，完整原前端和受控浏览器回归见执行计划。

R02总体doing、R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为flashcardArchiveStorage.source/Read/Write仍为void的借用root关闭回调和版本登记收尾；本批完成导出根关闭，不等同长期闪卡storage关闭已完成。其他排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史、其余历史/快照与后台完整退出/切换仍待。

### R02：闪卡storage借用root及版本登记（2026-10-31）

本批完成flashcardArchiveStorage.source/Read/Write借用root检查型关闭和版本登记。source返回func()error，复用已验证的幂等关闭组件，借用前后检查捕获owner；关闭后端在借用时固定，不晚读可变字段。Read/Write取得原每文件锁后重新准入，锁持有到root关闭及版本交付；Write仅在本次Source副本上取消内层重复Lock，Authorize仍保留，未改变原路径、MessagePack、牌组/日志与缓存生命周期业务。

关闭/收尾失败不推进版本；失败写若已改变磁盘字节，重试需先重读。纯文件缺失的原缺省保持，缺失和关闭等复合错误不再被加载器忽略；缓存牌组跨请求及索引替换后的保存保持。公开HTTP/CLI、原格式和打包路径不变，完整验证见执行计划。

R02总体doing、R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为共享Source.Read的文件描述符关闭、读取取消/身份及调用者缺失错误契约；其他排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史、其余历史/快照和后台完整退出/切换继续串行。

### R02：只读Source的受控读取与缺失契约（2026-10-31）

本批完成共享indexrepair.Source.Read的文件描述符关闭、分块取消/身份和直接调用者缺失分类。公开Source字段与Read签名不变，读取不调用Authorize或取得writer锁，只读Source仍可使用，避免Rewrite内部/原调用者已持锁时重复加锁。替换os.Root.ReadFile为受控Open/Stat/Read/Close；捕获源文件与父目录身份，在已打开父root内借用文件，核对文件描述符/可见路径/父目录、普通文件类型和size/mtime，并在64KiB分块前后、文件及父root关闭之后检查取消和已观察到的身份变化。

关闭/读取/取消失败不返回部分bytes，初始纯缺失与不完整读取分开，缺失分支已同步共享分类。已有空文件和原读取权限边界保留，完整验证见执行计划。

R02总体doing、R03–R10 todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。其余独立文件/root关闭、历史索引、排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史及后台完整退出/切换继续串行。

### R02：闪卡解包读取与借用root生命周期（2026-11-01）

本批完成ImportSY解包闪卡读取的Context接入：生产路径改用已捕获ArchiveImportContext.openScratch与owner guard，读取期间、交付前及借用root关闭后复核取消和owner。先捕获storage/riff目录与.deck/.cards两份候选的存在状态、inode、类型、size/mtime，再读取原始字节；任何读取、文件/父root/外层root关闭或最终检查失败均不返回候选map，原错误原因保留。

共享ReadLimited保留每文件32MiB上限、原始字节与空缺省；全部关闭/guard成功后才交付。此处不保证整个ImportSY回滚。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为historyIndexContext.index的独立root关闭与Plan/查询生命周期，其余排序、通知、缓存、writer、属性、广播、私有handler、闪卡map、行历史及后台退出/切换继续串行。

### R02：历史索引候选与读取root生命周期（2026-11-01）

本批完成historyIndexContext.index的独立读取root关闭与队列准入生命周期：保留原historyindex.Plan只读解析和历史字段投影，将候选扫描独立成scan；扫描/借用root关闭/最终Context与owner核验全部成功后，才调用原ReplaceDirectory。关闭失败不再被忽略，扫描与关闭双错误保留；扫描panic先转为失败再执行检查型关闭，不交付候选或发送进度。

保留初始纯缺失、原只读历史投影与已接收队列实际Flush后通知；通知失败不回滚已提交SQL。根/目录身份核验不是完整目录快照。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为historyindex.Plan的目录枚举资源、完整候选扫描身份和取消核验；其他adapter、排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史及后台完整退出/切换继续串行。

### R02：历史候选扫描的资源和变化检查（2026-11-01）

本批完成historyindex.Plan内部目录枚举资源与候选扫描核验。用受控Open/Stat/分批ReadDir/Close替换fs.WalkDir对os.Root.FS的隐式枚举；每批64项，严格区分正常EOF与复合错误，零进展返回错误。目录打开描述符须匹配已捕获身份，读前后及关闭后检查Context、根/路径身份与元数据，读取和关闭错误汇合，失败不交付部分entries。只借用外层Source.Data，内部目录描述符各关闭一次，不申请Authorize或writer锁。

候选交付前重枚举比较子项，并复核所有节点元数据；保留原顺序、只读格式与字段。此观察清单不是跨进程原子快照。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为ListNotebooksContext的独立root关闭与列表/修复生命周期；其他独立读取和目录枚举adapter、排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史及后台完整退出/切换继续串行。

### R02：笔记本列表的owner与资源生命周期（2026-11-01）

本批完成ListNotebooksContext外层root关闭与列表/修复调用生命周期：保留原notebooks.Scan/Apply和列表字段，捕获AppConf、data、workspace、主SQL及blocktree依赖。预取消、空/关闭owner与无效根不进入扫描，打开后比较根描述符身份，扫描/修复前后及root关闭后核对Context、owner、DB选择器和所选绝对路径身份。root检查型幂等关闭，列表错误、panic及关闭错误汇合，失败不返回部分或成功列表；关闭后的检查不再使用已关闭root。

保留原只读/修复权限和settingsGate→repairGate顺序，回调复用捕获的依赖，避免重入；后续关闭失败不回滚已执行修复。内部notebooks读取/枚举关闭仍待。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为internal/notebooks的有界元数据读取、Scan枚举及Apply缺失分类/关闭；其他独立读取/枚举adapter、排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史及后台完整退出/切换继续串行。

### R02：Notebook候选与修复的IO错误准入（2026-11-01）

本批完成internal/notebooks的有界元数据读取、Scan枚举关闭和Apply错误准入。原read复用Source.ReadLimited，保留1MiB每文件上限，接入共享文件/父root关闭、逐块取消、路径/描述符身份与长度检查；路径或超限错误仍可识别ErrPath，底层原因保留。Scan/Apply使用IsSourceNotExist，仅真正初始纯缺失才能进入缺省/补建分支，已选定读取不完整或复合关闭错误不会触发补建或损坏清理。

目录枚举显式关闭，错误不交付Plan；修复读/写/关闭失败保留原原因，完整保存后才afterCreate。源部分副作用不回滚。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为云附件uploadCloudAssets/cloudUploadFile的root与延迟文件读取生命周期；其他独立读取/目录枚举adapter、排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史及后台完整退出/切换继续串行。

### R02：云上传资源和延迟reader生命周期（2026-11-01）

本批完成uploadCloudAssets外层root与cloudUploadFile延迟读取生命周期。捕获请求Context、AppConf/data/workspace、根路径身份并在打开前后及检查型关闭后核验，关闭的账户或workspace owner不准继续。外层root幂等关闭，所有失败保留已确认上传计数和原错误；最终检查不使用已关闭root。

延迟文件读取前后和关闭后检查身份/取消，超限仍不打开payload；失败不发起该文件上传，已完成的远端部分效果不回滚。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为archiveExportContext.documentPaths的独立目录枚举关闭与候选交付；其他读取/目录枚举adapter、排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史及后台完整退出/切换继续串行。

### R02：只读ReadDir与归档路径候选交付（2026-11-01）

本批完成archiveExportContext.documentPaths的检查型目录枚举与候选交付，并新增共享indexrepair.Source.ReadDir。Source字段和Read接口不变，ReadDir仅借用外层root、不取Authorize或writer锁；捕获选中目录/借用root/读取描述符身份，128项分批枚举并按原字典序返回。读前后、文件与借用root关闭后检查Context、目录路径及inode/size/mtime；读取、Stat、两层关闭、混合EOF/缺失和无进展错误汇合，不返回部分entries。初始纯缺失保留原错误，选定读取后的失败带原ErrSourceReadIncomplete，不得当成可选空目录。

归档路径候选在全部关闭与最终guard成功后交付；保留原顺序/过滤/源tmp，不宣称全仓原子快照。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为ArchiveImportContext.walkScratch及其AV候选/目录改名/storage预检调用者；其他独立读取/枚举、排序/通知/缓存/writer/属性/广播/私有handler/闪卡map/行历史及后台完整退出/切换继续串行。

### R02：ImportSY scratch受检遍历（2026-11-01）

本批完成ArchiveImportContext.walkScratch的受检目录枚举。原根访问、回调、字典序深度优先、目录与文件上的fs.SkipDir及fs.SkipAll语义保持；链接仍在业务回调前拒绝。每层目录改用共享Source.ReadDir，继承选中目录/描述符身份、128项分批读取及文件和借用root检查型关闭；枚举前后复核请求Context和捕获owner。初始真正缺失仍保留原错误，已选目录读取/关闭失败带ErrSourceReadIncomplete并停止遍历。

三个调用者的原准入边界已核对并通过故障回归：remapScratchAVs完成全部AV候选收集后才生成ID、改写和改名；renameScratchDirectories完成全部扫描后仍按最深目录优先改名；removeScratchStorage完成整个storage树校验后才RemoveAll。模拟后续目录读取/关闭不完整时，AV原字节、原目录和storage均保留，不把复合缺失当成无数据。外层openScratch的私有Source、Authorize和幂等检查型关闭不变。

这仍是顺序业务：一旦后续写/改名开始，晚期失败可能保留成功前缀；未增加整个ImportSY回滚、原子目录快照、内容哈希或跨进程CAS。prepare文档候选、mountArchive、mergeArchiveDirectory和资源发布等独立fs.WalkDir仍未迁移，不能据此机械标完成。公开UI、HTTP/CLI、docj格式、依赖和打包布局未改。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为ArchiveImportContext.prepare的docj候选扫描，复用本批受检walk后再进入文档读取/ID映射；mount/merge/资源发布遍历继续独立审查。完整验证与证据见AnonyPower执行计划的`wikiman-r02-import-scratch-walk/results.json`。

### R02：ImportSY prepare文档候选清单（2026-11-01）

本批将ArchiveImportContext.prepare的docj候选扫描接入walkScratch受检遍历。prepare局部清单捕获所有所经目录与docj普通文件的类型、inode、size/mtime；完整扫描后、每个候选读取前后及最终交付前分别复核，任何枚举、读取、关闭、取消、owner或身份变化错误都不返回部分prepared结果。

原字典序、docj筛选、文档解析、路径和ID映射保持；测试覆盖后续目录不完整、扫描中替换、读取后替换与第二文档部分读取，确认scratch清理且原输入ZIP不变。该清单不泛化为mount/merge/资源发布契约，也不是内容哈希、CAS、原子目录快照或整批回滚。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为mountArchive文档候选扫描；mergeArchiveDirectory和资源发布继续独立待办。验证见AnonyPower执行计划的`wikiman-r02-import-prepare-scan/results.json`。

### R02：ImportSY mount文档候选清单（2026-11-01）

本批将mountArchive的文档路径收集接入walkScratch，并增加mount局部元数据清单。扫描记录目录和docj普通文件身份，保留隐藏目录SkipDir、字典序、根/嵌套路径；merge前后复核全部候选，之后才进入原Load、registry Upsert和LegacyMainQueue索引。

后续目录不完整或扫描中候选替换不进入merge；merge后候选替换不进入索引，但已发生的顺序复制不回滚。既有晚关闭错误在索引准入后仍等待Flush并作为失败返回。该边界不泛化到mergeArchiveDirectory或资源发布，也不是内容哈希、CAS或原子整批。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为mergeArchiveDirectory的受检源枚举与目标发布准入；资源发布遍历继续独立待办。验证见AnonyPower执行计划的`wikiman-r02-import-mount-scan/results.json`。

### R02：ImportSY目录合并清单与顺序发布（2026-11-01）

本批将mergeArchiveDirectory改为两阶段执行：先用walkScratch完整捕获全部目录和普通文件并复核清单，成功后才打开目标写root；再按原字典序逐项复核源、创建并检查目录、受控读源、读目标并通过Rewrite CAS发布。最终再次复核源清单，所有关闭错误继续汇合。

枚举/扫描身份失败时目标零写入；发布开始后的后续源读取、目标冲突或最终复核失败仍保留成功前缀，不改变既有顺序合并契约。没有全量预读、整树临时切换、回滚或持久receipt。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为publishArchiveResources资源目录发现和成功合并后删除准入；emoji清洗递归枚举继续独立待办。验证见AnonyPower执行计划的`wikiman-r02-import-merge-directory/results.json`。

### R02：ImportSY资源发现与scratch删除准入（2026-11-01）

本批将publishArchiveResources的资源目录发现接入walkScratch。局部清单记录所有遍历目录及精确assets/emojis候选，保持字典序与SkipDir；完整发现复核成功后才逐候选merge，merge和候选目录复核成功后才RemoveAll。

枚举/扫描失败不merge不删；merge、owner或候选替换失败保留scratch；删除错误继续返回，已合并目标不回滚。目录身份不是子树内容哈希，merge到删除仍有间隙，RemoveAll也可能部分生效。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为sanitizeArchiveEmojis递归受检枚举与改名准入。验证见AnonyPower执行计划的`wikiman-r02-import-resource-discovery/results.json`。

### R02：ImportSY emoji逐层清洗计划（2026-11-01）

sanitizeArchiveEmojis现在逐层使用Source.ReadDir受检枚举。每层完整捕获直接子项身份、计算清洗目标并预检重复目标与既有目标，计划成功后才按原字典序逐项复核、改名和递归；目录递归使用清洗后的路径。

当前层读取/关闭、链接/类型、身份或碰撞失败时该层零改名；执行阶段后续改名或深层失败保留先前成功项。没有整棵临时树、回滚、内容哈希或跨进程CAS。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为extractArchive根目录ReadDir和单顶层目录选择准入。验证见AnonyPower执行计划的`wikiman-r02-import-emoji-sanitize/results.json`。

### R02：ImportSY解压根目录受检选择（2026-11-01）

extractArchive现在通过Source.ReadDir完整读取owned scratch根，再判断唯一顶层目录。选中目录捕获类型、inode、size/mtime并在返回前复核；读取/关闭不完整或目录替换时释放scratch且不返回可消费根。

原单顶层格式、data备份区分、显示根名称及prepare后续扫描不变。该检查不构成子树内容哈希、跨进程CAS或整包事务。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。ImportSY直接目录枚举迁移完成，下一项为Markdown导入源fs.WalkDir受检迁移。验证见AnonyPower执行计划的`wikiman-r02-import-extract-root/results.json`。

### R02：Markdown本地源受检扫描（2026-11-02）

markdownImportSource现在复用walkScratchWithIO建立完整本地源清单。每层Source.ReadDir成功后捕获目录/普通文件身份，扫描完成再复核全部条目；读取不完整或扫描期替换不交付部分源对象，失败root关闭一次。

原目录与单文件入口、根名称、隐藏项、字典序和后续扩展过滤保持。身份清单不构成内容哈希、跨进程CAS或导入事务。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为单文件导入附件目录复制的直接fs.WalkDir受检迁移。验证见AnonyPower执行计划的`wikiman-r02-markdown-source-walk/results.json`。

### R02：Markdown单文件附件目录受检扫描（2026-11-02）

markdownImportAssets.copyDirectory的单文件分支现在复用walkScratchWithIO。每层Source.ReadDir完成受检枚举，回调捕获目录/普通文件身份；完整扫描和整份清单复核通过后才创建目标目录并按原字典序发布。

后续目录读取不完整或扫描中替换早期文件时目标目录保持不存在。空目录、原相对名、权限/mtime及晚失败保留成功前缀的契约不变；没有整树回滚、内容哈希或跨进程CAS。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。Markdown导入生产代码直接目录枚举已清零，下一项为归档导出资源枚举准入。验证见AnonyPower执行计划的`wikiman-r02-markdown-asset-walk/results.json`。

### R02：归档导出资源受检清单（2026-11-02）

archiveExportContext现在以Source.ReadDir建立资源索引清单和选中目录复制清单。所有笔记本/全局assets身份复核通过后才交付索引；选中目录完整复核后才创建目标，并保持字典序复制、空目录和目录mtime。

索引或复制阶段读取不完整、扫描期替换均在目标目录创建前失败。原全局资源优先、PDF注解、封面/emoji、隐藏目录内容、raw AV循环和相关资源策略保持；晚复制失败仍保留成功前缀，没有递归回滚、内容哈希或跨进程CAS。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为writeOwnedArchive归档输出枚举准入。验证见AnonyPower执行计划的`wikiman-r02-archive-export-resource-enumeration/results.json`。

### R02：归档输出受检成员清单（2026-11-02）

writeOwnedArchive现在先用Source.ReadDir建立完整字典序清单，捕获根、目录与普通文件身份及空目录状态。整份复核通过后才创建ZIP成员；写入期间逐项复核，结束前再次复核全部条目。

原可选根名、Deflate、文件权限/mtime、只输出空目录头和成员顺序保持。扫描失败时目标writer零字节；晚复制/关闭失败可留下私有archive.part前缀，但不会发布就绪归档。没有正文预读、内容哈希、跨进程CAS或通用writer回滚。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为Markdown导出包资源枚举准入。验证见AnonyPower执行计划的`wikiman-r02-archive-output-walk/results.json`。

### R02：Markdown导出包资源枚举准入（2026-11-02）

markdownExportResources.hasChildren现在通过Source.ReadDir完成单层读取并复核父目录身份；目录资源复制复用归档导出完整清单，全部扫描复核后才创建目标目录。

原全局/笔记本/注解资源优先级、缺失跳过、隐藏/空目录、字典序和目录mtime保持。子项读取失败不返回false成功，复制预检失败不创建目标；晚复制失败仍保留成功前缀，不做整包回滚。

R02总体仍doing，R03–R10仍todo；17设置/72事务/AV60/60/49订阅/42业务体统计不变。下一项为Markdown PDF注解源目录枚举准入。验证见AnonyPower执行计划的`wikiman-r02-markdown-export-package-enumeration/results.json`。

### R02：Markdown注解资源枚举准入（2026-11-02）

findAnnotationAsset现在只通过Source.ReadDir读取实际访问的目录，并在回调返回后复核父目录类型、inode、size/mtime。部分读取不作为缺失处理，父目录替换不接受旧目录项。

仍保留全局优先、笔记本字典序递归和首个后缀匹配短路；命中后不预扫无关后代。`.sya`读取、缓存、失败不发布和源只读语义保持。

R02总体仍doing，R03–R10仍todo；下一项为HTML导出资源根枚举准入。验证见AnonyPower执行计划的`wikiman-r02-markdown-export-annotation-enumeration/results.json`。

### R02：HTML设备资源根枚举准入（2026-11-02）

copyHTMLDeviceResource现在先通过Source.ReadDir完整枚举所选根，并复核外部根路径在读取前后的身份；成功后才创建目标根。枚举失败不留下空目标，根替换不接受旧目录项。

顶层子项仍逐项调用copyPath，目录后代继续使用既有清单；主题/图标/emoji、隐藏/空目录、mtime、可选资源和晚失败前缀语义不变。

R02总体仍doing，R03–R10仍todo；下一项为HTML树递归枚举准入。验证见AnonyPower执行计划的`wikiman-r02-html-resource-root-enumeration/results.json`。

### R02：HTML合并树按需枚举准入（2026-11-02）

mergeServices的List/Children现在只通过Source.ReadDir读取实际访问目录，并在回调返回后复核父目录身份。不完整读取不作为空列表，目录替换不接受旧目录项，失败不交付部分文件投影。

doctree仍控制按需递归；reserved/`.tmp`、隐藏项、排序、mergeMax、引用计数和源只读语义保持。未访问分支不预扫。

R02总体仍doing，R03–R10仍todo；下一项为HTML desktop目录遍历准入。验证见AnonyPower执行计划的`wikiman-r02-html-tree-enumeration/results.json`。

### R02：HTML桌面导出预检完整枚举准入（2026-11-02）

桌面导出预检现在先以resourceManifest完整读取私有输出根并复核根及全部后代身份，再执行目标index和路径类型碰撞检查。Source.ReadDir的不完整错误保持原因链，已枚举文件被替换返回ErrSourceChanged，两者都发生在copyHTMLDirectory之前。

通用清单允许独立根`.`作为清单起点，但不放宽后代路径、链接和普通文件约束。复制阶段仍保留原第二次顶层读取和顺序副作用边界。

R02总体仍doing，R03–R10仍todo；下一项为html_handoff_context.go的顶层复制枚举准入。验证见AnonyPower执行计划的`wikiman-r02-html-desktop-preflight-enumeration/results.json`。

### R02：HTML目录交付完整枚举准入（2026-11-02）

copyHTMLDirectory现在先用resourceManifest完整扫描私有HTML根，复核独立根及所有后代身份，成功后才按顶层条目调用现有copyPath。不完整读取保留原因链，先前条目替换返回ErrSourceChanged，两者均不产生任何目标条目。

实际复制仍由copyPath逐顶层目录完成，保留空目录、mtime、覆盖检查和晚失败前缀；完整准入与实际复制之间不提供内容哈希或跨进程CAS。

R02总体仍doing，R03–R10仍todo；下一项为conflict_sort_context.go的目录枚举准入。验证见AnonyPower执行计划的`wikiman-r02-html-handoff-enumeration/results.json`。

### R02：冲突排序按需目录枚举准入（2026-11-02）

addConflictSort的doctree目录服务现在按实际请求通过Source.ReadDir完整读取，并在注入回调前后复核父目录身份、逐项验证路径。读取不完整或父目录替换时返回错误，不改写sort.json且不发布排序通知。

该迁移不建立整本笔记本清单，保留MaxListCount、原排序/引用计数、reserved/tmp过滤和按需Children语义。私有readDir/notify仅用于故障验证。

R02总体仍doing，R03–R10仍todo；下一项为index_fix.go的索引订正遍历准入。验证见AnonyPower执行计划的`wikiman-r02-conflict-sort-enumeration/results.json`。

### R02：索引订正完整清单准入（2026-11-02）

resetDuplicateBlocksOnFileSys与fixBlockTreeByFileSys现在先以Source.ReadDir完整扫描单个笔记本，捕获并最终复核根、目录和普通文件身份，成功后才归档非法文档、重写重复块或处理注册表差异。不完整读取和扫描期替换不会产生本阶段副作用。

清单保持隐藏目录跳过、原字典序深度遍历及docj/资源过滤。边界仍是逐笔记本，不承诺全笔记本事务、内容哈希或跨进程CAS。

R02总体仍doing，R03–R10仍todo；下一项为incremental_index_context.go的增量索引遍历准入。验证见AnonyPower执行计划的`wikiman-r02-index-fix-enumeration/results.json`。

### R02：增量索引目录完整清单准入（2026-11-02）

expandIndexPaths现在通过入口私有的Source.ReadDir清单完整扫描目录，并在返回docj路径前复核根及所有后代身份。读取不完整或扫描期间替换早期候选时返回空结果，不进入增量索引；隐藏目录、字典序、过滤与去重保持。

本批只收紧目录展开准入，不移动后续remove/upsert/flush边界，也不宣称内容哈希或跨进程CAS。model生产代码的直接fs.WalkDir/fs.ReadDir调用已清零。

R02总体仍doing，R03–R10仍todo；下一项审查internal/indexrepair/notebook.go及其余R02主线。验证见AnonyPower执行计划的`wikiman-r02-incremental-index-enumeration/results.json`。

### R02：笔记本文档完整清单准入（2026-11-02）

Source.Documents现在通过私有Source.ReadDir清单完整扫描笔记本，最终复核合法目录和docj候选后才向索引调用者交付路径。不完整读取或扫描期替换早期文档返回nil，不启动后续文档加载和索引副作用。

隐藏、assets、`.siyuan`及其他非法ID目录继续整棵跳过，链接继续拒绝，合法文档顺序与过滤保持。本批未改变indexNotebook/indexReferences的消费和完成屏障。

R02总体仍doing，R03–R10仍todo；下一项审查internal/assetcache/cache.go。验证见AnonyPower执行计划的`wikiman-r02-notebook-documents-enumeration/results.json`。

### R02：资产缓存两阶段清单准入（2026-11-02）

Cache.Load现在先通过私有fs.ReadDir清单完成资产树扫描和最终身份/元数据复核，再一次性替换资产快照并清空哈希。读取不完整或扫描期间替换早期文件时，旧资产与哈希继续可用。

通用fs.FS契约、缺失根、10万项预算、隐藏/`.sya`/filelock过滤、链接拒绝、字典序和owner隔离保持；生产os.Root后端额外使用SameFile识别同大小同mtime替换。

R02总体仍doing，R03–R10仍todo；下一项审查internal/sourcetx/project.go。验证见AnonyPower执行计划的`wikiman-r02-asset-cache-enumeration/results.json`。

### R02：sourcetx内容投影全页面清单准入（2026-11-02）

内容重建在任何documents/blocks/refs写入前，先按页面与字典序完成两次一致的data树清单；候选docj按打开文件身份读取，SQL处理后再比较最终清单。枚举失败、同元数据身份替换或扫描期间增删均回滚候选数据库，不发布部分投影。缺失data根、隐藏目录、合法box/docj过滤、链接拒绝和既有预算保持。

R02仍doing，R03–R10仍todo；project.go直接fs.WalkDir已清零，字面未勾选仍44。下一项审查internal/sourcetx/read.go。验证见AnonyPower执行计划的`wikiman-r02-sourcetx-project-enumeration/results.json`。

### R02：sourcetx源读取完整清单准入（2026-11-02）

SourceFiles与DocumentSource底层现共用两次预确认、打开身份复核和读取后最终确认的目录清单。失败不返回部分文件列表或已过期文档。Files保留隐藏目录内容，Document保留隐藏目录跳过；缺失根、链接拒绝、字典序和原预算保持。

R02仍doing，R03–R10仍todo；read.go两处直接fs.WalkDir已清零，字面未勾选仍44。下一项审查internal/sourcetx/transaction.go。验证见AnonyPower执行计划的`wikiman-r02-sourcetx-read-enumeration/results.json`。

### R02：sourcetx事务条目稳定清单准入（2026-11-02）

候选页面图的entryDocument校验现复用sourcetx受检清单。目标box目录的两次完整枚举必须在路径和生产文件身份上一致，随后才叠加事务changes并判断目标docj唯一；枚举失败或扫描期替换不会准入journal、暂存或源发布。

隐藏目录、缺失目录由overlay补足、链接拒绝和唯一性规则保持。事务入口不新增条目预算；Files与Document仍显式保留10万项限制。R02仍doing，字面未勾选仍44；下一项审查internal/sourcetx/recovery.go。验证见AnonyPower执行计划的`wikiman-r02-sourcetx-transaction-enumeration/results.json`。

### R02：sourcetx恢复 journal 单层稳定清单（2026-11-02）

journalIDs现复用共享受检清单的单层模式，对`.local/journal`完成两次路径和生产身份一致的枚举后，才向read admission或Recover交付排序后的operation ID。读取中断、目录身份变化或同元数据inode替换不会漏掉待恢复操作而开放读取门。

临时文件忽略、json/commit配对、20万子项预算、缺失目录、目录/链接拒绝和恢复发布边界保持。R02仍doing，字面未勾选仍44；sourcetx业务直接遍历已清零，下一项审查internal/appearance/owner.go。验证见AnonyPower执行计划的`wikiman-r02-sourcetx-recovery-enumeration/results.json`。

### R02：appearance资源稳定清单与安装预检（2026-11-02）

appearance catalog现对installed/bundled各自完成两次单层os.Root清单和根关闭检查后才合并；bundle安装在目标创建前完成两次递归清单。链接、非普通文件、条目与字节预算在预检中拒绝，不完整枚举或同元数据身份替换不交付catalog，也不开始安装目标写入。

复制窗口在打开前、打开后和复制后核对源文件身份，输入/输出关闭错误返回，完成后再次复核整棵源清单。custom保留、bundle覆盖、隐藏过滤、顺序发布和晚失败成功前缀保持；不提供目录级回滚或跨进程CAS。

R02仍doing，字面未勾选仍44；appearance业务直接遍历已清零，下一项审查internal/workspace/emoji.go。验证见AnonyPower执行计划的`wikiman-r02-appearance-enumeration/results.json`。

### R02：自定义 emoji 稳定清单准入（2026-11-03）

ownedEmojis不再直接fs.ReadDir。emoji专用清单以data os.Root打开根与一层子目录的独立句柄，分批读取、检查关闭，并复核打开目录、路径与条目的类型、大小、mtime和SameFile身份。两次清单一致后才按原顺序构造映射并一次发布到本workspace registry。

不完整读取、同元数据文件替换、目录在打开窗口替换和关闭失败均保留旧registry。隐藏项、一层递归、深层忽略、非法名/链接、扩展名覆盖、缺失目录与10000键限制保持；不读取emoji内容或提供跨进程CAS。

R02仍doing，字面未勾选仍44；emoji.go直接fs.ReadDir已清零，下一项审查internal/workspace/native_avquery.go。验证见AnonyPower执行计划的`wikiman-r02-emoji-enumeration/results.json`。

### R02：Native AV 查询一致清单（2026-11-03）

SearchNativeAttributeViews不再直接fs.ReadDir。`storage/av`根和直接子项完成两次身份一致的分批枚举后，关联索引与候选JSON才按清单读取；候选文件检查打开前后、读取后、路径及关闭错误，名称和views使用同一解析快照，最终清单变化时清空结果。

无关项过滤、100000项/关联预算、16MiB限制、updated排序、12项上限、exclude与缺失空结果保持。该查询不回滚blocktree派生缓存，也不提供跨进程CAS。

R02仍doing，字面未勾选仍44；native_avquery.go直接fs.ReadDir已清零，下一项审查internal/workspace/native_blocktree.go。验证见AnonyPower执行计划的`wikiman-r02-native-avquery-enumeration/results.json`。

### R02：只读投影目录稳定清单（2026-11-03）

`rebuildBlockTree`、文档信息Subfiles和文档树List/Children现共用workspace目录句柄读取器。读取器分批读取、排序并复核目录路径和直接子项身份，传播部分读取与关闭错误；单层调用执行两次一致性确认。块树在此基础上生成两次递归清单，并在解析全部docj后、替换blocktrees前执行最终确认。

块树失败保持旧索引；导航从同一快照投影大小和目录类型。原非法顶层/隐藏目录跳过、链接拒绝、缺失叶文档子目录、reserved/tmp过滤、`.docj`名称计数和预算不变。共享层不吸收调用方策略，也不提供跨进程CAS。

R02仍doing，字面未勾选仍44；native_blocktree.go、native_document_info.go、native_document_tree.go直接枚举已清零。workspace剩余4处直接枚举，下一项为native_document_lifecycle.go。验证见AnonyPower执行计划的`wikiman-r02-workspace-read-enumeration/results.json`。

### R02：生命周期与闪卡目录稳定清单（2026-11-03）

`PlanNativeDocumentLifecycle`及共享创建服务不再通过fs.WalkDir发现子文档。每个子树先完成两次一致的递归清单，重复请求必须匹配，Prepare/Create结束后再统一确认；枚举或最终确认失败不交付计划。`flashcardStore`和`flashcardNotebookIDs`也改用两次一致的单层清单，不完整读取、关闭失败或扫描间变化不返回部分牌组/文档ID。

缺失子树、递归顺序、`.docj`/`.deck`过滤、10000文档预算与既有闪卡语义保持；有效候选链接提前拒绝。共享清单仍只提供进程内稳定读取，不提供内容快照或跨进程CAS。

R02仍doing，字面未勾选仍44；native_document_lifecycle.go、native_flashcard.go、native_flashcard_read.go直接枚举已清零。workspace生产代码只剩read.go一处直接枚举。验证见AnonyPower执行计划的`wikiman-r02-lifecycle-flashcard-enumeration/results.json`。

### R02：最终文档源读取稳定清单（2026-11-03）

`readSource`现以两次一致的递归目录清单定位唯一docj，随后按清单身份打开候选、分块读取、检查文件关闭和路径身份；JSON/ID验证后再次建立全树清单，只有最终一致才交付原始AST字节和路径。

隐藏目录跳过、链接拒绝、10万扫描项、16MiB、唯一文档和未知字段保真语义保持。部分目录/文件读取、关闭、打开窗口替换或最终源变化均返回空结果；该边界不提供跨进程CAS。

R02仍doing，字面未勾选仍44；workspace业务生产代码直接fs.ReadDir/fs.WalkDir已清零。验证见AnonyPower执行计划的`wikiman-r02-workspace-final-read/results.json`。

### R02：Workspace 外围后台生命周期边界（2026-11-03）

Workspace生产枚举边界不变；本批补齐其外围legacy runtime的任务、定时器、事件scope及assets/emojis watcher关闭语义。初始化失败不保留半开owner，关闭超时不谎报排空或丢失重试句柄，活跃回调真实返回后才能完成关闭；R02字面未勾选由44降至43。验证见AnonyPower执行计划的`wikiman-r02-legacy-lifecycle-acceptance/results.json`。

### R02：导入/source-root 完成项复核（2026-11-03）

本批未改变Workspace低层协议；它复核已由后续批次完成的data ZIP、Markdown/HTML及PDF调用链均使用捕获owner、请求Context、checked root/file关闭和失败清理。历史复合项中不属于这些调用链的后台、历史/快照及广播缺口继续保留。R02字面未勾选43→29，验证见AnonyPower执行计划的`wikiman-r02-import-source-io-normalization/results.json`。

### R02：Workspace Context订阅自动注销（2026-11-03）

SubscribeIndexEvents与SubscribeOriginalSQLProgress现把传入Context绑定到LocalBus订阅生命周期。取消先阻止后续事件准入，再等待已经进入的真实回调；回调仍只使用resources owner，不捕获创建它的Lease。手工Close与Context取消可并发且可重试join。

该变化复用现有LocalBus和资源关闭屏障，不增加转发goroutine或公开topic。专项十轮、workspace整包和全仓分层竞态通过；它不代表其他compat事件桥或42类业务体完成，R02仍doing。验证见AnonyPower的`wikiman-r02-task-job-subscription-lifecycle/results.json`。
