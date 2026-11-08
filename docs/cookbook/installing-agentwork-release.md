---
description: "Install downloaded AgentWork server, Mac and iOS Simulator packages, then connect Web or mobile clients."
---

# Install downloaded AgentWork releases

English | [中文](installing-agentwork-release.zh.md)

## Summary

Start one Linux or Mac server, then connect Web, Mac or iOS. Server packages contain Node, the Wiki kernel and both Web and Wiki frontends; the deployment machine needs no Node, Go or pnpm installation. Mac/iOS apps provide the client only; matching releases share the backend.

## Table of Contents

- [Choose and verify downloads](#downloads)
- [Install the server](#server)
- [Connect other devices](#network)
- [Install the Mac app](#mac)
- [Install the iOS app](#ios)
- [Verify and upgrade](#verification)
- [Dev Note](#dev-note)

<a id="downloads"></a>
## Choose and verify downloads

Linux x64 servers require x86_64 CPUs and GNU libc 2.35 or newer; confined shell execution also requires a kernel with Landlock enabled. Mac ARM64 servers and apps require Apple Silicon and macOS 15 or newer. The iOS Simulator package requires a Mac, Xcode and a running compatible simulator; the current minimum iOS version is 18.4. Do not mix versions or architectures.

Download matching packages listed at the top of the release README. Most users need one server and their selected clients; native packages already include the frontend. GitHub's automatic Source code downloads contain source, not executable programs. With all owned assets and SHA256SUMS in one directory, use `shasum -a 256 -c SHA256SUMS` on Mac or `sha256sum -c SHA256SUMS` on Linux. Partial downloads produce missing-file reports for other assets; at least compare the selected file's digest, and do not treat missing files as a successful whole-release check.

<a id="server"></a>
## Install the server

1. Extract the selected server archive into a new program directory and enter it; preserve running versions. The commands below assume artifact.json and payload are in the current directory. Keep configuration and knowledge data outside the program directory.

```sh
PROGRAM="$(pwd)/payload/server"
mkdir -p "$HOME/AgentWork/data"
cd "$HOME/AgentWork/data"
umask 077
```

2. For a first deployment, create an empty library and configuration. These commands refuse to overwrite an existing library or server.json. For an existing portable library, back it up, skip initialization, and write a separate configuration pointing library at it. Do not directly reuse a source-mode home; the packaged server needs its own home/profile.

```sh
"$PROGRAM/bin/node" --input-type=module <<'NODE'
import { mkdir, writeFile } from 'node:fs/promises'
import { resolve, join } from 'node:path'
import { randomUUID } from 'node:crypto'
const library = resolve('library')
await mkdir(library, { mode: 0o700 })
await mkdir(join(library, 'pages'), { mode: 0o700 })
await writeFile(join(library, 'store.json'), JSON.stringify({
  schemaVersion: 1, layoutVersion: 1, documentFormat: 'docj',
  storeId: randomUUID(), name: 'AgentWork',
}) + '\n', { flag: 'wx', mode: 0o600 })
await writeFile('server.json', JSON.stringify({
  home: resolve('home'), library, workspace: resolve('workspace'),
  host: '127.0.0.1', port: 18686,
}, null, 2) + '\n', { flag: 'wx', mode: 0o600 })
NODE
```

3. Check the configuration and start the server in the foreground. Keep the terminal open; use Ctrl+C to stop normally. The server uses a named dsh profile. Never run multiple instances against one home or library.

```sh
"$PROGRAM/bin/node" "$PROGRAM/start.mjs" --config "$HOME/AgentWork/data/server.json" --check
"$PROGRAM/bin/node" "$PROGRAM/start.mjs" --config "$HOME/AgentWork/data/server.json"
```

4. Open the complete Web link printed by the terminal, including its first-authentication token. A bare URL reports that Web authentication is required. In the Mac app's settings, enter the root server address and the token parameter's value; a successful connection saves its session credential. Do not share tokens or private logs or add them to Git.

For persistent operation, run the same start command through systemd on Linux or launchd on macOS, using absolute program/configuration paths. The working directory, configuration, logs and three data directories should belong to the same service account. Terminal exit or one successful --check does not establish a running background service.

<a id="network"></a>
## Connect other devices

The default listener accepts only local connections. A remote computer can first run `ssh -N -L 18686:127.0.0.1:18686 user@server`, then connect through `http://127.0.0.1:18686`, still using the printed token for first authentication. This tunnel serves only the computer that opens it; 127.0.0.1 on a phone refers to the phone itself.

Phones and other intranet/VPN devices need a reachable HTTPS address. Configure a company HTTPS reverse proxy, or set the real bind IP plus absolute tlsCert/tlsKey paths in server.json; configure publicUrl and trustedHosts accordingly. Every device must trust the certificate and its hostname/expiry must match. Keep certificate verification enabled and tokens out of public links. Replace the example fields with real addresses and certificate paths, retaining home/library/workspace.

```json
{
  "host": "192.168.1.6",
  "port": 18686,
  "publicUrl": "https://agentwork.example.internal:18686",
  "trustedHosts": ["agentwork.example.internal:18686"],
  "tlsCert": "/absolute/certs/fullchain.pem",
  "tlsKey": "/absolute/certs/private-key.pem"
}
```

<a id="mac"></a>
## Install the Mac app

Extract mac-arm64, quit the previous AgentWork normally, then copy payload/mac-arm64/AgentWork.app to Applications. This prerelease uses an ad-hoc development signature, without Developer ID/notarization. After checking the download source and digest, a user whose Mac blocks launch can inspect System Settings → Privacy & Security and choose Open Anyway; do not disable Gatekeeper globally.

The app opens the workbench immediately. In Settings → Service connection, enter the root address and initial access token; Mac attempts the local server by default. Mac stores the address and expiring session credential under ~/.agent-work/client/. Upgrading from Keychain storage requires entering the token once, then reconnection is automatic. Other programs running as the same OS user may read the credential file; exclude it from shared backups. Configure model API keys in Settings → Models after connecting.

<a id="ios"></a>
## Install the iOS app

ios-simulator runs only in Simulator and cannot be installed on an iPhone. Start an iOS Simulator through Xcode, extract the archive and install it with the command below; open AgentWork from the simulator home screen.

```sh
xcrun simctl install booted /absolute/unpacked/payload/AgentWork-simulator.app
```

Installing on an iPhone with a personal Apple account requires building the matching source on your own Mac and signing through Xcode. Check out this release tag, install Node/pnpm using .node-version and packageManager, then run pnpm install --frozen-lockfile. Build frontend through the unified agentwork build command; pass that same artifact through --frontend to ios-device with --signing development-device and AGENTWORK_APPLE_TEAM. Alternatively run prepare:frontend and sync under apps/ios, open apps/ios/ios/App/App.xcodeproj in Xcode, select your Personal Team and connected iPhone, and Run. The device owner must handle Apple authorization, Developer Mode and developer-app trust in system UI. Personal Team profiles generally expire after seven days and require signing again; this is not a publicly installable IPA for arbitrary phones.

Allow local-network access when the iPhone first opens the app, then enter a reachable HTTPS root address and token in Settings → Service connection. iOS retains its existing device-Keychain credential storage. General public device distribution requires separately configured Apple Developer Program membership and TestFlight or App Store; the current release cohort does not promise that installation method.

<a id="verification"></a>
## Verify and upgrade

Create or edit a knowledge document, wait for saving, then open it on another client and confirm the new content. Ask the Agent to read it and open the citation back to the source; restart the client to check automatic connection. Configure an API key under Settings → Models if no model is available. Knowledge editing currently requires a live connection; offline writes and simultaneous editing of one block are unsupported.

Before upgrading, save edits, stop the old server and back up home, library, workspace and private configuration. Extract the new program into a new directory, start a compatible version with the same private configuration, and update clients to matching resources. On failure, retain logs and data; never delete the library, force file replacement or run two services against it. Program rollback does not restore data; incompatible data formats require a verified backup.

<a id="dev-note"></a>
## Dev Note

This tutorial accompanies complete releases as downloadable README.md (Chinese) and README.en.md. Their digests are included in SHA256SUMS. Download tables come from verified target inventories; simulator archives must never be labeled as phone installation packages.
