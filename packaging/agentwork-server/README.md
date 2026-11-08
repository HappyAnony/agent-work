---
description: "Build-only dependency closure for standalone AgentWork server distributions on Linux and macOS."
---

# AgentWork server closure

English | [中文](README.zh.md)

## Summary

Build a standalone AgentWork server with its production DSH and WikiMan dependencies. This private workspace supplies the deployment dependency graph; it has no application bin and does not start a service.

## Table of Contents

- [Build and run](#build-and-run)
- [Dev Note](#dev-note)

<a id="build-and-run"></a>
## Build and run

Use the [unified build tutorial](../../docs/cookbook/releasing-agentwork.md). Native production dependencies are installed on the target platform, materialized into the runtime and verified with the shared frontend. The packaged adapter launches the ordinary named `dsh` profile. User home, library, workspace and private configuration remain outside the program package.

The [command reference](../../scripts/agentwork/README.md) owns artifact validation and development responsibilities. The ordinary source CLI distribution does not acquire WikiMan as a default dependency through this workspace.

<a id="dev-note"></a>
## Dev Note

None.
