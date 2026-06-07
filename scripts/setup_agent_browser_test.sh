#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

make_bin() {
  local name="$1"
  local body="$2"
  cat >"$TMPDIR/$name" <<EOF
#!/usr/bin/env bash
$body
EOF
  chmod +x "$TMPDIR/$name"
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  if [[ "$haystack" != *"$needle"* ]]; then
    echo "expected output to contain: $needle" >&2
    echo "$haystack" >&2
    exit 1
  fi
}

assert_not_contains() {
  local haystack="$1"
  local needle="$2"
  if [[ "$haystack" == *"$needle"* ]]; then
    echo "expected output not to contain: $needle" >&2
    echo "$haystack" >&2
    exit 1
  fi
}

make_bin agent-browser 'if [[ "$1" == "--version" ]]; then echo "agent-browser 0.27.1"; exit 0; fi; echo "agent-browser $*"'
make_bin brew 'if [[ "$1" == "list" ]]; then exit 1; fi; echo "brew $*"'
make_bin npm 'echo "npm $*"'
make_bin cargo 'echo "cargo $*"'

OUT="$(PATH="$TMPDIR:/usr/bin:/bin" NANOGO_AGENT_BROWSER_DRY_RUN=1 "$ROOT/scripts/setup_agent_browser.sh")"
assert_contains "$OUT" "agent-browser already at required version"
assert_not_contains "$OUT" "brew upgrade agent-browser"
assert_not_contains "$OUT" "npm install -g agent-browser"
assert_not_contains "$OUT" "cargo install agent-browser --locked"
assert_contains "$OUT" "DRY RUN: agent-browser install"
assert_contains "$OUT" "DRY RUN: agent-browser doctor --offline --quick"

rm "$TMPDIR/agent-browser"
OUT="$(PATH="$TMPDIR:/usr/bin:/bin" NANOGO_AGENT_BROWSER_DRY_RUN=1 "$ROOT/scripts/setup_agent_browser.sh")"
assert_contains "$OUT" "DRY RUN: brew install agent-browser"
assert_not_contains "$OUT" "npm install -g agent-browser"
assert_not_contains "$OUT" "cargo install agent-browser --locked"

rm "$TMPDIR/brew"
OUT="$(PATH="$TMPDIR:/usr/bin:/bin" NANOGO_AGENT_BROWSER_DRY_RUN=1 "$ROOT/scripts/setup_agent_browser.sh")"
assert_contains "$OUT" "DRY RUN: npm install -g agent-browser"
assert_not_contains "$OUT" "cargo install agent-browser --locked"

rm "$TMPDIR/npm"
OUT="$(PATH="$TMPDIR:/usr/bin:/bin" NANOGO_AGENT_BROWSER_DRY_RUN=1 "$ROOT/scripts/setup_agent_browser.sh")"
assert_contains "$OUT" "DRY RUN: cargo install agent-browser --locked"
