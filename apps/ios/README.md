---
description: "AgentWork iOS packages the DSH and WikiMan interface locally and connects to one Linux or macOS backend."
---

# AgentWork iOS

English | [中文](README.zh.md)

## Summary

The Capacitor application loads packaged DSH, plugin and WikiMan resources in WKWebView at `dsh-app://app`, sharing the Mac editor's origin validation. The selected server owns the Agent, tools, knowledge libraries and saved documents. This reference covers the native client; [server deployment](../../docs/cookbook/deploying-agentwork-server.md) owns backend setup.

## Table of Contents

- [Build and install](#build-and-install)
- [Connect](#connect)
- [Runtime and limits](#runtime-and-limits)
- [Developer checks](#developer-checks)

<a id="build-and-install"></a>
## Build and install

Use a Mac with full Xcode 26 or newer, Node 24.9 or newer and pnpm. The application requires iOS 18.4 or newer. Xcode must support the connected device; verify pairing and platform support before installation.

Select full Xcode as the active developer directory, or set `DEVELOPER_DIR` to its `Contents/Developer` directory before running Apple build tools.

From the repository root, build matching server resources and prepare the native project:

```sh
pnpm install
bash native/wikiman/build.sh
pnpm run build:artifacts
pnpm --filter @deepseek-ai/dsh-ios prepare:frontend
pnpm --filter @deepseek-ai/dsh-ios sync
```

Open `apps/ios/ios/App/App.xcodeproj` in Xcode, select the App target, choose your Personal Team under Signing & Capabilities, connect and trust the iPhone, select it as the destination and run. Apple account entry, agreements, certificate authorization and device Developer Mode require the device owner's action. Personal Team provisioning expires after seven days; rebuild and install again to renew it. A signed application is device-specific; an unsigned IPA cannot be installed by opening it on the phone.

<a id="connect"></a>
## Connect

The app opens its complete packaged interface immediately and attempts the saved server in the background. Open **Settings → Service connection** to enter a trusted root HTTPS server address and the access token from its launch link. The native process exchanges the token for a server cookie, verifies the packaged frontend fingerprints and saves the credential in the device Keychain. Later connections to the same address can leave the token empty. The interface receives neither the cookie nor a local backend. DeepSeek account sign-in and automatic credential onboarding are hidden; configure model API keys under **Settings → Models** after connecting.

The phone must reach the server through its LAN or VPN. A private certificate authority requires an installed profile and full root trust on the phone. Certificate chain, expiry and hostname validation remain enabled. Client and server frontend versions must match; rebuild this app when Web, plugin or Wiki resources change. Connection failure keeps the interface available and marks Service connection as unavailable. Backend-dependent actions offer configuration or retry. A new installation has no default server; the phone uses the manually selected LAN or VPN address.

<a id="runtime-and-limits"></a>
## Runtime and limits

HTTP bodies and WebSocket messages pass through the native authenticated transport; static frontend resources load locally. Returning to the foreground checks live sockets; the shared client owns reconnection. Navigation closes requests from the previous document. Safe area and keyboard clearance belong to the native window.

Backgrounding iOS may suspend the interface. Agent tasks continue on the server, while knowledge edits require a live connection and successful save. Offline editing, background synchronization and concurrent editing of one block are not provided. Use the existing [acceptance workflow](../../docs/cookbook/deploying-agentwork-server.md#verify-the-workflow) to verify saving, Agent reads and original-source navigation on the installed application.

<a id="developer-checks"></a>
## Developer checks

The JavaScript carrier tests build directly from source; the Swift package exercises the native address and resource validation on macOS:

```sh
pnpm exec vitest run apps/ios/tests/carrier.spec.ts
swift test --package-path apps/ios
```

`sync` preserves the fractional iOS deployment target and verifies copied modules outside Capacitor's reserved `plugins` directory. Personal signing settings can live in ignored `ios/App/Signing.local.xcconfig` as `DEVELOPMENT_TEAM = <team id>`. Packaged frontend output, credentials, provisioning profiles and build products are excluded from version control. The application reuses the existing DSH artwork; Wiki resources retain their bundled AGPL notices and source in [native/wikiman](../../native/wikiman/README.md).

## Dev Note

None.
