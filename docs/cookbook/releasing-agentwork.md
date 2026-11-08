---
description: "Build matching AgentWork server and native client artifacts, collect a release, and use GitHub Actions from develop."
---

# Build and release AgentWork

English | [中文](releasing-agentwork.zh.md)

## Summary

Build the shared frontend once, build each native target on its own platform, and verify the complete release before distributing it. The same commands serve local operators, [Agents](../../.agents/skills/agentwork-release/SKILL.md) and GitHub Actions. This tutorial covers development distributions and explicit publication; Apple production credentials and a private deployment runner require separate configuration.

## Table of Contents

- [Prepare a fixed checkout](#prepare)
- [Build shared resources](#frontend)
- [Build native targets](#native)
- [Collect and publish](#release)
- [Use GitHub Actions](#actions)
- [Deploy a stopped server](#deploy)
- [Recover from failure](#recover)
- [Dev Note](#dev-note)

-----

<a id="prepare"></a>
## Prepare a fixed checkout

Use `develop`, the exact Node version in [.node-version](../../.node-version), pnpm from the root `packageManager`, and a C/C++ compiler. Linux server builders also need `musl-gcc` (`musl-tools` on Ubuntu) for the static Landlock launcher and the complete native package; an addon-only source test build is insufficient. Native Go builds select the version in [go.mod](../../native/wikiman/wiki-kernel/go.mod). Mac and iOS builders require macOS and full Xcode; set `DEVELOPER_DIR` to its `Contents/Developer` directory if Command Line Tools is selected. CI selects Xcode 26.3. The Mac distribution targets macOS 15 or newer; iOS requires 18.4 or newer.

Choose and commit the product version using the repository's version policy. The root, CLI, Mac, iOS, server closure and WikiMan manifests must agree. The existing [version updater](../../scripts/release/bump.ts) owns the repository's shared version sequence; AgentWork publication uses separate `agentwork-v<version>` tags and does not publish npm packages or upload to COS.

```sh
pnpm install --frozen-lockfile
pnpm --silent agentwork inspect
```

Official builds require a clean working tree. `--allow-dirty` permits an explicitly marked development build; release collection rejects it. Preserve existing work rather than resetting the checkout. Use new output directories for every build. Keep credentials, signing files and all user data outside outputs.

<a id="frontend"></a>
## Build shared resources

Run these commands from the repository root. They compile DSH and Wiki frontend resources, include language/font/plugin chunks and notices, and verify every payload file. The archive preserves executable modes and contained links during Actions upload.

```sh
pnpm --silent agentwork build --target frontend --out .artifacts/agentwork/frontend
pnpm --silent agentwork verify --input .artifacts/agentwork/frontend
pnpm --silent agentwork pack --input .artifacts/agentwork/frontend --out .artifacts/agentwork/frontend.tar.gz
```

Copy that archive to other builders and check out the same source commit and lockfile there. Import it into a new directory with `pnpm --silent agentwork unpack --input /absolute/frontend.tar.gz --out /absolute/frontend`. All native commands below consume that imported artifact through `--frontend`; they do not rebuild its frontend bytes.

<a id="native"></a>
## Build native targets

On the Linux x64 builder, use the imported frontend and a new output:

```sh
pnpm --silent agentwork build --target server-linux-x64 --frontend /absolute/frontend --out .artifacts/agentwork/server-linux-x64
pnpm --silent agentwork smoke --input .artifacts/agentwork/server-linux-x64 --fixture packages/experimental/wikiman/tests/fixtures/portable
pnpm --silent agentwork pack --input .artifacts/agentwork/server-linux-x64 --out .artifacts/agentwork/server-linux-x64.tar.gz
```

The CI Linux baseline is Ubuntu 22.04. Packages include Node and platform-native dependencies; the packaged launcher checks that GNU libc is at least the build baseline. Validate on the deployment distribution too. Build `server-linux-arm64` on ARM64 rather than copying another machine's native dependencies.

On the Mac ARM64 builder, use the same shared frontend:

```sh
pnpm --silent agentwork build --target server-darwin-arm64 --frontend .artifacts/agentwork/frontend --out .artifacts/agentwork/server-darwin-arm64
pnpm --silent agentwork build --target mac-arm64 --frontend .artifacts/agentwork/frontend --out .artifacts/agentwork/mac-arm64
pnpm --silent agentwork build --target ios-simulator --frontend .artifacts/agentwork/frontend --out .artifacts/agentwork/ios-simulator
```

The Mac target produces `payload/mac-arm64/AgentWork.app` with an ad-hoc development signature. Copy the app to Applications for personal use; it has no Developer ID/notarization guarantee. For a production Mac build, select `--signing developer-id` and configure the certificate, expected identity/Team ID and Apple notarization credentials described in the [packaging guide](../../apps/desktop/packaging/README.md). Verification must succeed; missing credentials never select an unsigned fallback. Intel Macs use the `mac-x64` and `server-darwin-x64` targets on an Intel builder.

The simulator artifact is `payload/AgentWork-simulator.app`, not an installable iPhone IPA. A device build uses `--target ios-device --signing development-device` and `AGENTWORK_APPLE_TEAM`; configure your account, certificate and profile in Xcode first. Personal Team installation and renewal remain the [device owner's workflow](../../apps/ios/README.md#build-and-install). The script does not install on a phone or export a public Personal Team distribution.

Archive each target using `agentwork pack`, naming the archives after their targets. Place the five archives—frontend, Linux x64 server, Mac ARM64 server, Mac ARM64 client and iOS simulator—in a new directory such as `.artifacts/agentwork/cohort`. Optional architectures can accompany that complete cohort.

<a id="release"></a>
## Collect and publish

```sh
pnpm --silent agentwork collect --input .artifacts/agentwork/cohort --out .artifacts/agentwork/release
pnpm --silent agentwork verify --release --input .artifacts/agentwork/release
```

Collection rejects missing targets, changed archives, mixed source/lockfile/protocol/data versions and embedded frontend resources that differ from the shared artifact. The release directory contains target archives, `release.json` and `SHA256SUMS`. Existing output and version directories are preserved. Mac/iOS signing states remain visible in the release inventory.

Publication is an explicit operation. Authenticate `gh`, or set `GH_TOKEN` privately. Obtain the expected commit from your selected successful build attempt or the fixed local checkout, then prepare a reviewable draft:

```sh
pnpm --silent agentwork publish --input .artifacts/agentwork/release --repository HappyAnony/agent-work --mode draft --expected-commit "$(git rev-parse HEAD)"
```

After reviewing the draft and assets, use the same command with `--mode publish` when publication is authorized. Stable Mac releases require Developer ID signing and notarization; the current unsigned Mac cohort can be published only as a prerelease. Matching assets are reused on retry; different content under the same version is rejected. Network failures during mutation report `publication-unknown`: inspect the preserved draft before retrying.

<a id="actions"></a>
## Use GitHub Actions

Open [Actions](https://github.com/HappyAnony/agent-work/actions). `AgentWork CI` checks pushes and PRs into `develop`. `AgentWork build` runs on `develop` pushes (except plan-only changes) or manual dispatch: one shared frontend job, a hosted native matrix and complete release collection. Intermediate artifacts and reports are retained for seven days. The inherited DSH workflows are guarded in this repository; no upstream runner, COS or organization secret is required.

For manual builds, choose **AgentWork build → Run workflow → develop**. Wait for the entire run to succeed and record both its run ID and attempt number. For a retry, use **Re-run all jobs** so all targets consume the same-attempt frontend. A failed-job-only retry cannot silently reuse a previous attempt's resource artifact.

Choose **AgentWork release → Run workflow**, enter the exact successful build run ID and attempt, and select `draft` first. The workflow validates the build's source/event/attempt, downloads its complete release and invokes the same publisher. Select `publish` only for an authorized public release. This workflow alone receives `contents: write`; ordinary builds remain read-only and have no signing secrets.

With an authenticated GitHub CLI, an Agent or operator can use the same workflow:

```sh
gh workflow run agentwork-build.yml --repo HappyAnony/agent-work --ref develop
gh run list --repo HappyAnony/agent-work --workflow agentwork-build.yml --limit 5
```

For a known run, `gh run watch RUN_ID --repo HappyAnony/agent-work --exit-status` waits for its result. Use `pnpm --silent agentwork run --repository HappyAnony/agent-work --id RUN_ID --attempt ATTEMPT` before selecting release artifacts. Do not infer success from one matrix job or automatically pick an older successful attempt. Queued, cancelled, expired, inaccessible and failed results require the [documented recovery action](#recover).

<a id="deploy"></a>
## Deploy a stopped server

Extract a verified server artifact onto its matching Linux or Mac host. Create a private JSON configuration outside the program directory, substituting absolute paths:

```json
{"home":"/absolute/agentwork-home","library":"/absolute/portable-library","workspace":"/absolute/agent-workspace","host":"127.0.0.1","port":18686}
```

The portable library must already exist and contain valid `store.json`. The three data directories must be distinct. Back up and migrate an existing source-based home separately; a packaged server owns its own `agentwork-server` profile and refuses an unmanaged profile. The optional keys `tlsCert`, `tlsKey`, `publicUrl` and `trustedHosts` configure the existing DSH listener; certificates and keys stay outside release packages. Follow [server connectivity](deploying-agentwork-server.md) for TLS, authentication and device access.

Run `pnpm --silent agentwork deploy --input /absolute/server-artifact --root /absolute/programs --config /absolute/server.json --dry-run` first. Preview creates no directories, locks or backups. An active managed PID, active Wiki owner, pending recovery or incompatible format prevents deployment. Close saved editors and stop the existing server gracefully; the script does not force-stop user tasks.

Repeat without `--dry-run` to back up home/library/workspace privately, copy the verified program into an immutable version directory and atomically select `current`. The result explicitly reports `service: stopped` and a `start` argument array. Start that command under your service manager or terminal, then check Web/Mac/iOS and the [knowledge acceptance loop](deploying-agentwork-server.md#verify-the-workflow). Program selection and a configuration check do not establish live HTTP health or editor acceptance.

For program rollback, run `agentwork rollback` with the previous verified server artifact and the same root/config, previewing first. The service must be stopped; format versions must match. Data restoration is separate and manual. A failed backup has no completed receipt and must not be treated as restorable. Retain old version directories and private backups until recovery is verified.

The manual **AgentWork stopped-service deployment** workflow uses repository variables `AGENTWORK_DEPLOY_RUNNER` (for example `["self-hosted","linux","x64","agentwork-deploy"]`), `AGENTWORK_DEPLOY_ROOT` and `AGENTWORK_DEPLOY_CONFIG`. Configure a dedicated trusted runner on the target or its controlled network; the workflow does not assume hosted runners can reach a private IP. Missing settings fail in hosted preflight before a private job queues. The default mode is read-only `preview`; `deploy`/`rollback` select programs and still leave the service stopped. It never runs PR code on that runner.

<a id="recover"></a>
## Recover from failure

| Report | Safe next action |
| --- | --- |
| Dirty source, mismatched tools or existing output | Preserve work, fix the checkout/toolchain and select a new output; do not clear unrelated files. |
| Missing/tampered/mixed artifact | Download the exact selected attempt again or rebuild the whole cohort; do not edit checksums to suppress failure. |
| Queued/failed/cancelled build | Inspect the run; wait, fix the cause or rerun all jobs. Do not publish a partially successful matrix. |
| Expired artifact or insufficient Actions permissions | Rebuild, or obtain access to the correct run; never fall back silently. |
| Unknown publication outcome | Inspect tag, draft and asset digests, then retry the same verified release. |
| Running/busy server, stale PID or deployment guard | Preserve editors and inspect the owning process/receipt; remove stale guards only after confirming no owner remains. |
| Deployment interrupted or unhealthy restarted service | Keep it stopped, inspect `current` and backup receipts, select a compatible previous program, and verify health before reuse. |
| Changed data format | Follow a reviewed migration or verified backup restore; program rollback does not downgrade data. |

<a id="dev-note"></a>
## Dev Note

The [command reference and development responsibilities](../../scripts/agentwork/README.md) own detailed fields and extension rules. The [TODO](../../.agents/plans/agentwork/TODO.md) records executed acceptance and remaining platform, signing and deployment work; this tutorial is not evidence that a particular remote release or device installation occurred.
