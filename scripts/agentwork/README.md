---
description: "AgentWork build and release command fields, artifact guarantees, failure handling and extension responsibilities."
---

# AgentWork build and release reference

English | [中文](README.zh.md)

## Summary

Use `pnpm --silent agentwork <command>` from a checkout to build and verify matching server/client distributions. Commands reserve new outputs, preserve existing versions and return one JSON result on stdout; child logs use stderr. The [operator tutorial](../../docs/cookbook/releasing-agentwork.md) owns the ordered procedure and the [Agent skill](../../.agents/skills/agentwork-release/SKILL.md) owns continuation and authorization handling.

## Table of Contents

- [Commands and reports](#commands)
- [Artifact guarantees](#artifacts)
- [Developer responsibilities](#responsibilities)
- [Validation](#validation)
- [Dev Note](#dev-note)

-----

<a id="commands"></a>
## Commands and reports

| Command | Inputs and effect |
| --- | --- |
| `help` / `inspect` | List operations / read source, lockfile, tools and persistence identity without building. |
| `build` | `--target`, `--out`, optional `--frontend`, `--release-id`, `--allow-dirty`, `--signing`, `--timeout-ms`; produce a new complete artifact. |
| `verify` | `--input`, optional `--release`; verify one artifact or a complete release without activation. |
| `pack` / `unpack` | `--input`, `--out`; transport one artifact with modes and contained links preserved. |
| `release` | Repeated `--artifact`, `--out`, optional `--require` comma-list and `--timeout-ms`; assemble local archives. Public publication still requires the full product cohort. |
| `collect` | `--input` directory of target `.tar.gz` archives, `--out`, optional `--timeout-ms`; import and assemble the full cohort. |
| `publish` | `--input`, `--repository`, `--expected-commit`, optional `--mode draft\|publish` (default draft); verify assets and explicitly mutate GitHub Release. |
| `run` | `--repository`, `--id`, `--attempt`; validate a successful AgentWork build on develop, including its exact attempt. |
| `select` | `--input` release directory, `--target`, `--out`, `--expected-commit`; import one source-bound target. |
| `runner` | `--labels` JSON, `--root`, `--config`; validate controlled deployment-runner configuration without queuing a job. |
| `smoke` | `--input` server artifact, `--fixture` portable fixture, optional `--timeout-ms`; boot an isolated named profile without model/GitHub credentials and compare authenticated served Web bytes. |
| `deploy` / `rollback` | `--input` server artifact, `--root`, `--config`, optional `--dry-run`; select a compatible program for a stopped service and preserve private backups. |

`--timeout-ms` defaults to 30 minutes per build/archive subprocess. Cancellation terminates the owned process tree and waits for it before output cleanup. New incomplete build outputs are removed; existing outputs are not removed. Network, server or unreadable publication responses, and unknown deployment outcomes, retain drafts, program versions or private receipts for inspection rather than claiming rollback.

Success reports use `{ok:true,schemaVersion:1,operation,result}`. Failures use `{ok:false,schemaVersion:1,operation,error:{code,message}}`. Exit `0` means that operation completed, `2` means invalid arguments/input, `130` means cancellation, and `1` means another failure. A successful build is not a published release; a successful deployment report explicitly leaves `service:stopped` and provides the profile start command. Use the result fields, not diagnostic wording, for Agent decisions.

<a id="artifacts"></a>
## Artifact guarantees

An artifact root contains exactly `artifact.json` and `payload/`. Its manifest records the product version/release ID, commit, dirty state, source fingerprint, lockfile digest, tools, target OS/CPU, protocol/store/persistence versions, shared-resource identity, signing state and complete payload inventory. Files carry SHA-256, size and executable mode; links must resolve inside the payload. Missing, additional, modified or escaping entries fail verification. Archives are checked before extraction; native binaries are checked as ELF or Mach-O for their declared CPU.

The frontend includes Web, client plugins/chunks/assets, Wiki stage/appearance/languages and license notices. Native builders consume that artifact, and release validation compares embedded shared files with its inventory. Server packages import Web and installed plugin bytes into their actual runtime package locations; the Wiki kernel uses the imported Wiki resources. Mac reads its bundled frontend; iOS keeps the original Web index separately from its native carrier. Runtime-generated authenticated injections are not copied from a static server snapshot.

The server closure in [packaging/agentwork-server](../../packaging/agentwork-server/README.md) compiles Host libraries and GUI Node adapters on the target platform, installs production dependencies, materializes package links and includes Node and Wiki binaries. The `node-adapters` build pass emits no browser bundles; browser assets are imported from the shared artifact. Its generated CLI resolution includes WikiMan, while the source DSH default distribution stays independent. The private startup adapter launches the normal `dsh --profile agentwork-server` path. Configuration, library, home, workspace and backups are not release assets.

`release.json` lists required targets, archives, checksums, artifact-manifest digests and signing states; `SHA256SUMS` also covers the release inventory. The default required cohort is defined by `requiredTargets` in [release.ts](release.ts) and checked against the Actions matrix. Optional Linux ARM64 and Intel Mac targets require native builders; the default hosted matrix does not claim those additional acceptance results.

<a id="responsibilities"></a>
## Developer responsibilities

| Change | Owning code | Required release impact |
| --- | --- | --- |
| Generic Agent tool or external integration | Independent capability package under `packages/` | Test its real composition; rebuild the server, and shared frontend/native clients when it contributes client code. |
| Knowledge business, search, import, sync and references | [WikiMan package](../../packages/experimental/wikiman/README.md) | Verify save → Agent reads committed revision → citation opens source; keep source-format migrations explicit. |
| Native document transactions and format | [wiki-kernel](../../native/wikiman/README.md) | Rebuild both server platforms; update compatibility metadata and review data migration/rollback. |
| Rich text, block interactions, Wiki resources | `native/wikiman/wiki-app/` | Rebuild the shared frontend and all consumers; verify languages/assets and embedded resource comparisons. |
| Navigation, settings, responsive client pages | Owning `packages/client/` plugin or WikiMan client | Follow UI/locale rules; rebuild shared resources and native clients; verify affected rendered screens. |
| Native transport, Keychain and OS integration | `apps/desktop/` or `apps/ios/` | Rebuild that carrier; distinguish simulator/device and development/production signing evidence. |
| Artifact format, target adapter and publication | This directory | Update schema/readers, negative cases, Actions matrix and both human/Agent documentation together. |
| Runner/event/job orchestration | `.github/workflows/agentwork-*.yml` | Call the same CLI; preserve repository/source/attempt checks and least permissions. |

Keep orchestration out of the Agent loop, Wiki source out of deployment scripts, and credentials out of manifests. Resolve configuration in its owning component. New format fields must update every producer and consumer; incompatible persistence changes follow the repository's migration and upgrade-guide rules. Adding a platform also updates the native build environment, required/optional target policy, binary validation, tests and acceptance matrix. Formal WikiMan package migration remains tracked separately in the [TODO](../../.agents/plans/agentwork/TODO.md); scripts discover it by package name rather than fixing its experimental directory into the public CLI.

<a id="validation"></a>
## Validation

```sh
pnpm exec vitest run scripts/agentwork --maxWorkers=2 --testTimeout=60000 --hookTimeout=60000
pnpm run build
pnpm run test:docs
```

Unit fixtures cover valid and invalid manifests, native headers, embedded bytes, archives, failed/queued/cancelled run selection, resumable publication, stopped-service guards and data-version refusal. They do not establish a successful cloud run, signed production distribution, physical-phone installation or live deployment health. Record those executions, actual commands and remaining blockers in the TODO.

<a id="dev-note"></a>
## Dev Note

None.
