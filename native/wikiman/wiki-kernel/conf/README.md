# Original configuration data and ownership

`AppData` is the original 31-field serialized AppConf contract, not a reduced
replacement. `model.AppConf` embeds it; original JSON field names and UI accesses
are unchanged. The non-persisted `User` view is also retained and deep-cloned.
This package does not import model/util/sql/treenode, read process environment,
detect a device, open a file, select a workspace, or start a worker at import.
Explicit `NewAPI` still creates a token only when the original boot asks for it;
Context acquisition never calls it or the default factories.

## Classified persistence (R02 partial)

PreferenceDocument v1 is a replacement document of typed leaf JSON paths, not
an unvalidated flat AppConf import or a merge patch. Preferences(scope) exports
one owner; ApplyPreferences reconstructs a detached full original UI projection
over an explicit base, preserving integer precision. Missing leaves inherit that
base. Duplicate/unknown/wrong-owner fields, scalar nulls and runtime authority
are rejected; no defaults/tokens are created by loading.

Workspace preferences persist at pages/<pageId>/data/.wiki/preferences.json
through the existing sourcetx CAS/journal/recovery path. The reserved document is
validated even through generic Apply. Device fields persist only in this store's
local .local/conf/device-preferences.json under existing session/source locks,
atomic replacement and CAS; this is not a shared source commit receipt. Missing
documents are read-only absence. Manager mutations require releasing live leases
first and refresh contexts before reads resume. No old personal conf is migrated. SetIndexPending shares the original change/flush
callback business; Context state is runtime-only, never a classified source write.

Device exceptions now include repo.key, sync.s3/webdav/local, local export
template/watermark paths and editor.plantUMLServePath. sync.synced/stat and
export.pandocBin are runtime values. These must not enter a workspace document.
StorePreference persists at .wiki/preferences.json through the same source
transaction service, with scope=store and all-page receipt invalidation. Overlay
order is explicit base, store, workspace, device (each has disjoint field owners).
A shared document cannot import device secrets or runtime state. Full original settings/import
controllers and notebook repair still need owner adapters; original SaveError
remains the explicit compatibility single-file binding.

## Classification

`ScopeFor` validates field names against the original typed contract and resolves
the most-specific policy; `FieldScopes` returns a detached policy inventory.

| Default owner | Original top-level JSON fields |
|---|---|
| Device preferences (17) | logLevel, appearance, langs, lang, uiLayout, userData, account, accessAuthCode, system, keymap, ai, bazaar, api, publish, openHelp, showChangelog, cloudRegion |
| Store preferences (3) | sync, stat, repo |
| Workspace preferences (8) | fileTree, tag, editor, export, graph, search, flashcard, snippet |
| Runtime values (3) | readonly, localIPs, dataIndexState |

Nested exceptions: system identity/version/platform/container and physical paths
are runtime values; fileTree's automatic selection/current-tab/max-open-tabs/
close-tabs controls and editor font/width/RTL controls are device preferences;
editor.readOnly is a runtime view value. Remaining nested fields inherit the
parent. Non-serialized User and single-view fields stay runtime/session-owned.
Classification is not write authority: imported system paths/readonly flags
cannot redirect IO or grant a source write. Device credentials must not be
archived into the shared source merely because they occur in the old full shape.

This is an ownership contract, not an automatic file migration. Existing legacy
conf.json remains where its explicit AppConf owner bound it. Future persistent
device/store/page settings services must preserve the original full UI projection
while writing only their own category through the correct transaction boundary.

## Explicit inputs and snapshots

Device detection and AI environment overrides happen only at the original boot
adapter, which passes inputs to NewSystemForDevice/NewAIWithEnvironment.
NewEditorWithMarkdown and NewFileTreeWithSettings preserve boot/reset semantics
without fetching util globals. Repository SaveDir requires an explicit nonempty
workspace and rejects a filesystem-root target before destructive repo callers.

WorkspaceManager.Options.Configuration supplies settings for an explicit
(storeId,pageId); acquisition deep-clones them before publishing a Context.
Lease.Configuration returns another detached copy. A missing provider gives an
empty snapshot, not implicit defaults or file creation. AppData.ParserOptions is
shared by original model.NewLute and Context WithConfiguredParser; missing parser
settings are an error, not a fallback to the active workspace.

## Original business dependencies (R02 partial)

`Business` owns a source-bound live notebook reader, copied language catalogs and
explicit regional account endpoints. AppConf's Box/GetBox/BoxNames/GetBoxes/
GetOpenedBoxes/GetClosedBoxes and Language/language now consume this service.
The original compact GetBoxes projection and full opened/closed fields are
preserved. Masked() serializes its receiver and cannot transfer private authority.
Detached configurations have no implicit reader; errors are available through
Business.Notebooks and Context methods. Only compatibility boot explicitly binds
the legacy ListNotebooks adapter, which still has legacy repair/global effects.

Options.ConfigurationBusiness binds the same service to a Context; notebook and
language access runs inside its lease/source admission. Providers must be bounded,
identity-bound and must not reenter the manager or fall back to process globals.
Shared store settings are persisted; full original settings controllers and scoped
notebook repair remain pending. Never attach the compatibility reader to a new
multi-workspace handler.

### Owner-local duration labels

`NewBusiness` accepts an optional fourth `map[string]map[string]string` argument
containing the original language catalogs' `_time` labels. It copies both maps.
`Business.Duration(lang)` snapshots and validates the complete original threshold
table, returning a formatter or `ErrBusinessUnbound`; it does not select device
language, read files, install defaults or fall back to a global language map.
Providers must explicitly bind catalogs for the configured owner language.
The compatibility `util.HumanizeRelTime/HumanizeTime` magnitude helper shares
`conf.RelativeMagnitudes`, preserving original thresholds and labels. Context AV
field/group/rollup range calculations use the same formatter. Missing labels fail
when duration is needed; unrelated reads remain available.

## Original SQL progress (R02 partial)

SQLProgressMessage contains the original five message formatting bodies with
explicit language/policy inputs: block insertion (90), delete (93), rename (234),
history (191), and asset content (217). Mobile suppression only applies to the
last three, preserving original insert/delete switches. Legacy subscribers keep
process boot/message sinks; Context subscribers use this owner's languages and
an explicit sink, never the active global Conf or WS route. The full Context
block-insert producer and remaining original event bodies still need migration.
