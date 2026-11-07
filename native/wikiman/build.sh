#!/usr/bin/env bash
# Build the source-owned portable runtime; Node applications still launch through dsh.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT="${WIKIMAN_OUT_DIR:-$ROOT/out}"
mkdir -p "$OUT/bin" "$OUT/wiki-app"
(
  cd "$ROOT/wiki-kernel"
  go build -trimpath -ldflags='-s -w' -tags fts5 -o "$OUT/bin/wiki-kernel" ./cmd/wiki-kernel
  go build -trimpath -ldflags='-s -w' -tags fts5 -o "$OUT/bin/wiki-cli" ./cmd/wiki-cli
)
if [ "$(uname -s)" = Darwin ]; then
  xcrun clang -Os -fobjc-arc -framework AppKit -framework WebKit -framework PDFKit \
    "$ROOT/wiki-pdf/main.m" -o "$OUT/bin/wiki-pdf"
fi
(
  cd "$ROOT/wiki-app"
  pnpm install --ignore-workspace --frozen-lockfile --ignore-scripts
  pnpm run build
)
rsync -a --delete "$ROOT/wiki-app/stage/" "$OUT/wiki-app/stage/"
rsync -a --delete "$ROOT/wiki-app/appearance/" "$OUT/wiki-app/appearance/"
# An optional trusted Pandoc archive enables Word export without embedding every platform binary.
if [ -n "${WIKIMAN_PANDOC_ARCHIVE:-}" ]; then
  mkdir -p "$OUT/wiki-app/pandoc"
  cp "$WIKIMAN_PANDOC_ARCHIVE" "$OUT/wiki-app/pandoc/pandoc-$(cd "$ROOT/wiki-kernel" && go env GOOS)-$(cd "$ROOT/wiki-kernel" && go env GOARCH).zip"
fi
mkdir -p "$OUT/notices/pollwatch"
cp "$ROOT/LICENSE-AGPL-3.0.txt" "$OUT/notices/"
cp "$ROOT/wiki-kernel/internal/pollwatch/backend/LICENSE" "$ROOT/wiki-kernel/internal/pollwatch/backend/"*.md "$OUT/notices/pollwatch/"
printf 'WikiMan runtime built at %s\n' "$OUT"
