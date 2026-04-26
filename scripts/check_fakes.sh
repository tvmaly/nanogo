#!/usr/bin/env bash
# Enforce invariant #5: every core interface package has a fake/ sibling.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FAIL=0

# Packages in core/ that must have a fake/ sibling (add as they're implemented)
REQUIRED=(
  "core/event"
  "core/llm"
  "core/tools"
  "core/session"
  "core/agent"
  "core/memory"
  "core/harness"
  "core/scheduler"
  "core/heartbeat"
)

for pkg in "${REQUIRED[@]}"; do
  fake_dir="$REPO_ROOT/$pkg/fake"
  if [ ! -d "$fake_dir" ]; then
    echo "MISSING fake/: $pkg/fake/" >&2
    FAIL=1
  else
    go_files=$(find "$fake_dir" -maxdepth 1 -name "*.go" | wc -l)
    if [ "$go_files" -eq 0 ]; then
      echo "EMPTY fake/: $pkg/fake/" >&2
      FAIL=1
    else
      echo "  OK  $pkg/fake/"
    fi
  fi
done

if [ "$FAIL" -eq 1 ]; then
  echo "check_fakes: FAILED" >&2
  exit 1
fi
echo "check_fakes: OK"
