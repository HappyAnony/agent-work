# Pinned upstream and scoped changes

Source: github.com/siyuan-note/dejavu, v0.0.0-20251202041457-7402b7c625c5.
Original AGPL-3.0 license and copyright headers are retained. This directory is a
local Go module replacement, not a second repository implementation.

The optional Conf.RequestContext is instance owned. Official provider HTTP
requests and upload SDK calls receive it; nil retains upstream background
behavior. The kernel's metadata adapter binds S3/WebDAV transports to the same
operation Context. Metadata iteration checks cancellation. Scoped tag queries
suppress the original global index-download event; legacy callers retain it.

Metadata handlers do not gain directory/snapshot write authority. A separate
directory capability now uses the extracted cloudlock core. Remaining full-sync
queues, source operations and checkout lifecycle require further migration.

Scoped metadata bypasses the upstream process-global index/file caches, validates
object IDs before forming local paths, and explicitly releases its store codecs.
Malformed metadata replies return errors rather than panicking. The original
global caches remain available only to legacy callers.
S3 scoped configuration uses the explicit region/credentials instead of loading
shared AWS profiles; its SDK retry wait also receives the operation Context.

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
