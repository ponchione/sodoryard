#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=${REPO_ROOT:-$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)}

echo "Checking required source-build prerequisites"
"$REPO_ROOT/scripts/doctor-dev.sh" --required-only

echo
echo "Installing web dependencies"
npm install --prefix "$REPO_ROOT/web"

echo
echo "Installing desktop dependencies"
npm install --prefix "$REPO_ROOT/desktop"

echo
echo "Checking generated project-memory bindings"
(cd "$REPO_ROOT" && go run -tags sqlite_fts5 ./cmd/yard-projectmemory-codegen --check)

echo
echo "Bootstrap complete."
echo "Useful next commands:"
echo "  make desktop-dev          # development loop"
echo "  make desktop-install-user # rebuild source package and refresh taskbar launcher"
echo "  make doctor-dev           # full source-development health report"
