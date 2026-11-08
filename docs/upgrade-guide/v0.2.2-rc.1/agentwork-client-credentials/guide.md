---
kind: upgrade-guide
description: "AgentWork Mac connections move to owner-only files and require reauthentication after upgrading from Keychain storage."
---

# Reconnect AgentWork Mac with file credentials

English | [中文](guide.zh.md)

## Change

The remote AgentWork Mac client stores its server selection in `~/.agent-work/client/connection.json` and its authenticated session cookie in `credentials.json` beside it. The directory requires mode `0700`; both files require mode `0600` and must belong to the current user. Links, unsupported formats and unsafe permissions are rejected. The cookie is an access credential readable by other programs running as that user; this storage does not provide Keychain's application isolation. Exclude `credentials.json` from shared or unprotected backups.

The Mac client migrates only the selected server address from Electron's previous `remote-server.json`. It does not decrypt `encryptedCookie` or read Keychain. The original settings and Keychain entry remain intact. The first connection requires the server's launch token again; the client exchanges it for an expiring, origin-bound cookie without persisting the launch token. Missing or expired credentials require authentication. Windows and iOS keep their existing storage implementations.

## Migration

1. Install matching client and server resources using the [build workflow](../../../cookbook/releasing-agentwork.md). Quit the previous Mac client normally before replacing it.
2. Open **Settings → Service connection**, confirm the migrated server address, enter its current launch token, and connect. Enter the token only in the connection form.
3. Quit and reopen the app. Confirm that the same server reconnects without a Keychain prompt. Selecting another origin requires its own valid credential.
4. If Settings reports a local credential-storage failure, check the client directory and files for ownership, permissions and free space. Back up damaged or unsupported files privately before moving them out of the client directory, then reopen the app and configure the connection again. Do not copy server tokens or cookies into logs, Git or release assets.
