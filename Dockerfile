# syntax=docker/dockerfile:1.6

# ─── Stage 1: frontend builder ──────────────────────────────────────
# Builds the React frontend that tidmouth embeds via go:embed.
# Output is /web/dist which the Go stage copies to webfs/dist/ before
# compiling, so the embed picks it up.
FROM node:22-slim AS frontend-builder

WORKDIR /web

# Copy the full frontend source (including public/ which the postinstall
# script copies augmented-ui.css into).
COPY web/ ./
RUN npm install

# Build. Output goes to /web/dist.
RUN npm run build


# ─── Stage 2: Go builder ────────────────────────────────────────────
# Compiles the four Go binaries (tidmouth, sirtopham, yard, knapford)
# with sqlite_fts5 + lancedb cgo wiring. Rebuilds rpath to point at
# the runtime image's library location (/usr/local/lib) so the
# binaries find liblancedb_go.so without env var gymnastics.
FROM golang:1.25-trixie AS go-builder

WORKDIR /workspace

# Copy go.mod and go.sum first for layer cache friendliness.
COPY go.mod go.sum ./
RUN go mod download

# Copy the source tree (everything not excluded by .dockerignore).
COPY . .

# Copy the frontend build output to the location tidmouth's
# webfs/embed.go expects.
COPY --from=frontend-builder /web/dist ./webfs/dist

# Build the four binaries with the corrected rpath. The CGO_LDFLAGS
# rpath points at /usr/local/lib because that's where the runtime
# stage stages liblancedb_go.so.
ENV CGO_ENABLED=1
ENV CGO_LDFLAGS="-L/workspace/lib/linux_amd64 -llancedb_go -lm -ldl -lpthread -Wl,-rpath,/usr/local/lib"

RUN go build -tags sqlite_fts5 -o /out/tidmouth ./cmd/tidmouth
RUN go build -tags sqlite_fts5 -o /out/sirtopham ./cmd/sirtopham
RUN go build -tags sqlite_fts5 -o /out/yard ./cmd/yard
RUN go build -o /out/knapford ./cmd/knapford


# ─── Stage 3: runtime ───────────────────────────────────────────────
# Slim debian image with glibc + the four binaries + lancedb shared
# library + agent prompts. No Go toolchain, no Node, no source.
FROM debian:trixie-slim AS runtime

# ca-certificates: needed for HTTPS calls to provider APIs (codex,
# anthropic). tini: PID 1 init for clean signal handling.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        tini \
    && rm -rf /var/lib/apt/lists/*

# Stage liblancedb_go.so at the standard site-installed library path.
# ldconfig updates the linker cache so binaries find it via the
# normal search path, in addition to the embedded rpath.
COPY --from=go-builder /workspace/lib/linux_amd64/liblancedb_go.so /usr/local/lib/
RUN ldconfig

# Install the four binaries.
COPY --from=go-builder /out/tidmouth /usr/local/bin/tidmouth
COPY --from=go-builder /out/sirtopham /usr/local/bin/sirtopham
COPY --from=go-builder /out/yard /usr/local/bin/yard
COPY --from=go-builder /out/knapford /usr/local/bin/knapford

# Install the 13 agent prompts at the canonical container location.
# yard install reads SODORYARD_AGENTS_DIR (set below) when invoked
# inside the container, so the substitution lands at this path.
COPY --from=go-builder /workspace/agents /opt/yard/agents

# Tell yard install where the agents directory lives. Operators
# inside the container do not need to pass --sodoryard-agents-dir.
ENV SODORYARD_AGENTS_DIR=/opt/yard/agents

# Bind-mounted project lives at /project; make it the working
# directory so a bare 'yard init' operates on the mounted project.
WORKDIR /project

# tini as PID 1 means signals propagate cleanly. Default command is
# yard --help so a bare 'docker run' shows the help text.
ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["yard", "--help"]
