#!/usr/bin/env bash
# Enforce invariant #4: core/ LOC budgets.
# Counts non-test, non-fake .go files only.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FAIL=0

count_loc() {
  local dir="$1"
  local full="$REPO_ROOT/$dir"
  [ -d "$full" ] || { echo 0; return; }
  find "$full" -maxdepth 1 -name "*.go" ! -name "*_test.go" -print0 \
    | xargs -0 wc -l 2>/dev/null | tail -1 | awk '{print $1}'
}

count_loc_recursive() {
  local dir="$1"
  local full="$REPO_ROOT/$dir"
  [ -d "$full" ] || { echo 0; return; }
  find "$full" -name "*.go" ! -name "*_test.go" ! -path "*/fake/*" -print0 \
    | xargs -0 grep -ch "." 2>/dev/null | awk '{s+=$1}END{print s+0}'
}

check() {
  local dir="$1"
  local ceiling="$2"
  local actual
  actual=$(count_loc_recursive "$dir")
  if [ "$actual" -gt "$ceiling" ]; then
    echo "BUDGET EXCEEDED: $dir: $actual > $ceiling LOC" >&2
    FAIL=1
  else
    echo "  OK  $dir: $actual / $ceiling"
  fi
}

echo "Core LOC budget check:"
check "core/agent"     500
check "core/memory"    550
check "core/tools"     400
check "core/skills"    220
check "core/session"   180
check "core/heartbeat" 120
check "core/runtime"   120
check "core/event"      96
check "core/harness"   120
check "core/llm"       130
check "core/config"    100
check "core/obs"        70
check "core/transport"  60
check "core/scheduler"  40

TOTAL=$(count_loc_recursive "core")
echo "  Core total: $TOTAL / 2690"
if [ "$TOTAL" -gt 2690 ]; then
  echo "BUDGET EXCEEDED: core total $TOTAL > 2690" >&2
  FAIL=1
fi

if [ "$FAIL" -eq 1 ]; then
  echo "loc_budget: FAILED" >&2
  exit 1
fi
echo "loc_budget: OK"
