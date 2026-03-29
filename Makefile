BINARY     := sirtopham
BIN_DIR    := bin
CMD_PKG    := ./cmd/sirtopham
FRONTEND   := frontend
GO_TAGS    := sqlite_fts5
GOFLAGS_DB := -tags '$(GO_TAGS)'

.PHONY: build test dev-backend dev-frontend dev frontend-deps frontend-build clean

build: frontend-build
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build $(GOFLAGS_DB) -o $(BIN_DIR)/$(BINARY) $(CMD_PKG)

test:
	CGO_ENABLED=1 go test $(GOFLAGS_DB) ./...

dev-backend:
	CGO_ENABLED=1 go run $(GOFLAGS_DB) $(CMD_PKG) serve --dev

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
