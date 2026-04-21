#!/usr/bin/env bash
set -euo pipefail

repo_root=$(mktemp -d)
trap 'rm -rf "$repo_root"' EXIT

mkdir -p "$repo_root/bin"
for name in tidmouth yard; do
  printf '#!/usr/bin/env bash\necho %s\n' "$name" >"$repo_root/bin/$name"
  chmod +x "$repo_root/bin/$name"
done

target_home=$(mktemp -d)
trap 'rm -rf "$target_home"' EXIT

echo "Running install-user-bin smoke test"
REPO_ROOT="$repo_root" TARGET_BIN_DIR="$target_home/bin" SKIP_BUILD=1 ./scripts/install-user-bin.sh >/tmp/install-user-bin.out 2>&1

for name in tidmouth yard; do
  if [[ ! -x "$target_home/bin/$name" ]]; then
    echo "expected executable at $target_home/bin/$name" >&2
    cat /tmp/install-user-bin.out >&2 || true
    exit 1
  fi
done

if ! grep -q "Installed 2 binaries" /tmp/install-user-bin.out; then
  echo "expected summary output" >&2
  cat /tmp/install-user-bin.out >&2 || true
  exit 1
fi

echo "PASS: install-user-bin copied expected binaries"
