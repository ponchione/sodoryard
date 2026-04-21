#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=${REPO_ROOT:-$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)}
TARGET_BIN_DIR=${TARGET_BIN_DIR:-${HOME}/bin}
SKIP_BUILD=${SKIP_BUILD:-0}
BINARIES=(${BINARIES:-tidmouth yard})

if [[ "$SKIP_BUILD" != "1" ]]; then
  echo "Building binaries via make all"
  make all -C "$REPO_ROOT"
fi

mkdir -p "$TARGET_BIN_DIR"

installed=0
for name in "${BINARIES[@]}"; do
  src="$REPO_ROOT/bin/$name"
  if [[ ! -f "$src" ]]; then
    echo "missing built binary: $src" >&2
    exit 1
  fi
  install -m 755 "$src" "$TARGET_BIN_DIR/$name"
  echo "installed $name -> $TARGET_BIN_DIR/$name"
  installed=$((installed + 1))
done

echo "Installed $installed binaries into $TARGET_BIN_DIR"
