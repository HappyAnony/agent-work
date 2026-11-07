---
kind: upgrade-guide
description: "WikiMan Web editor responses use a document-relative API base instead of the private kernel address."
---

# Resolve WikiMan editor URLs against the document

English | [中文](guide.zh.md)

## Change

In Web compositions, `WikiEditorView.baseUrl` now contains `/api/wikiman` instead of a private kernel loopback URL. Custom portable editor consumers that assume an absolute URL must resolve this value against their document. Non-Web consumers retain the private loopback URL.

## Migration

1. Construct the editor API URL with `new URL(editor.baseUrl, location.href)`. Preserve DSH authentication for same-origin requests and continue sending the page-scoped editor ticket.
2. Load portable application assets from the serving DSH origin. A packaged remote client supplies these assets locally and forwards content requests to the selected server.
3. Reopen a document, save a unique marker, ask the Agent to read it in the configured workspace, and open the read result’s original-source citation. Follow the [deployment tutorial](../../../cookbook/deploying-agentwork-server.md) for remote clients.
