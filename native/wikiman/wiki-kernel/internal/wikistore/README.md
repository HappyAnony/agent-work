# Portable Wiki metadata reader

This package implements the S02 source-layout and page-tree read layer. It does
not boot legacy model workers, create `.local`, rebuild SQLite, or edit source.

- Schema contracts: [store](schema/store.schema.json), [page](schema/page.schema.json).
- `Scanner.Scan` validates every page metadata record before returning a tree.
  Duplicate JSON keys, unknown top-level fields, invalid identities/versions,
  missing/cyclic/deleted parents, unsafe paths and invalid entry documents fail
  without publishing a valid-looking subset. Empty pages and duplicate names
  are allowed. Duplicate ranks warn and sort deterministically by string ID.
- A snapshot's accessors detach mutable fields. Its digest uses path and byte
  hashes rather than timestamps; body-only edits do not change page-tree identity.
- `Tracker` treats filesystem notifications as dirty signals only. Explicit
  refresh always rescans, including when events were lost. Invalid refresh never
  exposes a previous tree as current. Watchers have explicit context/close lifetime.
- Defaults: 100,000 page directories, 1 MiB per metadata file, 64 MiB total
  metadata, depth 10,000, JSON nesting 64. Limits are configurable; exceeding one
  returns diagnostics rather than silently truncating data.

StoreContext's shared source lock, journal checks, SQLite projection and atomic
write/publication are implemented in [sourcetx](../sourcetx/README.md); callers use
that coordinator instead of treating raw Scanner as a locking API.
Candidate reuses the same complete metadata graph validator with the transaction
owner's overlay entry-document resolver before any source publication.
Double scanning detects changes but cannot
guarantee isolation from arbitrary external writers. Entry checks establish
native document identity/path existence, not full AST/asset integrity; those
belong to the content check service.

```sh
go test -race ./internal/wikistore
```

Tests use temporary copies of the hand-authored root fixture. They cover valid
and malformed metadata, cycles/deletions, path aliases/symlinks, limits, same-mtime
changes, body-only edits, snapshot detachment, watcher invalidation and concurrent
reader/refresh behavior. No personal data or runtime database is needed.
