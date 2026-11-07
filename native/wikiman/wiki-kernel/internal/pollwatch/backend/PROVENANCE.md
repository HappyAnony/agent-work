# Polling backend provenance

- Source: `github.com/radovskyb/watcher`, module version `v1.0.7`, the existing
  kernel dependency; copied from its immutable Go module source, not a new download.
- Upstream: https://github.com/radovskyb/watcher/tree/v1.0.7
- Files: watcher.go, ishidden.go, ishidden_windows.go, samefile.go,
  samefile_windows.go. The original BSD-3-Clause terms and copyright are in
  [LICENSE](LICENSE). No upstream tests/examples/CLI are distributed here.
- Changes: poll completion closes a channel rather than an unbuffered send;
  cancellation joins the current helper; polling sleep and event delivery select
  on shutdown. Nonrecursive Add, event diff/rename/write/chmod behavior and poll
  intervals are retained. All other source is inherited unchanged except Go
  formatting and build-tag normalization.
- Scope: private backend for context-owned polling watches; the existing module
  remains for its public event types. No module-cache mutation or global replace.
- Binary distribution must include these BSD-3-Clause notices with the existing
  original kernel notices. Local verification is not distribution/legal approval.
