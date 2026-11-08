---
kind: upgrade-guide
description: "AgentWork release inventories add installation-guide digests, and new publication requires the documented format."
---

# Publish AgentWork with installation guides

English | [中文](guide.zh.md)

## Change

New AgentWork release directories use release.json schemaVersion 2 with checksummed README.md and README.en.md documents. Publication requires this complete format and adds direct target downloads to release notes. Existing schemaVersion 1 release directories remain readable for verification and deployment, but cannot be newly published with this publisher. Artifact manifests and knowledge data formats are unchanged.

## Migration

1. Rebuild or collect a complete cohort through the current unified scripts; do not hand-edit old release.json or checksum files.
2. Verify the release and confirm both installation guides appear in its documents inventory and SHA256SUMS.
3. Publish a new product version from the verified cohort. Preserve existing public releases and their assets. See the [installation tutorial](../../../cookbook/installing-agentwork-release.md) and [release workflow](../../../cookbook/releasing-agentwork.md).
