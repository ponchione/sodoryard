BIN_DIR             := bin
WEB_DIR             := web
WEBFS_DIST          := webfs/dist
GO_TAGS             := sqlite_fts5
GOFLAGS_DB          := -tags '$(GO_TAGS)'
LANCEDB_LIB_DIR     := $(CURDIR)/lib/linux_amd64
LANCEDB_CGO_LDFLAGS := -L$(LANCEDB_LIB_DIR) -llancedb_go -lm -ldl -lpthread
CGO_TEST_ENV        := CGO_ENABLED=1 CGO_LDFLAGS="$(LANCEDB_CGO_LDFLAGS)" LD_LIBRARY_PATH="$(LANCEDB_LIB_DIR)"
CGO_BUILD_ENV       := CGO_ENABLED=1 CGO_LDFLAGS="$(LANCEDB_CGO_LDFLAGS) -Wl,-rpath,$(LANCEDB_LIB_DIR)"
RETIRED_BINARIES    := $(BIN_DIR)/sirtopham $(BIN_DIR)/knapford

.PHONY: all build cleanup-retired-binaries tidmouth yard install-user-bin test dev-backend dev-frontend dev frontend-deps frontend-build frontend-typecheck clean

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
tidmouth: frontend-build
	rm -rf $(WEBFS_DIST) && cp -r $(WEB_DIR)/dist $(WEBFS_DIST)
	mkdir -p $(BIN_DIR)
	$(CGO_BUILD_ENV) go build $(GOFLAGS_DB) -o $(BIN_DIR)/tidmouth ./cmd/tidmouth

# yard: operator-facing CLI for project bootstrap, runtime control, and chain
# orchestration. Same SQLite (FTS5) and lancedb cgo wiring as tidmouth.
yard:
	mkdir -p $(BIN_DIR)
	$(CGO_BUILD_ENV) go build $(GOFLAGS_DB) -o $(BIN_DIR)/yard ./cmd/yard

install-user-bin:
	bash ./scripts/install-user-bin.sh

test:
	$(CGO_TEST_ENV) go test $(GOFLAGS_DB) ./...

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

# -- Frontend ---------------------------------------------------------
frontend-deps:
	cd $(WEB_DIR) && npm install

frontend-build: frontend-deps
	cd $(WEB_DIR) && npm run build

frontend-typecheck:
	cd $(WEB_DIR) && npx tsc --noEmit

# -- Clean ------------------------------------------------------------
clean:
	rm -rf $(BIN_DIR)
	rm -rf $(WEB_DIR)/dist
	rm -rf $(WEBFS_DIST)
