# Local source modifications

The imported baseline is recorded in `source-snapshot.json`.

- `wiki-app/package.json`, its lockfile and webpack entries build portable assets independently, without Electron distribution or standalone launch scripts.
- `wiki-app/src/portable/boot.ts` and `bridge.ts` accept the DSH locale over the private editor channel.
- The portable editor and kernel assets allow the exact packaged-client parent origins `dsh-app://app` and `dsh-app://localhost` alongside local Web and IronMan windows.
- `wiki-app/src/portable/mount.ts`, `localCapabilities.ts`, `portable.scss` and the portable webpack aliases remove official account bootstrap and membership gates, use native document-action menus and resize the editor when panes change size.
- The portable kernel server removes cloud-account configuration and boot wiring. `internal/wikiops/ui.go` rejects account, cloud and official sync routes; the corresponding private device-cloud adapters are removed.
- Imported Go tests use the local copied fixtures. Account-route expectations match the portable denial policy.
- The DSH Host bridge and tab/split controller in `packages/experimental/wikiman/src/client/portable-{channel,workbench}.ts` adapt IronMan's `portable/bridge.ts` and `features/wiki-runtime/services/portableEditor.ts`; the DSH plugin owns their lifecycle, localization and Remote operations.
- Portable requests support an authenticated DSH relay and a separate event-stream origin for packaged remote clients; parent origins admit remote documents only when exactly same-origin. Narrow touch layouts collapse native side panels into overlays.
- `build.sh` can reuse an existing frontend while compiling native Go binaries for Linux or macOS; the macOS PDF helper remains platform-specific.
