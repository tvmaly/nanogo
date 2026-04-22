#!/usr/bin/env bash
# Enforce invariant #1: core/ must never import ext/ or pkg/
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MODULE="github.com/tvmaly/nanogo"
FAIL=0

while IFS= read -r -d '' file; do
  if grep -qE "\"${MODULE}/(ext|pkg)/" "$file"; then
    echo "VIOLATION: $file imports ext/ or pkg/" >&2
    grep -nE "\"${MODULE}/(ext|pkg)/" "$file" >&2
    FAIL=1
  fi
done < <(find "$REPO_ROOT/core" -name "*.go" ! -name "*_test.go" -print0)

if [ "$FAIL" -eq 1 ]; then
  echo "check_imports: FAILED" >&2
  exit 1
fi
echo "check_imports: OK"
