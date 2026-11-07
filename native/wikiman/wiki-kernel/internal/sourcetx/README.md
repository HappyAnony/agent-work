# S04 source coordination and projection

This package is a local Go core API, not itself a Server or CLI command set. S05
connects shared wikiops to CLI and the portable server; the old GUI still does not use these
locks. Test against temporary portable fixtures, not the active legacy source.

R02 classified preferences: .wiki/preferences.json is the shared StorePreference
source; pages/<pageId>/data/.wiki/preferences.json is a
versioned WorkspacePreference leaf document, validated by generic Apply as well
as the preference service. Saves use the same source CAS/journal/recovery and
receipt path. Store preference receipts invalidate all page IDs (including deleted
nodes); preview, interruption rollback/forward recovery and generic Apply use the
same exact root path whitelist and typed scope validation. TreeSourceDigest remains
the page metadata graph digest; preference CAS uses the actual file hash.
Configuration reads overlay store, workspace, then device on an explicit AppData base
without creating missing defaults. Device preferences live only under this
store's .local/conf/device-preferences.json, use writer session/source locks and
single-file atomic CAS, and never produce a source receipt. Pending recovery and
symlink guards apply to all scopes. Root siblings/identity files are not thereby
write-authorized. Backup/Git/sync source whitelists must include this shared root
file and exclude .local; B1 snapshot/restore implementation is still pending,
not proven by the journal tests here. See the
[classification contract](../../conf/README.md).

## Document archives

Portable deletion records live at
`tombstones/documents/<pageId>/<boxId>/<docId>.json`. The matching directory
under `deleted/documents/` holds exact document, descendant and local file
bytes. Each record carries the original path, UTC deletion time and file
hashes, plus original sort ranks and hashes for any reversible AV effects.
The writer requires matching active and retained images in one commit.
Restoration consumes the record and retained bytes in the same commit.
Changed effect preimages cause a conflict. Archives are outside active indexes.

Explicit and pre-change versions live at
`versions/documents/<pageId>/<boxId>/<docId>/<operationId>/`.
The writer creates a document snapshot and any associated AV snapshots together.
Those files are immutable through this API. Journal recovery checks active and
archive paths with the same integrity rules and preserves the committed
operation's outcome for retry. Backups of portable source must include all
three archive namespaces.

## Ownership and locks

`Open(root)` canonicalizes aliases and provisions only `.local/{locks,journal,
staging,receipts,index}`. Lock files have stable inodes and are never unlinked to
claim ownership. Symlink/nonregular lock files fail closed. First access to a
read-only root without provisioned runtime files returns `source_read_only`.
Metadata/body/config reads never create documents, page.db or default config.
Each Open also binds the exact store.json hash. Read, Apply/Preview, Rebuild and
Recover recheck it while source-locked; replacing a registered store identity or
configuration returns source_identity_changed before callbacks/publication.
Closing and explicitly reopening/registering a source is required to rebind it.

`Store.Read(ctx, fn)` owns a shared `source.lock` over the entire logical read,
first checking unfinished/corrupt journals. `ScanTree` uses this path without a
persistent DB. Workspace reads and the combined HTTP/CLI read service use the
same guard; direct Scanner is a lower-level validator, not a locking entry point.

`Store.ReadContext(ctx, fn)` additionally passes a request-scoped admission to
the callback. Nested reads of the exact same Store reuse identity/journal
validation only while that shared source lock is held. This is not a persistent
cache: independent requests revalidate, another Store is rejected, and a context
retained after callback exit returns source_session_closed. Admission shutdown
revokes new borrowers then waits for already-admitted concurrent reads before
releasing the source lock, including error/panic paths. Cancellation is checked
at entry. Callers must finish their children; cancellation cannot safely unlock
under a child still reading. No write/lock upgrade is permitted in this scope.
The compatibility Read callback still works, but does not expose its scoped
context; nested domain services should use ReadContext.

`Store.Writer(ctx, wait)` owns an exclusive `session.lock` until Close. It may
remain open for an editor session. A second writer returns `store_busy` or waits
with context cancellation. Apply/Recover/Rebuild acquire locks in the fixed order
`session → source exclusive`. A cancellable per-writer queue serializes operations;
`CloseContext` waits for them rather than releasing a live session. An idle writer
session does not block readers. Calling a write operation inside a shared Read
callback is unsupported: do not upgrade a lock; return and acquire a writer.

All cooperating Go/Rust callers must obey this protocol. Locks cannot make an
uncooperative Git checkout, raw SQLite connection, rsync mirror or direct editor
write atomic. Those operations require releasing the knowledge store first.

## Apply, CAS and receipts

`Writer.Apply(ctx, Request, nil)` accepts file changes beneath
`pages/<pageId>/{page.json,data/...}`. No store identity/runtime/parent traversal
paths are writable. `ExpectedHash` is required semantically: empty means **must
not exist**, otherwise it is the exact SHA-256 of current bytes. Deleting a page
metadata file is forbidden; update deletedAt/revision for soft delete. Existing
page revision must increase by one; a newly created page starts at one. Optional
ExpectedTreeDigest protects a wider metadata base.

Before touching source, Apply validates all CAS conditions and native docj using
the shared original-byte span parser, overlays page metadata onto the complete
graph, checks schema/cycles/parents/entry documents, and rejects orphan page data.
It does not implement domain semantics such as automatic subtree selection or
docx page/title coupling; S05 constructs those composite requests.

`Writer.Preview(ctx, Request)` runs the same Apply preflight under the writer's
session/source locks: hashes, revisions, AST, paths, candidate graph and entry
overlay must all validate. It stops before staging/journal/index publication and
does not reserve the source for a future Apply. A new operation returns status
`dry_run`, sourceCommitted=false/indexFresh=false, candidate tree digest and
changed page IDs. A matching completed operation instead returns its historical
receipt with status `dry_run_replay` (sourceCommitted reflects that prior commit,
not a write by Preview); operation ID payload conflicts still fail. Runtime lock
provisioning is permitted. Manager.Preview uses the same cancellable store gate.

Publication sequence, while source-exclusive:

1. Publish a durable staging intent with operationId/request hash. Interrupted
   preparation can resume only the identical request after fresh CAS checks.
   Unknown staging material is never recursively discarded.
2. Write/fsync before/after images and their directory entries. A strict prepared
   journal records file hashes/basic POSIX modes, original absence and newly
   created parent dirs. ACL/xattr/owner snapshot restoration is not claimed here.
3. Persist projection-stale state before source publication. Publish each source
   file via a same-directory temporary file, fsync and rename, or unlink an
   explicitly deleted leaf. Recheck published hashes and the validated full tree.
4. Persist/fsync a commit marker bound to the journal hash.
5. Build/validate/publish the projection, then persist a synchronous receipt.

The caller gets operationId, status, sourceCommitted, indexFresh, changedPageIds,
treeSourceDigest and, for a fresh index, epoch/treeGeneration. A projection error
after commit returns `index_stale` **with sourceCommitted=true/indexFresh=false**;
it never silently rolls back committed source or claims indexing completed.
Pure readers may still scan that consistent committed source. Projected queries
reject stale state until a controlled Rebuild succeeds.

Operation IDs deduplicate within the retained local receipt history only. The
same normalized request returns its original receipt before CAS rechecking;
reusing an ID with a different payload conflicts. A rolled-back receipt remains
rolled_back; use a new ID for a deliberately new attempt. This is not permanent
cross-clone exactly-once processing. Journal/images/receipts are retained with
no automatic expiry/GC; removing `.local` also removes this deduplication history.

S05 content operations also persist a clientHash (canonical typed domain request,
excluding dryRun) in the request/journal/receipt. Read-locked Lookup allows a
completed logical create/delete/move to return its original receipt before
resolving a path that may no longer exist. A different domain payload conflicts;
unknown/invalid hash fields fail closed. The physical RequestHash continues to
bind all prepared changes. Recovery preserves clientHash from the durable journal,
so restart does not turn an idempotent domain retry into a second mutation.

## Recovery admission

Ordinary reads/writes reject an unfinished or invalid journal with
`recovery_required`; opening an editor cannot bypass it. Explicit Recover holds
session/source exclusively. For each unfinished operation it validates strict
JSON (including duplicate keys), identity, paths, all images and **all target
hashes before restoring anything**. Targets must match before or after bytes;
an unrelated edit stops recovery and is preserved.

Without commit, recover before images/absence; with commit, finish after images.
Only that operation's reserved temporary paths and recorded empty new source
directories are removed. No recursive source deletion occurs. Recovery may be
interrupted and repeated; completed receipts are skipped so an old journal can
never replay over a later operation. Index rebuilding and receipt persistence
finish recovery. Failure keeps admission closed; no automatic GUI unfreeze.
Committed operations with completed but index-stale receipts use Rebuild without
replaying their source. Unknown/orphan preparation directories remain diagnostic
material, not source truth or an excuse to delete unrelated files.

## Original edit dependencies and undo

Original edit candidates carry file hashes and directory membership for their
page read set. The writer rechecks them under exclusive admission before
preparing and publishing the journal, so a changed dependency or new file
rejects the whole plan. A validated no-change request records a receipt with an
empty changed page list. History reuses retained before and after images from a
committed operation, verifies their hashes, and applies them as ordinary CAS
changes. Editor history additionally checks the document scope bound to the
original receipt and journal. No history image is sent to the editor.

## Projection, snapshots and cursors

The persistent `.local/index/page.db` contains pages, documents, block spans,
references and schema/store/digest metadata. Apply overlays only requested source
files; a full O(V+E) metadata validator catches nonlocal effects. The SQL update
upserts changed records and descendant display paths, removes absent rows, and
reindexes only documents whose content hash changed. No mtime shortcut is used.

Candidate SQLite files are built in the same index directory; one transaction
updates the requested tables. Validate full page records/parents/order/paths,
content referential/bounds invariants and SQLite integrity_check. Close candidate
connections, fsync and rename only with source/session exclusion. This core never
returns a live persistent connection across the gate. Existing WAL/SHM/rollback
sidecars cause a safe busy/stale result; they are **not** separately removed,
copied without their DB or overwritten. A consumer using WAL must checkpoint and
close its complete SQLite lifecycle before controlled replacement.

`Rebuild(ctx, scope)` supports tree/content/all with separate TreeFresh and
ContentFresh flags. Default selection belongs to the future handler (all).
Tree scope does not parse bodies or claim content success. Force-all builds from
source even if DB is missing/corrupt; no old DB is treated as authoritative. Full
validation/rebuild and incremental action sequences are compared in tests.

`ProjectedTree` rescans current metadata under the shared source lease and checks
the projection's exact records/digests/epoch/generation before returning an
immutable snapshot. There is no stale ordinary-view fallback. Source metadata
and projection publication is gated until complete. `View.Cursor/CheckCursor`
invalidates pagination after a new generation or process epoch. Bounded detached
Events carry operation/changed pages/digest/epoch/generation; index rebuild and
source recovery also invalidate consumers. Persistent content manifests use
source path/hash digests; body-only edits do not change treeSourceDigest.

WorkspaceManager.Apply/Rebuild/Recover gate new acquisitions/tasks for that store, drain and
closes its read resources, invokes this coordinator, then publishes refreshed
metadata. Its advisory watcher is stopped/restarted across controlled mutations
to avoid late self-generated dirty events. Failure with unfinished recovery keeps
that store frozen. ClaimSession explicitly retains an editor's writer; these
operations borrow it rather than locking their own session twice. Without a
retained editor session, they create/close a short-lived CLI-style writer. Drain
releases a retained session only after leases and mutations end. Other stores
remain independent. Host/editor hookup is S06–S08.

## Limits and evidence

Limits fail explicitly: 10,000 files/request, 64 MiB/file (docj 16 MiB), 256 MiB
combined before/after bytes, 8 MiB journal, 4 KiB/64-component source paths,
100,000 docj and 512 MiB total docj per projection, 256 MiB old projection copy,
200,000 journal-directory entries, and 128 retained in-memory events. Native
parser limits are 100,000 nodes/512 levels. No backup retention/prune is implied.

```sh
GOPROXY=off go test -race ./internal/sourcetx ./internal/workspace ./internal/docast ./internal/wikistore ./internal/handlers/... -count=1
GOPROXY=off go vet ./internal/sourcetx ./internal/workspace ./internal/docast ./internal/wikiops ./internal/wikistore ./internal/handlers/...
```

Tests use copied synthetic fixtures only: CAS/cycles/entry deletion, incremental
create/move/reorder/subtree soft-delete/restore vs full rebuild, aliases and stable
lock inodes, cancellable queues/lock waits, malformed images/journals, foreign
target edits, ENOSPC injection, actual POSIX write denial, preparation resume,
SIGKILL before/after commit, interrupted recovery retry, stale-index receipts,
DB deletion/corruption, sidecar preservation, cursors, special-character paths,
manager drain/refresh and existing-read recovery admission. A temporary Rust
fs2 probe verifies Go shared/exclusive and session locks against the host's
native locking library; unavailable Cargo is an explicit skipped test, not proof.
The probe is offline and uses an isolated target directory. macOS arm64 is the
current exercised platform; Intel/bundled GUI integration remains later stages.

### B1 root shared-preference snapshot component

preferences_snapshot.go provides SnapshotStorePreferences, strict DecodeStorePreferencesSnapshot/Verify, a read-only restore plan and the Writer restore adapter. The component kind is root-store-preferences, version 1; it is not a full-store backup. The optional .wiki/preferences.json file is captured under the same shared source admission as the validated store/page metadata. Raw bytes, presence, exact path/size/SHA-256, canonical UTC timestamp, identity/format and the component digest are retained. Base64 in the JSON envelope preserves source formatting and integers above 2^53. Missing present/data/file fields, null presence/size, duplicates, unsupported versions, unsafe paths, invalid typed store scopes or secrets fail closed. A digest authenticates no source on its own.

Restore uses a new target plan: ExpectedHash is current-file CAS (empty means absent), TargetTreeDigest is captured from the current target, and the snapshot graph/hash remain provenance. Only the same store ID may receive the component; this store's pinned store.json hash is rechecked before exclusive publication. Canonical requests always target exactly StorePreferencesPath and reuse Apply's CAS, journal, recovery and all-page receipt. Existing committed receipts replay before stale CAS, preserving later edits. A rolled-back receipt remains uncommitted; deliberate retry requires a new operation ID. Source read admissions cannot be upgraded to restoration locks. Absent-to-absent is an asserted no_op with SourceCommitted=false and no new source receipt; all other restores publish exact bytes/deletion through the normal transaction.

The root-only capture creates no source defaults or archive directory and excludes page/device/runtime data. Fresh-process tests restore a serialized synthetic component without the old runtime. The complete B1 archive whitelist, durable sealing/incomplete markers, archive authentication/transports and complete new-directory restoration are S09 work, not supplied by this component. A full archive must compose all source reads under one shared admission, then release that lock before transport.

Payloads are bounded to 1 MiB and encoded envelopes to 2 MiB. Capture applies the same encoded bound as decode, including escaped identity text, so it cannot return a successful component that its decoder refuses to reopen. Consistency assumes writers obey the same source lock; unmanaged direct file writes are outside this protocol.
