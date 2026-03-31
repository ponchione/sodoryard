BINARY              := sirtopham
BIN_DIR             := bin
CMD_PKG             := ./cmd/sirtopham
WEB_DIR             := web
WEBFS_DIST          := webfs/dist
GO_TAGS             := sqlite_fts5
GOFLAGS_DB          := -tags '$(GO_TAGS)'
LANCEDB_LIB_DIR     := $(CURDIR)/lib/linux_amd64
LANCEDB_CGO_LDFLAGS := -L$(LANCEDB_LIB_DIR) -llancedb_go -lm -ldl -lpthread
CGO_TEST_ENV        := CGO_ENABLED=1 CGO_LDFLAGS="$(LANCEDB_CGO_LDFLAGS)" LD_LIBRARY_PATH="$(LANCEDB_LIB_DIR)"
CGO_BUILD_ENV       := CGO_ENABLED=1 CGO_LDFLAGS="$(LANCEDB_CGO_LDFLAGS) -Wl,-rpath,$(LANCEDB_LIB_DIR)"

.PHONY: build test dev-backend dev-frontend dev frontend-deps frontend-build frontend-typecheck clean

# ── Build ────────────────────────────────────────────────────────────
# Compiles the React frontend, copies dist/ into webfs/ for go:embed,
# then builds the Go binary with the frontend embedded.
build: frontend-build
	rm -rf $(WEBFS_DIST) && cp -r $(WEB_DIR)/dist $(WEBFS_DIST)
	mkdir -p $(BIN_DIR)
	$(CGO_BUILD_ENV) go build $(GOFLAGS_DB) -o $(BIN_DIR)/$(BINARY) $(CMD_PKG)

test:
	$(CGO_TEST_ENV) go test $(GOFLAGS_DB) ./...

# ── Development ──────────────────────────────────────────────────────
# Two-terminal workflow:
#   Terminal 1: make dev-backend
#   Terminal 2: make dev-frontend
# The Vite dev server proxies /api/* to the Go backend.

dev-backend:
	$(CGO_TEST_ENV) go run $(GOFLAGS_DB) $(CMD_PKG) serve --dev

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
