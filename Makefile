BINARY              := sirtopham
BIN_DIR             := bin
CMD_PKG             := ./cmd/sirtopham
FRONTEND            := frontend
GO_TAGS             := sqlite_fts5
GOFLAGS_DB          := -tags '$(GO_TAGS)'
LANCEDB_LIB_DIR     := $(CURDIR)/lib/linux_amd64
LANCEDB_CGO_LDFLAGS := -L$(LANCEDB_LIB_DIR) -llancedb_go -lm -ldl -lpthread
CGO_TEST_ENV        := CGO_ENABLED=1 CGO_LDFLAGS="$(LANCEDB_CGO_LDFLAGS)" LD_LIBRARY_PATH="$(LANCEDB_LIB_DIR)"
CGO_BUILD_ENV       := CGO_ENABLED=1 CGO_LDFLAGS="$(LANCEDB_CGO_LDFLAGS) -Wl,-rpath,$(LANCEDB_LIB_DIR)"

.PHONY: build test dev-backend dev-frontend dev frontend-deps frontend-build clean

build: frontend-build
	mkdir -p $(BIN_DIR)
	$(CGO_BUILD_ENV) go build $(GOFLAGS_DB) -o $(BIN_DIR)/$(BINARY) $(CMD_PKG)

test:
	$(CGO_TEST_ENV) go test $(GOFLAGS_DB) ./...

dev-backend:
	$(CGO_TEST_ENV) go run $(GOFLAGS_DB) $(CMD_PKG) serve --dev

dev-frontend:
	@echo "Frontend not yet implemented"

dev: dev-backend

frontend-deps:
	@echo "Frontend not yet implemented"

frontend-build:
	@echo "Frontend not yet implemented — skipping"

clean:
	rm -rf $(BIN_DIR)
	rm -rf $(FRONTEND)/dist
