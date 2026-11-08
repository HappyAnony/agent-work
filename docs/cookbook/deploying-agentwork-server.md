---
description: "Deploy one AgentWork backend on Linux or macOS and connect Web, mobile browsers and a Mac client with local frontend resources."
---

# Deploy an AgentWork server

English | [中文](deploying-agentwork-server.zh.md)

## Summary

Run the Agent, tools, Wiki kernel and data on one Linux or macOS server. Web and mobile browsers load the server frontend; the optional Mac distribution loads matching frontend resources from its application bundle. This tutorial uses a private SSH tunnel. HTTPS ingress is a separate deployment step.

For immutable server packages, matching Mac/iOS builds and GitHub Actions, follow [Build and release AgentWork](releasing-agentwork.md). The checkout-based commands below remain the source-development path.

## Table of Contents

- [Prepare the server](#prepare-the-server)
- [Connect through SSH](#connect-through-ssh)
- [Package the Mac client](#package-the-mac-client)
- [Install the iOS client](#install-the-ios-client)
- [Verify the workflow](#verify-the-workflow)
- [Operation and limits](#operation-and-limits)

-----

<a id="prepare-the-server"></a>
## Prepare the server

Start with a built DSH checkout, Node 24.9 or newer, pnpm, the Go toolchain selected by the native module, and a C compiler. Follow [development setup](../development.md) and the [Wiki runtime build](../../native/wikiman/README.md). Compile native binaries on each server platform. Keep the Web, plugin and Wiki frontend artifacts identical to those used for the Mac package.

Stop the writer of an existing portable library before copying it to the server. Preserve the original and choose separate directories for the DSH home, the copied library and the Agent workspace. A portable library contains `store.json`; import an older `page.db` library through [Library tools](../../packages/experimental/wikiman/README.md#import-a-legacy-library) first. Do not copy platform-native `node_modules` or macOS AppleDouble `._*` files between machines.

Run the preparation utility from the repository root. Replace the three data paths with absolute server paths. It creates a new profile, preserves existing profiles, mounts portable WikiMan, disables canonical Session-log upload and selects the server directory browser. Model requests still send their selected context to the configured provider.

```sh
pnpm exec tsx scripts/prepare-agentwork-server.ts \
  --home /absolute/server-home \
  --store /absolute/portable-library \
  --workspace /absolute/workspace
DSH_HOME=/absolute/server-home pnpm dsh --profile agentwork-server \
  --host 127.0.0.1 --port 18681 --no-open
```

The terminal prints a launch URL containing an owner access token. Retain it privately. Restarting the server requires the token from the new launch link; open **Settings → Service connection** and enter the current token if the saved cookie is refused. The server stays on loopback; its Wiki kernel also stays private. The home owns model credentials, Sessions and settings. The library directory owns knowledge and attachments; the workspace owns Agent files. Back up all three.

<a id="connect-through-ssh"></a>
## Connect through SSH

On the Mac, forward a loopback port to the server listener. Replace the SSH destination with your server account. Keep this connection open while using Web or the Mac client.

```sh
ssh -N -o ExitOnForwardFailure=yes -o ServerAliveInterval=30 \
  -L 127.0.0.1:18680:127.0.0.1:18681 user@server
```

For Web, replace the authority of the server launch URL with `127.0.0.1:18680`, keeping its token. For the Mac client, enter `http://127.0.0.1:18680` and that token in **Settings → Service connection**. Authentication exchanges it for a session cookie. The Mac saves its cookie in owner-only files; see [client credential storage](../upgrade-guide/v0.2.2-rc.1/agentwork-client-credentials/guide.md) for locations, protection and upgrade steps. Selecting another server requires that server’s token.

Connect every device to the same backend to share changes. A Linux deployment and a Mac deployment with copied libraries remain independent stores. Use **Settings → Service connection** to confirm the selected server address. When comparing updates, also select the same Wiki space; separate spaces can have identical names. Creating a notebook updates the other connected clients’ document trees automatically.

Add the server workspace through the in-page directory chooser. Select the exact `workspaceRoot` configured by the preparation utility: Wiki tools are available only to Sessions created in that directory. Configure model API keys through **Settings → Models**; AgentWork hides DeepSeek account sign-in and automatic credential onboarding. These settings affect the selected server.

<a id="package-the-mac-client"></a>
## Package the Mac client

On macOS, assemble a new frontend directory after the DSH and Wiki builds. The directory includes browser bundles, Chinese resources and license notices, without server data or backend executables. Existing output directories are preserved.

```sh
pnpm exec tsx apps/desktop/scripts/prepare-remote-frontend.ts \
  --out "$PWD/.artifacts/remote-frontend"
DSH_DESKTOP_UNSIGNED=1 \
DSH_REMOTE_FRONTEND_DIR="$PWD/.artifacts/remote-frontend" \
pnpm --dir apps/desktop exec electron-builder \
  --config scripts/electron-builder-remote.config.mjs \
  --mac --arm64 --publish never
```

This command produces a personal, ad-hoc-signed build; distribution with Apple signing and notarization follows the ordinary [packaging guide](../../apps/desktop/packaging/README.md). The client opens its packaged interface immediately and verifies the server’s index, plugin entry and Wiki build fingerprints before enabling backend features. Rebuild the Mac package after changing those assets or adding a client plugin. A version mismatch appears in **Settings → Service connection** and prevents backend activation. The Mac uses its saved address, or the build composition’s loopback default (`http://127.0.0.1:18686`) when no address is saved. Failure never selects another server automatically.

<a id="install-the-ios-client"></a>
## Install the iOS client

The [iOS application](../../apps/ios/README.md) packages the same frontend locally and uses the selected HTTPS backend. Follow its Xcode build and Personal Team installation instructions. The phone needs a directly reachable LAN or VPN HTTPS address; a loopback SSH tunnel on the Mac is insufficient.

<a id="verify-the-workflow"></a>
## Verify the workflow

Use a disposable document for acceptance.

1. Open **Knowledge base**, select a `doc` or a document inside a `wiki`, and enter a unique marker.
2. Choose **Page actions → Save and close**, reopen it, and confirm the marker.
3. Create a Session in the configured server workspace and ask the Agent to read that marker. Expand tool activity and confirm `wiki_read` returned the new text.
4. Choose **Open Wiki source** and confirm the original document contains the marker.
5. Repeat in a mobile browser or a narrow touch viewport. Close the Mac client and confirm the server remains reachable from Web.

Recorded model output can verify actual Agent tool execution and persistence without a provider key. It does not verify a live model’s reasoning or provider connectivity.

<a id="operation-and-limits"></a>
## Operation and limits

One trusted owner uses this server from multiple devices. The deployment does not provide organization accounts, multi-user authorization, offline editing or simultaneous collaborative editing of one block. Revision checks reject stale writes. Close editor views before library maintenance; a lost connection revokes its views.

A phone cannot use the Mac’s loopback SSH tunnel directly. For phone access, configure a trusted root HTTPS origin reachable on the company network or VPN, with WebSocket forwarding and the same DSH authentication. The Mac accepts remote HTTPS addresses and loopback HTTP only. No HTTPS ingress or physical-phone acceptance is implied by an SSH-only deployment.

For a private certificate authority, the device owner must install and trust its root certificate on each device. The Mac client reads operating-system trust roots at startup and retains certificate-chain, expiry and hostname verification. Restart the client after changing trust settings. On macOS, import the root into the login or System keychain, enable SSL trust and save the authorization. On iOS, enable full root trust after installing the certificate profile. Trust on one device does not configure another device.

The remote client has no local shell, local Agent filesystem or local backend. Directory choices refer to the server. The server owns lifecycle and restart policy; stopping it interrupts its tasks. Keep each portable store under one kernel owner. Native PDF export currently requires the macOS helper; Linux returns converter unavailable. Word export requires a trusted platform Pandoc helper.

## Dev Note

None.
