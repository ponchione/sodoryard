BIN_DIR             := bin
WEB_DIR             := web
DESKTOP_DIR         := desktop
DESKTOP_RENDERER_HOST ?= 127.0.0.1
DESKTOP_RENDERER_PORT ?= 5173
YARD_BINARY          ?= $(CURDIR)/$(BIN_DIR)/yard
YARD_PROJECT_DIR     ?= $(CURDIR)
GO_TAGS             := sqlite_fts5
GOFLAGS_DB          := -tags '$(GO_TAGS)'
WEBFS_DIST          := webfs/dist
GO_PACKAGES         = $$(go list $(GOFLAGS_DB) ./... | awk '$$0 !~ /\/web\/node_modules\//')
LANCEDB_LIB_DIR     := $(CURDIR)/lib/linux_amd64
LANCEDB_CGO_LDFLAGS := -L$(LANCEDB_LIB_DIR) -llancedb_go -lm -ldl -lpthread
CGO_TEST_ENV        := CGO_ENABLED=1 CGO_LDFLAGS="$(LANCEDB_CGO_LDFLAGS)" LD_LIBRARY_PATH="$(LANCEDB_LIB_DIR)"
CGO_BUILD_ENV       := CGO_ENABLED=1 CGO_LDFLAGS="$(LANCEDB_CGO_LDFLAGS) -Wl,-rpath,$(LANCEDB_LIB_DIR)"
RETIRED_BINARIES    := $(BIN_DIR)/sirtopham $(BIN_DIR)/knapford

.PHONY: all build cleanup-retired-binaries webfs-assets tidmouth yard yard-embedded install-user-bin test dev-backend dev-frontend dev desktop-deps desktop-build desktop-test desktop-package desktop-dev frontend-deps frontend-build frontend-test frontend-typecheck projectmemory-bindings projectmemory-bindings-check projectmemory-sdk-smoke clean

# `make build` builds every retained binary needed for a runnable local tree:
# the operator-facing yard CLI plus the internal tidmouth engine used by chain
# spawning. `make all` is kept as an alias for the same supported artifact set.
all: build

build: cleanup-retired-binaries tidmouth yard

cleanup-retired-binaries:
	rm -f $(RETIRED_BINARIES)

# -- Binaries ---------------------------------------------------------
# tidmouth: the retained internal headless engine harness used by chain
# spawning. The frontend build/copy happens here so `make build` prepares
# webfs/dist before building the operator-facing yard binary.
webfs-assets: frontend-build
	rm -rf $(WEBFS_DIST)
	mkdir -p $(WEBFS_DIST)
	cp -r $(WEB_DIR)/dist/. $(WEBFS_DIST)/
	touch $(WEBFS_DIST)/.gitkeep

tidmouth: webfs-assets
	mkdir -p $(BIN_DIR)
	$(CGO_BUILD_ENV) go build $(GOFLAGS_DB) -o $(BIN_DIR)/tidmouth ./cmd/tidmouth

# yard: operator-facing CLI for project bootstrap, runtime control, and chain
# orchestration. Same SQLite (FTS5) and lancedb cgo wiring as tidmouth.
yard:
	mkdir -p $(BIN_DIR)
	$(CGO_BUILD_ENV) go build $(GOFLAGS_DB) -o $(BIN_DIR)/yard ./cmd/yard

yard-embedded: webfs-assets
	mkdir -p $(BIN_DIR)
	$(CGO_BUILD_ENV) go build $(GOFLAGS_DB) -o $(BIN_DIR)/yard ./cmd/yard

install-user-bin:
	bash ./scripts/install-user-bin.sh

test: projectmemory-bindings-check
	$(CGO_TEST_ENV) go test $(GOFLAGS_DB) $(GO_PACKAGES)

# -- Development ------------------------------------------------------
# Two-terminal workflow:
#   Terminal 1: make dev-backend
#   Terminal 2: make dev-frontend
# The Vite dev server proxies /api/* to the Go backend.

dev-backend:
	$(CGO_TEST_ENV) go run $(GOFLAGS_DB) ./cmd/yard serve --dev

dev-frontend:
	cd $(WEB_DIR) && npm run dev

dev: dev-backend

desktop-deps:
	cd $(DESKTOP_DIR) && npm install

desktop-build: yard-embedded desktop-deps
	cd $(DESKTOP_DIR) && npm run build

desktop-test: desktop-deps
	cd $(DESKTOP_DIR) && npm test

desktop-package: desktop-build
	cd $(DESKTOP_DIR) && npm run package:unpacked

desktop-dev: yard frontend-deps desktop-deps
	set -e; \
	renderer_port=$$(node $(DESKTOP_DIR)/scripts/find-free-port.mjs "$(DESKTOP_RENDERER_HOST)" "$(DESKTOP_RENDERER_PORT)"); \
	renderer_url="http://$(DESKTOP_RENDERER_HOST):$$renderer_port"; \
	echo "Starting desktop renderer at $$renderer_url"; \
	( \
		cd $(WEB_DIR) && npm run dev -- --host $(DESKTOP_RENDERER_HOST) --port $$renderer_port --strictPort \
	) & \
	web_pid=$$!; \
	trap 'kill $$web_pid 2>/dev/null || true' EXIT INT TERM; \
	cd $(DESKTOP_DIR) && \
		YARD_PROJECT_DIR="$(YARD_PROJECT_DIR)" \
		YARD_BINARY="$(YARD_BINARY)" \
		YARD_RENDERER_URL="$$renderer_url" \
		npm run dev

# -- Frontend ---------------------------------------------------------
frontend-deps:
	cd $(WEB_DIR) && npm install

frontend-build: frontend-deps
	cd $(WEB_DIR) && npm run build

frontend-test: frontend-deps
	cd $(WEB_DIR) && npm run test

frontend-typecheck:
	cd $(WEB_DIR) && npx tsc --noEmit

projectmemory-bindings:
	go run $(GOFLAGS_DB) ./cmd/yard-projectmemory-codegen

projectmemory-bindings-check:
	go run $(GOFLAGS_DB) ./cmd/yard-projectmemory-codegen --check

projectmemory-sdk-smoke: frontend-deps
	$(CGO_TEST_ENV) go run $(GOFLAGS_DB) ./cmd/projectmemory-sdk-smoke

# -- Clean ------------------------------------------------------------
clean:
	rm -rf $(BIN_DIR)
	rm -rf $(WEB_DIR)/dist
	rm -rf $(DESKTOP_DIR)/out
	mkdir -p $(WEBFS_DIST)
	find $(WEBFS_DIST) -mindepth 1 -maxdepth 1 ! -name .gitkeep -exec rm -rf {} +
