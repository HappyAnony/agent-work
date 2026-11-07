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
APP_SOURCE="${WIKIMAN_PREBUILT_APP_DIR:-$ROOT/wiki-app}"
if [ -n "${WIKIMAN_PREBUILT_APP_DIR:-}" ]; then
  case "$APP_SOURCE" in /*) ;; *) printf 'WIKIMAN_PREBUILT_APP_DIR must be absolute\n' >&2; exit 1 ;; esac
  test -f "$APP_SOURCE/stage/build/portable/boot.js"
  test -f "$APP_SOURCE/appearance/langs/zh_CN.json"
  if [ "$(cd "$APP_SOURCE" && pwd -P)" = "$(cd "$OUT/wiki-app" && pwd -P)" ]; then
    printf 'Prebuilt app resources must be outside the output directory\n' >&2; exit 1
  fi
else
  (
    cd "$ROOT/wiki-app"
    pnpm install --ignore-workspace --frozen-lockfile --ignore-scripts
    pnpm run build
  )
fi
rsync -a --delete "$APP_SOURCE/stage/" "$OUT/wiki-app/stage/"
rsync -a --delete "$APP_SOURCE/appearance/" "$OUT/wiki-app/appearance/"
# An optional trusted Pandoc archive enables Word export without embedding every platform binary.
if [ -n "${WIKIMAN_PANDOC_ARCHIVE:-}" ]; then
  mkdir -p "$OUT/wiki-app/pandoc"
  cp "$WIKIMAN_PANDOC_ARCHIVE" "$OUT/wiki-app/pandoc/pandoc-$(cd "$ROOT/wiki-kernel" && go env GOOS)-$(cd "$ROOT/wiki-kernel" && go env GOARCH).zip"
fi
mkdir -p "$OUT/notices/pollwatch"
cp "$ROOT/LICENSE-AGPL-3.0.txt" "$OUT/notices/"
cp "$ROOT/wiki-kernel/internal/pollwatch/backend/LICENSE" "$ROOT/wiki-kernel/internal/pollwatch/backend/"*.md "$OUT/notices/pollwatch/"
printf 'WikiMan runtime built at %s\n' "$OUT"
