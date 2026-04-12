BIN_DIR             := bin
WEB_DIR             := web
WEBFS_DIST          := webfs/dist
GO_TAGS             := sqlite_fts5
GOFLAGS_DB          := -tags '$(GO_TAGS)'
LANCEDB_LIB_DIR     := $(CURDIR)/lib/linux_amd64
LANCEDB_CGO_LDFLAGS := -L$(LANCEDB_LIB_DIR) -llancedb_go -lm -ldl -lpthread
CGO_TEST_ENV        := CGO_ENABLED=1 CGO_LDFLAGS="$(LANCEDB_CGO_LDFLAGS)" LD_LIBRARY_PATH="$(LANCEDB_LIB_DIR)"
CGO_BUILD_ENV       := CGO_ENABLED=1 CGO_LDFLAGS="$(LANCEDB_CGO_LDFLAGS) -Wl,-rpath,$(LANCEDB_LIB_DIR)"

.PHONY: all build tidmouth sirtopham knapford yard test dev-backend dev-frontend dev frontend-deps frontend-build frontend-typecheck clean

# `make all` builds every monorepo binary. `make build` is an alias for
# `make tidmouth` to preserve the single-binary workflow during Phase 1/2.
all: tidmouth sirtopham knapford yard

build: tidmouth

# ── Binaries ─────────────────────────────────────────────────────────
# tidmouth: the headless engine harness. Embeds the React frontend via
# webfs/go:embed until Knapford absorbs the web UI (Phase 6).
tidmouth: frontend-build
	rm -rf $(WEBFS_DIST) && cp -r $(WEB_DIR)/dist $(WEBFS_DIST)
	mkdir -p $(BIN_DIR)
	$(CGO_BUILD_ENV) go build $(GOFLAGS_DB) -o $(BIN_DIR)/tidmouth ./cmd/tidmouth

# sirtopham: chain orchestrator. Shares the same SQLite (FTS5) and lancedb
# cgo wiring as tidmouth because it opens the same .yard/yard.db file.
sirtopham:
	mkdir -p $(BIN_DIR)
	$(CGO_BUILD_ENV) go build $(GOFLAGS_DB) -o $(BIN_DIR)/sirtopham ./cmd/sirtopham

# knapford: web dashboard (Phase 6 placeholder for now).
knapford:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/knapford ./cmd/knapford

# yard: operator-facing CLI for project bootstrap. Same SQLite (FTS5) and
# lancedb cgo wiring as tidmouth/sirtopham because internal/initializer
# opens the same .yard/yard.db database.
yard:
	mkdir -p $(BIN_DIR)
	$(CGO_BUILD_ENV) go build $(GOFLAGS_DB) -o $(BIN_DIR)/yard ./cmd/yard

test:
	$(CGO_TEST_ENV) go test $(GOFLAGS_DB) ./...

# ── Development ──────────────────────────────────────────────────────
# Two-terminal workflow:
#   Terminal 1: make dev-backend
#   Terminal 2: make dev-frontend
# The Vite dev server proxies /api/* to the Go backend.

dev-backend:
	$(CGO_TEST_ENV) go run $(GOFLAGS_DB) ./cmd/tidmouth serve --dev

dev-frontend:
	cd $(WEB_DIR) && npm run dev

dev: dev-backend

# ── Frontend ─────────────────────────────────────────────────────────
frontend-deps:
	cd $(WEB_DIR) && npm install

frontend-build: frontend-deps
	cd $(WEB_DIR) && npm run build

frontend-typecheck:
	cd $(WEB_DIR) && npx tsc --noEmit

# ── Clean ────────────────────────────────────────────────────────────
clean:
	rm -rf $(BIN_DIR)
	rm -rf $(WEB_DIR)/dist
	rm -rf $(WEBFS_DIST)
