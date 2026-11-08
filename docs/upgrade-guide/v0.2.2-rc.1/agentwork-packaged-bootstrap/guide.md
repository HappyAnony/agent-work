---
kind: upgrade-guide
description: "AgentWork native clients require a packaged local bootstrap and configure server connections in Settings."
---

# Rebuild AgentWork clients with the local bootstrap

English | [中文](guide.zh.md)

## Change

AgentWork Mac and iOS clients open their packaged interface before server authentication. Server address and access token configuration moves from the startup form to **Settings → Service connection**. DeepSeek sign-in and automatic credential onboarding are hidden; model API keys remain in **Settings → Models**. Mac credential migration requires [reauthentication with file storage](../agentwork-client-credentials/guide.md); iOS retains its saved credentials.

Prepared frontend directories require `bootstrap.json` alongside `manifest.json`. A new carrier cannot use a directory prepared by the previous packager. The ordinary DSH Desktop distribution and served Web authentication remain unchanged.

## Migration

1. Build matching DSH and Wiki resources as described in [server deployment](../../../cookbook/deploying-agentwork-server.md#package-the-mac-client).
2. Prepare a new frontend directory with `apps/desktop/scripts/prepare-remote-frontend.ts` and rebuild the Mac client. For iOS, run `pnpm --filter @deepseek-ai/dsh-ios prepare:frontend` and `pnpm --filter @deepseek-ai/dsh-ios sync`, then rebuild in Xcode. Keep native and server resources at the same build.
3. Open the app with the server stopped. Confirm that the interface and Settings remain accessible and Service connection indicates the failure. Start the selected server and retry; confirm that the knowledge editor opens. If authentication is refused, enter the token from the current server launch link in Settings.
