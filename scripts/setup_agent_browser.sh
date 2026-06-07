#!/usr/bin/env bash
set -euo pipefail

# Installs or updates agent-browser and its managed browser binary.
#
# Test hooks:
#   NANOGO_AGENT_BROWSER_DRY_RUN=1 prints commands instead of running them.
#   NANOGO_AGENT_BROWSER_WITH_DEPS=1 allows Linux system dependency install.

DRY_RUN="${NANOGO_AGENT_BROWSER_DRY_RUN:-0}"
MIN_AGENT_BROWSER_VERSION="0.27.0"

run() {
  if [ "$DRY_RUN" = "1" ]; then
    printf 'DRY RUN:'
    printf ' %q' "$@"
    printf '\n'
    return 0
  fi
  "$@"
}

have() {
  command -v "$1" >/dev/null 2>&1
}

agent_browser_version() {
  agent-browser --version 2>/dev/null | sed -E 's/.*([0-9]+\.[0-9]+\.[0-9]+).*/\1/'
}

version_at_least() {
  local have_version="$1"
  local want_version="$2"
  local IFS=.
  local have_parts want_parts
  read -r -a have_parts <<<"$have_version"
  read -r -a want_parts <<<"$want_version"
  local i
  for i in 0 1 2; do
    local have_num="${have_parts[$i]:-0}"
    local want_num="${want_parts[$i]:-0}"
    if (( have_num > want_num )); then
      return 0
    fi
    if (( have_num < want_num )); then
      return 1
    fi
  done
  return 0
}

cli_ready() {
  if ! have agent-browser; then
    return 1
  fi
  local version
  version="$(agent_browser_version)"
  if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    return 1
  fi
  version_at_least "$version" "$MIN_AGENT_BROWSER_VERSION"
}

install_with_brew() {
  echo "Installing or updating agent-browser with Homebrew..."
  if brew list --formula agent-browser >/dev/null 2>&1; then
    run brew upgrade agent-browser
  else
    run brew install agent-browser
  fi
}

install_with_npm() {
  echo "Installing or updating agent-browser with npm..."
  run npm install -g agent-browser
}

install_with_cargo() {
  echo "Installing or updating agent-browser with Cargo..."
  run cargo install agent-browser --locked
}

install_cli() {
  if have brew; then
    install_with_brew
    return
  fi
  if have npm; then
    install_with_npm
    return
  fi
  if have cargo; then
    install_with_cargo
    return
  fi
  cat >&2 <<'EOF'
ERROR: agent-browser setup requires one installer on PATH: Homebrew, npm, or Cargo.

Install one of:
  - Homebrew: https://brew.sh/
  - Node/npm: https://nodejs.org/
  - Rust/Cargo: https://www.rust-lang.org/tools/install
EOF
  exit 1
}

install_browser_binary() {
  if ! have agent-browser && [ "$DRY_RUN" != "1" ]; then
    echo "ERROR: agent-browser was not found on PATH after CLI installation." >&2
    exit 1
  fi

  if [ "$(uname -s)" = "Linux" ]; then
    if [ "${NANOGO_AGENT_BROWSER_WITH_DEPS:-0}" = "1" ]; then
      echo "Installing agent-browser browser binaries with Linux system dependencies..."
      run agent-browser install --with-deps
    else
      echo "Installing agent-browser browser binaries..."
      run agent-browser install
      echo "If Linux system dependencies are missing, rerun with:"
      echo "  NANOGO_AGENT_BROWSER_WITH_DEPS=1 scripts/setup_agent_browser.sh"
    fi
  else
    echo "Installing agent-browser browser binaries..."
    run agent-browser install
  fi
}

doctor() {
  echo "Running agent-browser doctor..."
  run agent-browser doctor --offline --quick
}

main() {
  if cli_ready; then
    echo "agent-browser already at required version ${MIN_AGENT_BROWSER_VERSION}; skipping package-manager upgrade."
  else
    install_cli
  fi
  install_browser_binary
  doctor
  echo "agent-browser setup complete."
}

main "$@"
