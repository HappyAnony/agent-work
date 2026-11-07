#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/wiki-kernel"
go test -tags fts5 ./internal/handlers/... ./internal/wikiops ./internal/workspace ./internal/wikistore ./internal/sourcetx
