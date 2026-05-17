#!/usr/bin/env bash
# Document and enforce the Phase 17 core boundary during migration.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FAIL=0

while IFS= read -r -d '' dir; do
  name="$(basename "$dir")"
  case "$name" in
  agent|event|harness|llm|runtime|session|tools)
    echo "  KERNEL  core/$name"
    continue
    ;;
  esac
  echo "VIOLATION: undocumented core package core/$name" >&2
  FAIL=1
done < <(find "$REPO_ROOT/core" -maxdepth 1 -mindepth 1 -type d -print0 | sort -z)

if [[ "$FAIL" -eq 1 ]]; then
  echo "check_core_boundary: FAILED" >&2
  exit 1
fi

echo "check_core_boundary: OK"
