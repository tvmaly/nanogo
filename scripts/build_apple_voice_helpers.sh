#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/ext/voice/providers/apple/helpers/bin"
SRC="$ROOT/ext/voice/providers/apple/helpers"

if [ "$(uname -s)" != "Darwin" ]; then
  echo "apple voice helpers skipped: requires Darwin"
  exit 0
fi

if ! command -v swiftc >/dev/null 2>&1; then
  echo "ERROR: apple voice helpers require swiftc" >&2
  exit 1
fi

SDK_VERSION="$(xcrun --sdk macosx --show-sdk-version 2>/dev/null || true)"
if [ -z "$SDK_VERSION" ]; then
  echo "ERROR: apple voice helpers require a macOS SDK" >&2
  exit 1
fi
SDK_MAJOR="${SDK_VERSION%%.*}"
if [ "$SDK_MAJOR" -lt 26 ]; then
  echo "ERROR: apple SpeechAnalyzer helper requires macOS SDK 26+; found $SDK_VERSION" >&2
  exit 1
fi

mkdir -p "$OUT"
CACHE="/tmp/nanogo-swift-module-cache"
mkdir -p "$CACHE"
swiftc -module-cache-path "$CACHE" -O -o "$OUT/apple-avspeech-helper" "$SRC/apple-avspeech-helper/main.swift"
swiftc -module-cache-path "$CACHE" -O -o "$OUT/apple-speechanalyzer-helper" "$SRC/apple-speechanalyzer-helper/main.swift"
echo "Built Apple voice helpers in $OUT"
