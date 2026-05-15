#!/usr/bin/env bash
set -uo pipefail

REPO_ROOT=${REPO_ROOT:-$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)}
MODE=${1:-full}
MIN_GO=${MIN_GO:-1.25.5}
MIN_NODE_MAJOR=${MIN_NODE_MAJOR:-22}
LANCEDB_LIB_DIR=${LANCEDB_LIB_DIR:-"$REPO_ROOT/lib/linux_amd64"}

failures=0
warnings=0

ok() {
  printf 'ok: %s\n' "$1"
}

warn() {
  warnings=$((warnings + 1))
  printf 'warn: %s\n' "$1"
}

fail() {
  failures=$((failures + 1))
  printf 'fail: %s\n' "$1"
}

has_command() {
  command -v "$1" >/dev/null 2>&1
}

version_at_least() {
  local actual=$1
  local minimum=$2
  local actual_major actual_minor actual_patch minimum_major minimum_minor minimum_patch
  IFS=. read -r actual_major actual_minor actual_patch _ <<<"$actual"
  IFS=. read -r minimum_major minimum_minor minimum_patch _ <<<"$minimum"
  actual_major=${actual_major//[^0-9]/}
  actual_minor=${actual_minor//[^0-9]/}
  actual_patch=${actual_patch//[^0-9]/}
  minimum_major=${minimum_major//[^0-9]/}
  minimum_minor=${minimum_minor//[^0-9]/}
  minimum_patch=${minimum_patch//[^0-9]/}
  actual_major=${actual_major:-0}
  actual_minor=${actual_minor:-0}
  actual_patch=${actual_patch:-0}
  minimum_major=${minimum_major:-0}
  minimum_minor=${minimum_minor:-0}
  minimum_patch=${minimum_patch:-0}
  if (( actual_major != minimum_major )); then
    (( actual_major > minimum_major ))
    return
  fi
  if (( actual_minor != minimum_minor )); then
    (( actual_minor > minimum_minor ))
    return
  fi
  (( actual_patch >= minimum_patch ))
}

section() {
  printf '\n%s\n' "$1"
}

check_command() {
  local name=$1
  local label=$2
  if has_command "$name"; then
    ok "$label: $(command -v "$name")"
  else
    fail "$label is not installed or not on PATH"
  fi
}

check_go() {
  if ! has_command go; then
    fail "Go is not installed or not on PATH"
    return
  fi
  local actual
  actual=$(go env GOVERSION 2>/dev/null | sed 's/^go//')
  if [[ -z "$actual" ]]; then
    actual=$(go version | awk '{print $3}' | sed 's/^go//')
  fi
  if version_at_least "$actual" "$MIN_GO"; then
    ok "Go $actual"
  else
    fail "Go $actual is older than required $MIN_GO"
  fi
}

check_node() {
  if ! has_command node; then
    fail "Node.js is not installed or not on PATH"
    return
  fi
  local actual major
  actual=$(node -p 'process.versions.node' 2>/dev/null)
  major=${actual%%.*}
  if [[ "$major" =~ ^[0-9]+$ ]] && (( major >= MIN_NODE_MAJOR )); then
    ok "Node.js $actual"
  else
    fail "Node.js $actual is older than required major $MIN_NODE_MAJOR"
  fi
}

check_lancedb() {
  local shared="$LANCEDB_LIB_DIR/liblancedb_go.so"
  local archive="$LANCEDB_LIB_DIR/liblancedb_go.a"
  if [[ -f "$shared" ]]; then
    ok "LanceDB shared library: $shared"
  else
    fail "missing LanceDB shared library: $shared"
  fi
  if [[ -f "$archive" ]]; then
    ok "LanceDB archive library: $archive"
  else
    warn "missing LanceDB archive library: $archive"
  fi
}

check_node_modules() {
  if [[ -d "$REPO_ROOT/web/node_modules" ]]; then
    ok "web dependencies installed"
  else
    warn "web dependencies are not installed; run make bootstrap"
  fi
  if [[ -d "$REPO_ROOT/desktop/node_modules" ]]; then
    ok "desktop dependencies installed"
  else
    warn "desktop dependencies are not installed; run make bootstrap"
  fi
  if [[ -f "$REPO_ROOT/third_party/shunter-client/package.json" ]]; then
    ok "vendored @shunter/client package present"
  else
    fail "missing vendored @shunter/client package"
  fi
}

check_generated_bindings() {
  local output
  if ! has_command go; then
    warn "skipping generated binding check because Go is unavailable"
    return
  fi
  output=$(cd "$REPO_ROOT" && go run -tags sqlite_fts5 ./cmd/yard-projectmemory-codegen --check 2>&1)
  local status=$?
  if [[ $status -eq 0 ]]; then
    ok "project memory bindings are current"
  else
    fail "project memory bindings are stale or codegen failed"
    printf '%s\n' "$output" | tail -n 8
  fi
}

check_yard_config() {
  local yard="$REPO_ROOT/bin/yard"
  if [[ ! -x "$yard" ]]; then
    warn "bin/yard is not built; run make build or make desktop-install-user"
    return
  fi
  local output
  output=$(cd "$REPO_ROOT" && LD_LIBRARY_PATH="$LANCEDB_LIB_DIR:${LD_LIBRARY_PATH:-}" "$yard" config 2>&1)
  local status=$?
  if [[ $status -eq 0 ]]; then
    ok "yard config loads"
    printf '%s\n' "$output" | awk -F': ' '/^default_provider:|^default_model:|^default_reasoning_effort:|^embedding_base_url:|^local_services_mode:/ { printf "info: %s=%s\n", $1, $2 }'
  else
    fail "yard config failed"
    printf '%s\n' "$output" | tail -n 8
  fi
}

check_yard_auth() {
  local yard="$REPO_ROOT/bin/yard"
  if [[ ! -x "$yard" ]]; then
    warn "skipping provider auth check because bin/yard is not built"
    return
  fi
  local output
  output=$(cd "$REPO_ROOT" && LD_LIBRARY_PATH="$LANCEDB_LIB_DIR:${LD_LIBRARY_PATH:-}" "$yard" auth status 2>&1)
  local status=$?
  if [[ $status -eq 0 ]]; then
    ok "provider auth status is readable"
    printf '%s\n' "$output" | awk 'NR <= 6 { printf "info: %s\n", $0 }'
  else
    warn "provider auth status is not ready"
    printf '%s\n' "$output" | tail -n 8
  fi
}

check_local_models() {
  local llm_dir="$REPO_ROOT/ops/llm/models"
  local completion="$llm_dir/Qwen2.5-Coder-7B-Instruct-Q6_K_L.gguf"
  local embedding="$llm_dir/nomic-embed-code.Q8_0.gguf"
  if [[ -f "$completion" ]]; then
    ok "local completion model present"
  else
    warn "local completion model missing: $completion"
  fi
  if [[ -f "$embedding" ]]; then
    ok "local embedding model present"
  else
    warn "local embedding model missing: $embedding"
  fi
}

check_desktop_launcher() {
  if [[ "$(uname -s)" != "Linux" ]]; then
    warn "desktop launcher check is Linux-only"
    return
  fi
  local package_launcher="$REPO_ROOT/desktop/out/Yard-linux-x64/yard-desktop"
  local package_source="$REPO_ROOT/desktop/out/Yard-linux-x64/resources/app/yard-source.json"
  local desktop_entry="${XDG_DATA_HOME:-$HOME/.local/share}/applications/yard-desktop.desktop"
  if [[ -x "$package_launcher" ]]; then
    ok "source-built desktop package launcher is executable"
  else
    warn "source-built desktop package is missing; run make desktop-install-user"
  fi
  if [[ -f "$package_source" ]]; then
    local source_root
    source_root=$(node -e "const fs=require('node:fs'); const p=process.argv[1]; try { console.log(JSON.parse(fs.readFileSync(p, 'utf8')).sourceRoot || '') } catch { process.exit(1) }" "$package_source" 2>/dev/null)
    if [[ "$source_root" == "$REPO_ROOT" ]]; then
      ok "desktop package points at this source checkout"
    else
      warn "desktop package source root is $source_root, expected $REPO_ROOT; run make desktop-install-user"
    fi
  else
    warn "desktop package source metadata missing; run make desktop-install-user"
  fi
  if [[ -f "$desktop_entry" ]]; then
    ok "user desktop launcher installed: $desktop_entry"
    local exec_path
    exec_path=$(sed -n 's/^Exec="\([^"]*\)".*/\1/p' "$desktop_entry" | head -n 1)
    if [[ -n "$exec_path" && -x "$exec_path" ]]; then
      ok "installed desktop launcher Exec target is executable"
    else
      warn "installed desktop launcher Exec target is missing or not executable"
    fi
    if has_command desktop-file-validate; then
      local validation
      validation=$(desktop-file-validate "$desktop_entry" 2>&1)
      if [[ -z "$validation" ]]; then
        ok "installed desktop launcher validates"
      else
        warn "desktop-file-validate reported issues"
        printf '%s\n' "$validation"
      fi
    fi
  else
    warn "user desktop launcher is not installed; run make desktop-install-user"
  fi
}

section "Required Toolchain"
check_go
check_node
check_command npm npm
check_command make make
check_command gcc "C compiler"
check_lancedb

if [[ "$MODE" != "--required-only" ]]; then
  section "Repository Dependencies"
  check_node_modules
  check_generated_bindings

  section "Yard Runtime"
  check_yard_config
  check_yard_auth
  check_local_models

  section "Desktop Source Package"
  check_desktop_launcher
fi

printf '\nsummary: %d failure(s), %d warning(s)\n' "$failures" "$warnings"
if (( failures > 0 )); then
  exit 1
fi
