#!/usr/bin/env bash
set -euo pipefail

ROOT="$(mktemp -d)"
trap 'rm -rf "$ROOT"' EXIT

mkdir -p "$ROOT/core/bad" "$ROOT/modules/bad" "$ROOT/scripts"
cp "$(dirname "$0")/check_imports.sh" "$ROOT/scripts/check_imports.sh"

cat > "$ROOT/go.mod" <<'EOF'
module github.com/tvmaly/nanogo
EOF

cat > "$ROOT/core/bad/bad.go" <<'EOF'
package bad

import _ "github.com/tvmaly/nanogo/ext/x"
EOF

if (cd "$ROOT" && scripts/check_imports.sh >/tmp/check_imports_core.out 2>&1); then
  echo "check_imports_test: expected core import violation" >&2
  exit 1
fi

rm "$ROOT/core/bad/bad.go"
cat > "$ROOT/modules/bad/bad.go" <<'EOF'
package bad

import _ "github.com/tvmaly/nanogo/ext/x"
EOF

if (cd "$ROOT" && scripts/check_imports.sh >/tmp/check_imports_modules.out 2>&1); then
  echo "check_imports_test: expected modules import violation" >&2
  exit 1
fi

echo "check_imports_test: OK"
