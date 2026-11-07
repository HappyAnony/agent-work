# Shared Wiki services — S05
## Document lifecycle (R05)

`wiki-cli content --root PATH --request -` and host-authenticated
`POST /v1/content` use the same service. Every mutation carries an
`operationId`. Use `dryRun: true` to validate a candidate before publishing.
Reusing an operation ID with the same request returns its saved outcome. Use
current source hashes and a new operation ID after a conflict.

The Single template picker uses the editor ticket's page for template discovery
and requires its entry document ID for rendering. Read-only template SQL runs
on an in-memory projection of that entry. Page-wide index rows, attachment OCR
and attribute-view data are excluded. The UI render route only returns a
fragment. The normal scoped editor transaction publishes any inserted content.

| Action | Target and precondition |
|---|---|
| `document.create` | Existing `boxId`, a new `docId`, optional `parentDocId`, and Markdown or BlockDOM. `creationMode` also supports original Markdown, hierarchical path, daily-note and duplicate creation. The original daily-note mode can reuse the document for the day. |
| `document.import` | Base64 `data` for an original `.sy` document archive, into an existing `boxId` and optional `parentDocId`. The archive must use a single outer directory. Existing resources with different bytes cause a conflict. |
| `document.rename` | Current document `expectedHash`, plus `expectedPageHash` when updating the docx page and entry together. |
| `document.move` | Current document hash, optional `parentDocId` and `toBoxId` in the same page. A docx entry changing notebooks also requires its page hash. |
| `document.sort` | One sibling set in `order` and the sort file hash. Use an empty hash when no sort file exists. |
| `document.delete` | Current document hash. Use `cascade: true` to delete descendants. Delete a page and its entry through page soft deletion. |
| `document.deleted`, `document.recover` | List a notebook's deleted documents with `limit` and `cursor`. Recover one subtree using its tombstone hash as `expectedHash`. |
| `document.version.save/list/get/apply` | Save a version using the current document hash. Read or apply by `versionId` or `legacyHistoryPath`. Applying a legacy version requires `versionHash`. Applying over an active document requires its current hash. Restoring into an empty slot is create-only and may supply `parentDocId`. |
| `asset.rename`, `asset.delete` | Current attachment hash. Rename uses `newAssetName` and returns the original naming algorithm's resulting name. Delete is allowed only after checking live and retained references. |

Archive import remaps document, block and AV IDs and their references while
preserving creation timestamps and opaque source fields. Documents, sorting,
local and shared resources, AV data and bindings, and builtin flashcards are
published together. The result identifies the first imported document. The
request accepts up to 8 MiB of compressed input and up to 48 MiB expanded.
A document archive targets an existing notebook; whole-store restoration uses
the store recovery workflow.

The existing `document.put` native AST entry point accepts explicit
`notebookName` to create a missing notebook. Document deletion moves original
bytes into the portable retained directory and preserves attachments. Recovery
uses the original parent and refuses conflicts in active paths, IDs and affected
attribute views. Version application checks `expectedAVHashes` for each
existing attribute view it replaces. A version read returns saved AV hashes.
Original automatic history is read only from the owning page's runtime history.
`document.history` replays editor changes. `recover` resolves interrupted
source transactions.

`content check` audits one page's AST, parent paths, references, attachments
in open and closed notebooks,
attribute views, retained files and versions. `page check` with
`scope: "content"` also resolves block references across live pages. A
reference that resolves in its own document stays local. For other references,
one openable live document resolves the link and multiple documents are
reported as ambiguous. Soft-deleted pages are checked for retained structure and
history and are excluded as live link targets. Export resolves
attachments from the owning page, document and notebook. Editor tickets retain
their scoped editing permissions. The new lifecycle and version management
actions require the host credential.

## Original edit transactions (R04)

`wiki-cli edit --root PATH --request -` and host-authenticated `POST /v1/edit`
call `wikiops.Edit`. Send one store, page, unique operation ID, page-relative
source hashes and a batch of original operations. The 74 actions in
`editcore.EditingActions()` cover document blocks, attribute views and flashcard
membership. The legacy internal `create` action receives an in-process tree;
the document creation and recovery workflow belongs to the lifecycle API.

```json
{"protocolVersion":1,"storeId":"STORE_ID","pageId":"1760000000000000002","operationId":"edit-example","dryRun":true,"operations":[{"action":"setAttrs","id":"20261019090002-par0001","data":"{\"custom-example\":\"new value\"}"}]}
```

Preview returns `requiredSources` and candidate hashes without staging or
publishing. Commit with the required hashes and a unique operation ID. The
operation batch sees its own candidate changes; the writer checks both the
explicit preimages and the complete captured page read set. A conflict requires
reading current sources and planning a new operation. Repeating an identical
committed request yields its original receipt and committed hashes even after a
later edit. Source and index outcomes are separate: `receipt.sourceCommitted`
means durable publication and `indexFresh` means the original index is ready.
If index rebuild fails after commit, retry the same request to repair it.
Editor tickets cannot use the host route.

For undo or redo send `history: {"operationId":"edit-example","redo":true}`
instead of `operations`, with a new request operation ID and the current
expected source hashes. The kernel checks and applies exact before or after
images using the same journal and CAS. Scoped editor history is additionally
bound to the document of the original editor operation. Journal recovery and
user-facing historical version management are separate operations.

## Original read/query services (R03)

`wiki-cli query --root PATH --request - [--json]` and authenticated
`POST /v1/query` call the same `wikiops.Query` registry and original domain
services. The 68 actions below are read-only. `index.rebuild` replaces only
owner-local disposable main/blocktree indexes; it is not a source save.
The older `content.search` byte/text projection and `page index` metadata/span
projection retain their documented purposes; original full-text business lives
in this query service.

A request is `{protocolVersion:1,storeId,pageId,action,options:{...}}`.
No request accepts a root, host directory, role upgrade, writer capability or
current-workspace fallback. Unknown actions/options, duplicate JSON keys,
trailing input and mutation SQL fail. Options are at most 1 MiB, strings at most
64 KiB, collections at most 1000; original domain pagination/output/AST budgets
also apply. Complete request decoding remains capped at 16 MiB. Query CLI
rejects content-only output/converter flags and validates before opening a root.
The authoritative per-action option types and mapping are in
[query.go](query.go), exposed to maintainers by `QueryActions()`.

The native Server uses explicit pure original read defaults (`ReadDefaults`),
then the existing classified store/page/device preference resolution. It does
not load the legacy device AppConf or start a GUI, scheduler, sync or OCR engine.
Original language/time labels are embedded from the inherited 13 language files.
Configuration responses use the restricted detached projection and omit private
workspace paths/secrets. Existing device/process administration is not granted
through source JSON.

| Read family | Actions |
|---|---|
| Documents and navigation | `document.ast`, `document.view`, `document.search`, `document.tree`, `document.treeids`, `document.paths`, `document.info`, `document.infos`, `document.statistics`, `document.assets` |
| Blocks | `block.get`, `block.read`, `block.dom`, `block.position`, `block.relations`, `block.fold`, `block.location`, `block.attributes`, `block.attributes.batch`, `block.avkeys`, `blocks.recent`, `outline` |
| References | `references.search`, `references.backlinks`, `references.definitions`, `references.text`, `references.check`, `references.embeds`, `references.query`, `references.block`, `references.invalid` |
| Full text and labels | `search`, `sql`, `tags.search`, `tags.tree`, `tags.blocks`, `tags.spans`, `bookmarks`, `bookmarks.labels`, `graph` |
| Attribute views | `av.get`, `av.search`, `av.name`, `av.keys`, `av.primary`, `av.view`, `av.render`, `av.items`, `av.bound`, `av.images`, `av.mirrors` |
| Files/resources | `asset.get`, `asset.list`, `assets.names`, `assets.references`, `assets.search`, `assets.content`, `file.read`, `file.list`, `templates.search`, `widgets.search` |
| Configuration/local read state | `configuration`, `configuration.notebooks`, `configuration.notebook`, `configuration.emojis`, `notebook.info`, `storage` |
| Disposable index | `index.rebuild` |

`document.assets` selects image-only destinations with `Images:true`.
`block.get` returns the original Block display projection; `block.read` keeps
the shared multi-mode block result. Single/bulk info actions preserve their
inherited different missing-subdirectory behavior. `storage` accepts only
`recent.get`, `criteria.get`, `local.get`, `outline.get`; proposed read-time
compaction is not published. `references.query` selects the 15 fixed original
count/definition/annotation/recent query modes; it accepts typed filters, not SQL
fragments. `sql`, embed SQL and search method 2 accept one SELECT only.
Template catalogue paths are page-relative identifiers. `file.read/list` are
confined to page data; neither accesses the local runtime or device output roots.

```json
{"protocolVersion":1,"storeId":"STORE_ID","pageId":"1760000000000000002","action":"search","options":{"request":{"query":"knowledge","method":0,"page":1,"pageSize":20},"groupBy":0}}
```

Result: `{storeId,pageId,action,sourceHash,value}` in the usual versioned
success/error envelope. `sourceHash` binds the checked page-data and parser/index
inputs; it is neither a document CAS hash, source receipt, metadata tree digest,
nor permission to save a displayed projection. Use existing document.get hashes
for later content operations. Local browsing state remains local and is not
included in this source/index key.

One store read admission covers capture, original index preparation, business
read and final source verification. Checked file/directory identity, bounded
streaming reads and final hashing reject replaced/changed source without a
partial result. Raw AST/config/file reads do not require original AST indexing:
unknown native nodes and unknown JSON numbers remain readable and untouched.
No read upgrades an AV file, creates a missing body, repairs metadata or grants
source writes. Corrupt notebook metadata fails instead of hiding partial rows.

Original main/FTS/spans/attributes/refs/asset-link/blocktree projections are built
in a private candidate bundle, with all definitions present before reference
projection. Only a complete successful build is adopted under the owner gate.
Source edits/deletes invalidate the projection; missing derived tables trigger
rebuild. History and asset-content databases/queues survive main-index adoption.
Block/reference/IAL caches are invalidated on publication; accepted queue work
retains its progress/error semantics. Separate workspace bundles isolate even
identical document/block/AV IDs.

Attachment content is rehydrated only for content search/detail: original bounded
UTF-8/Office/PDF/EPUB converters are shared, conversion children are canceled and
joined, and successful complete results replace the disposable index. Attachment
row IDs are stable opaque path hashes across fresh CLI/HTTP index instances;
they are not source node IDs. No conversion runs for name/raw-file reads.

The route requires the host Bearer credential plus the active managed session.
Editor tickets are refused on this host query namespace; original desktop/single
API/WS/UI adapter assembly remains R06. Durable source edits/receipts, creation
and lifecycle restore workflows retain R05 gates. R03 completion is not whole-product
UI or backup completion.



The page handler calls `wikiops.Pages` directly: no HTTP client, GUI or daemon.
`readhttp.NewOperations(manager, token)` adapts the same request/result/error to
authenticated `POST /v1/pages`. Roots must already be registered by the host;
HTTP never accepts a root path. Tokens must be at least 32 bytes, travel only in
Authorization: Bearer headers, and must not be committed. This adapter does not
itself start a listener, enable CORS or mount on the legacy server. The explicit
portable kernel entry owns the listener; Session owns WebSocket lifetime.
The older `New(manager)` read adapter remains an unmounted test prototype, not
an authenticated public server.

On the managed portable Server, data routes additionally require the current
`X-Wiki-Session` from its ready record. The authenticated `/v1/session` control
route releases/resumes source ownership without changing PID and rotates the
generation on resume. See the [Server contract](../../README.md#portable-server).
Unbound in-process adapter tests are not a model for production authentication.

## CLI and schema

```sh
wiki-cli page --root '/path/to/inactive portable wiki' --request - --json
```

The command reads **one** JSON object from stdin (16 MiB limit), always writes
one JSON envelope to stdout and drains its contexts before successful return.
`--key=value` and separated values are supported; duplicate/unknown options,
missing values followed by --help/-h, duplicate JSON keys, trailing JSON, unknown
fields and unsupported protocol versions fail before opening the root.
`RunWithInput` is the native stdin entry; the older `Run` test helper supplies an
empty reader. Help/version do not read stdin or open roots. Exit statuses: 0
success, 2 argument/operation failure, 1 output failure. No logs go to stdout.

All requests contain `protocolVersion:1`, `storeId` and `action`:

| action | Additional request fields | Result |
| --- | --- | --- |
| get | pageId | One page record, raw metadata hash, display path, entryHash for docx |
| list | query?, includeDeleted?, limit?, cursor? | Page name substring search (case-insensitive), total and nextCursor |
| apply | operationId, updates, expectedTreeDigest?, dryRun? | Durable source/index receipt, or validated dry-run receipt |
| check | none | Full metadata graph/schema/entry-path validation diagnostics |
| index | scope: tree/content/all | Actual rebuilt index freshness/epoch/generation |

Options not applicable to the action are rejected, not silently ignored. get can
inspect a tombstone; list excludes deleted pages by default. Sorting is display
path then pageId. Default page size 50, maximum 1000. Cursors bind source metadata
digest, query, deleted filter and offset; a metadata change or different filter
returns cursor_invalidated rather than skipping/duplicating results. These
source-scan cursors are independent of persistent projection epochs. Empty result
lists may omit total (zero); missing nextCursor means exhausted. The default
`check` validates the metadata tree and entry existence. Request
`scope: "content"` for the additional checks above. Neither mode performs
repairs. Source scan errors fail rather than returning success.

Example stdin for name search:

```json
{"protocolVersion":1,"storeId":"STORE_ID","action":"list","query":"Wiki","limit":5}
```

Each apply update is `{page: <complete page.json record>, expectedHash,
entryExpectedHash?}`. Obtain the record/hashes from get; preserve ext and the
nullable fields, edit the intended values, increment revision and supply updatedAt.
The caller supplies IDs/timestamps: retries do not generate a new payload based
on current time. Empty expectedHash means create-only/revision 1; otherwise it is
the exact previous raw SHA-256 and revision must increase by one. Rank/parent
changes move/reorder the page. Soft-delete sets deletedAt, restore clears it;
include all affected descendants in one batch so the full tree remains valid.
Physical page-directory deletion and automatic subtree selection are not exposed.

For every docx update, entryExpectedHash is required. The service resolves its
native nested notebook document ID (no filesystem path in the request), preserves
unknown AST fields/numbers as RawMessage, updates only Properties.title to the
page name, and stages both changes in the **same** source transaction. Duplicate
AST fields/IDs or ambiguous/symlink document paths fail closed. Existing entry
documents are required by page apply; use content document.put with pageUpdate to
create a new docx together with its entry. Other page types may be created without data.
Maximum 1000 updates / 64 MiB prepared after-images. The core independently
validates path/CAS/AST/complete-tree invariants and publication limits.

Dry-run uses that very same locked preflight, before source/staging/journal/index
writes. Runtime locks/directories may be provisioned. It is not a reservation;
Apply revalidates. Completed operation IDs replay retained receipts; changed
payloads or stale hashes under a new operation ID conflict. Receipt history is
local, not a cross-clone exactly-once guarantee. See the
[source transaction contract](../sourcetx/README.md) for recovery and partial
source-committed/index-stale semantics; a failure can include a real commit receipt.

Both page adapters return `{ok, protocolVersion, result}` or
`{ok:false, protocolVersion, error:{type,hint,retryable}, result}`. Conflict/busy/
invalidated-cursor HTTP errors use 409, malformed requests 400, auth failures 401.
Authentication is checked before decoding/reading any source. Output never
includes tokens or accepts arbitrary root/runtime source paths.

## Content and recovery schema

`wiki-cli content --root PATH --request - [--json]` and authenticated POST
`/v1/content` accept the exact same ContentRequest. Required fields are
protocolVersion:1, storeId, pageId and action. All document/block operations also
require boxId/docId; block operations require blockId. Irrelevant nonzero fields
are rejected. Request types/Validate in content.go are authoritative.

| action | Extra fields | Semantics |
| --- | --- | --- |
| document.get / block.get | format: ast/text (default ast) | Original AST or text projection; hash is always the whole document CAS version; block refs are subtree-scoped |
| document.render | none | Native document rendered as Protyle BlockDOM; returns whole-document hash and html, not a second source format |
| document.export | format?: html/docx/pdf | Default inert self-contained HTML; docx uses trusted Pandoc, pdf uses the macOS wiki-pdf helper; binary outputs return base64 data/mediaType/hash; source/assets captured under one read admission |
| document.transactions | operations, nativeHistory? | 1–1000 supported Protyle operations committed together; nativeHistory:true enables journal-based undo for stable opaque subtrees; success returns historyId and originalIndexFresh |
| document.history | historyId, redo? | Apply exact server-side before or after images of an owned editor transaction using current expectedHash |
| document.list | query?, limit?, cursor? | Native paths, optional title and hashes in this page; query matches path; title is a read projection, never a source rewrite |
| search | query?, limit?, cursor? | Case-insensitive title/text substring search of native documents |
| check | none | Parse native document AST/IDs/references; invalid AST, ambiguous IDs and unresolved in-page refs produce sorted diagnostics and failure |
| document.put | ast, parentDocId?, pageUpdate?, notebookName? | Create or replace native document; optional full PageUpdate and entry must agree on title in the same transaction |
| document.restore | data | Explicit legacy byte import, separate from undo; exact base64 native bytes, max 4 MiB; existing document and nonempty expectedHash required; same AST/entry title/CAS/journal validation |
| document.delete | cascade? | Delete a document; descendants require explicit cascade:true; an active docx entry cannot be orphaned |
| document.move | parentDocId? | Move document and all descendants within this page/notebook; absent parent means notebook root |
| block.create | ast, parentId, beforeId? | Insert a new ID under an existing parent, optionally before one of its children |
| block.replace | ast | Replace that block with an explicit AST retaining its ID; other nodes/unknown fields survive |
| block.delete | none | Delete that block subtree (not the document root) |
| block.move | parentId, beforeId?, targetDocId?, targetExpectedHash? | Reorder/reparent; optional target document in the same page/notebook requires its own CAS and commits both documents atomically |
| asset.get / asset.list | assetName for get; query/limit/cursor for list | Confined page data/assets; get returns base64 data/hash |
| asset.put / asset.delete | assetName; put includes base64 data | Native binary attachment CRUD; safe relative names, no symlinks/traversal/hidden or .docj names |

`document.export` is read-only and returns `html`/`hash` in the normal JSON
envelope; it does not accept mutation fields or start a conversion server. It
uses the native export renderer, sanitizes to a static reading projection and
embeds only same-page assets under one read admission. Missing/unsafe assets
fail rather than produce partial success. The [editor session contract](../../../wiki-app/src/portable/README.md)
documents the exact MIME/size limits, CSP, blocked interactive content and
remaining reading-projection limitations. For example, stdin for `wiki-cli content`:

```json
{"protocolVersion":1,"storeId":"STORE_ID","pageId":"1760000000000000002","action":"document.export","boxId":"20261019090000-box0001","docId":"20261019090001-doc0001"}
```

All mutations require operationId, expectedHash (empty means create-only), and
accept expectedTreeDigest/dryRun. Deletion/block edits/moves require nonempty
hash. Timestamps/IDs are supplied, never guessed at execution/retry time. Read
the source version again after a conflict; do not silently overwrite it.

Rich-text updates, inserts, moves and attribute changes use the original
operation core. Untouched native subtrees and unknown fields on stable IDs are
preserved. HTML-only requests reject removal of hidden subtree metadata.
`nativeHistory:true` permits reversible removal of stable-ID subtrees.
Ambiguous anonymous row or inline extensions fail closed before publication.
The response contains an owned journal handle; it carries no history image.
DOCX conversion runs after the read barrier using only the captured sanitized
semantic HTML/embedded assets; it never re-reads mutable source or adopts a save
CAS hash. Trusted handlers configure WithPandoc (absolute executable/archive),
not request JSON. CLI/Server --pandoc is optional; Server can use the existing
host architecture archive under app-dir/pandoc. No conversion on ordinary reads.
Errors converter_unavailable/conversion_failed preserve source and editor saves.
Fixed sandboxed Pandoc arguments, private temporary HOME outside source, 20-second
deadline/serial gate, max 16 MiB DOCX, max 256 MiB executable/archive; exact single
regular ZIP entry extraction only. No user filters/templates/args/downloads.
Pandoc's sandbox option supplements sanitized HTML; trusted executables are not
an arbitrary-code OS sandbox. Word output is a reading projection; attachment
backup/full dynamic/native fidelity remain outside this implementation.

PDF uses WithPDF context configuration from trusted handlers, with sibling
wiki-pdf resolution or --pdf-helper ABSOLUTE_EXECUTABLE (CLI PDF export only).
Only macOS is supported. Core captures the same sanitized source/assets snapshot
before releasing the read barrier; convertPDF reuses trusted-path/private-temp/
bounded-output/deadline protections, never archive-extracts or accepts page args.
The fixed --html-stdin helper protocol returns PDF bytes; signature/EOF checked,
max 16 MiB. The helper independently validates with system PDFKit, bounds pages,
disables scripts and external/file loads and uses silent asynchronous A4 printing.
Absent/failed helper does not mutate source or poison the save queue. It is not
an arbitrary-executable OS sandbox or complete native/attachment backup format.

Optional notebookName creates only a missing `.wiki/conf.json` in the same
transaction; omit it for an existing notebook. Existing configuration is never
silently overwritten. A new document with parentDocId checks/guards its parent;
moving documents guards the destination parent and all descendant source hashes.
Parents/cycles/entry existence are checked on the final candidate tree.

PageUpdate has the page apply shape but entryExpectedHash must be omitted here:
the content request's expectedHash is the document version. Changing an entry
title requires the matching full page record/revision and metadata expectedHash.
Creating a docx requires its entry document and page record in this same request.
Source remains native .docj, never Markdown. Plain text is only a read projection
of native Data/TextMarkTextContent fields; unsupported encodings fail explicitly.
Structural AST validation preserves opaque RawMessage fields and large numbers;
it is not a full rich-text semantic normalizer or automatic repair engine.

Rich-text operations have `action`, `id`, optional `data`, `parentID`,
`previousID`, `nextID`. Supported actions are update/insert/delete/move,
setAttrs and foldHeading/unfoldHeading. Update/insert data is one BlockDOM HTML
block retaining its ID. setAttrs accepts a string-valued object (or its JSON
string); null deletes an attribute. Document-root/title edits, unknown actions,
ambiguous placements and unpreservable opaque inline changes fail closed.
The existing Lute/dataparser dependencies provide the codec; unknown native
fields and large numbers stay in RawMessage values rather than JS AST roundtrips.
Rendering is a projection, not a source rewrite or proof of full editor parity.
An accepted batch returns its exact candidate hash, not a later writer's hash.
Receipt replay intentionally has no hash: clients must stop and reread rather
than advance CAS from a possibly newer file. Uncertain writes remain dirty.

`--output text|ast` emits just one document/block read (without an envelope);
it is incompatible with --json. Request format:text without --output returns a
normal JSON result containing text. JSON success/failure envelopes, statuses and
partial-commit receipts are identical to page operations. Empty lists return
total:0; missing nextCursor means exhausted. Content cursors bind page, action,
query and every bounded source file hash, so body/attachment edits invalidate them.
Lists/search/check never publish an index or repair data. Search scans current
source under one shared guard; persistent page.db can be rebuilt independently.

Limits: 16 MiB JSON input/document; 8 MiB decoded attachment writes; 1000 list
items per response (default 50), 100,000 source entries/512 MiB per page scan;
64 MiB prepared after-images, 10,000 transaction files. Paths are confined via
os.Root and no symlink traversal. Blob resources are returned as opaque downloads,
not executable HTML. These are bounded local operations, not streaming large-file
upload or a general filesystem API.

### CLI status and recovery

`wiki-cli status --root PATH --request -` accepts
`{"protocolVersion":1,"storeId":"…"}`. It returns the CLI build version, local
session lock availability, source lock readability, pending recovery and
verified tree and content index freshness. On a pristine source both locks
are `uninitialized` and both indexes are `missing`. With runtime files the
session lock is `held` or `available`. A concurrent source publication
returns `store_busy`, exit 2, with `sourceLock:"busy"`. Status does not
reserve a writer. Index states are `missing`, `stale`, `fresh` or
`unverified`. The last value means a pending or invalid journal prevents
index admission. The optional `index` object contains the last published metadata.
Status opens any existing projection read-only and does
not create locks, rebuild indexes, start a listener or probe a server.

`wiki-cli recover --root PATH --request -` accepts
`{"protocolVersion":1,"storeId":"…","dryRun":true}` for a read-only preview.
The ordered `preview` array lists each pending operation, its
`roll_back` or `roll_forward` direction, and the source paths, target
SHA-256 hashes and modes. An empty target hash means removal. Rollback
also lists directories scheduled for removal in removal order. The same
image and target validation runs before real publication. Completed
operations are omitted. Empty preview means there is no pending recovery.
Preview never creates runtime files or makes a reservation.

Omit `dryRun` or set it to false to apply. The result carries durable
`receipts`; inspect `sourceCommitted` and `indexFresh` even when the command
reports an error. Recovery can admit a half-published tree and verifies
the store identity before writing. A held editor session returns
`store_busy`. A foreign change to a target returns `source_conflict`.
Missing or corrupt recovery material returns `recovery_required`.
Use the same explicit root for preview and apply.

Authenticated POST `/v1/status` and `/v1/recover` use the same request and
result contracts on the registered store. They require host authentication
and the active host session, since editor-scoped tickets cannot inspect or
repair the store. An interrupted source at server startup is handled with
the local CLI before normal registration. `/v1/session` is the separate host
lifecycle endpoint for release and resume.

### Agent command map

| CLI | Shared service | HTTP or UI entry |
| --- | --- | --- |
| `read` | Bounded native AST projection | Read handler |
| `page` | Page graph, batch CAS, checks and index rebuild | `/v1/pages` |
| `query` | Original read registry (`QueryActions`) and index maintenance | `/v1/query` |
| `content` | Original document, block, asset and history services | `/v1/content` |
| `edit` | Original Protyle operations and native history | `/v1/edit` |
| `status`, `recover` | Source lock, journal and projection services | `/v1/status`, `/v1/recover` |

Use `--help --json` to discover commands and exit codes without opening a
source. Call with an explicit root and IDs. Read the current hash before
mutating, preserve the operation ID for an identical retry, and generate a
new operation ID for a changed payload. A successful mutating process
returns after source and projection handling and releases its writer. For
a GUI-owned source, save and release in WikiMan, run the CLI command, then
resume in WikiMan so it reloads the new source generation.

Editor tickets, UI layouts, device preferences and event subscriptions
remain owned by the host and `/v1/ui`. The original UI also owns scoped
interaction-only workflows, such as tag-dock rename and PDF annotation
dialogs. The CLI exposes the shared business operations listed in its
contract, independently of those UI workflows.

Content operations persist their canonical domain clientHash in journal/receipt
(excluding dryRun). Retained completed creates/deletes/moves replay before path
lookup, including after source recovery. This is still local retained-history
deduplication, not cross-clone exactly-once. Physical CAS validation remains in
the source core, and interrupted/foreign edits are never overwritten implicitly.

## HTTP, resources and WebSocket

The portable server mounts POST /v1/pages, /v1/content, /v1/status, /v1/recover, /v1/editor
and /v1/ui, and GET /v1/resource?storeId=...&pageId=...&name=... . Resources
require authenticated registered identities and a safe asset name. A request
may also supply boxId and docId for document-owned attachment lookup. The
server returns no-store, nosniff, content-disposition:attachment and a
content-hash ETag. Unknown roots, duplicate identity parameters and URL-token
authentication are rejected.

POST /v1/ui accepts a protocol version, storeId, pageId, allowlisted original
endpoint and endpoint-specific JSON. The editor ticket and generation are sent
in X-Wiki-Editor and X-Wiki-Session. The successful result repeats storeId,
pageId, viewId and sessionId alongside the projected data. The host's Bearer
credential does not authorize this route. Single-view requests resolve blocks
within the current entry document. Desktop requests can use their own page.
Source writes call the shared planners, CAS and journal services.

GET /v1/events?storeId=...&pageId=... upgrades only after identity/auth/origin
validation. Native clients use Authorization: Bearer. Browsers may offer exactly
`[wiki.v1,wiki.auth.<TOKEN>]` subprotocols instead; only wiki.v1 is echoed. Origins
must match the server host (missing Origin is permitted for native clients).
Controlled views offer wiki.v1, wiki.editor.<TICKET> and wiki.session.<SESSION>
and receive frames tagged with both viewId and sessionId.
Never put real tokens in URLs/logs/Git. Connection budget is 64; payload reads
are bounded to 1 KiB. A workspaceReady frame precedes workspaceEvent frames,
each explicitly tagged storeId/pageId. Changed-page IDs are filtered to the
subscription. Heartbeats are transport events only, never source mutations.

The transport polls the core's bounded 128-event history. It is advisory, not a
durable event log: reconnect must fetch current state, and resyncRequired closes
a detected history gap/epoch change. A slow client has a 2-second write deadline.
Subscriptions hold no editing lease (which would deadlock writes); Session owns
their sockets/readers and closes/joins them before server manager drain. No task
or listener is launched by importing a package. CLI does not import the listener.

## Boundary after S05

Existing WikiMan uses the isolated wiki-compat runtime until S06–S08. These
services do not complete the old frontend/native host migration, snapshots,
backup/sync, real data migration, DMG release or cross-architecture validation.
Use synthetic/inactive portable sources until those stages are verified.

Tests cover direct service lifecycle (create/move/delete/restore), CAS, cursor
invalidation, strict input, true dry-run, unknown AST/large number preservation,
idempotent retry, and authenticated HTTP vs stdin CLI metadata/body/reference
equivalence. Native process smoke runs outside the repository with special
characters in paths, isolated HOME and immediate durable-file checks after exit.
