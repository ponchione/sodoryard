# 17 — Yard Containerization

**Status:** Design (ready for implementation plan)
**Owner:** Mitchell Ponchione
**Last Updated:** 2026-04-11
**Roadmap phase:** 7
**Depends on:** Phases 1–5b (binaries built, `yard init` exists)
**Defers:** Phase 6 (Knapford service) — compose declares the slot, leaves the binary as a placeholder

---

## 1. Goal

Package the railway as a runnable Docker image and a `docker-compose.yaml` so an operator can mount any project directory and use the railway CLI surface (`yard init`, `yard install`, `tidmouth run`, `sirtopham chain`, `tidmouth index`) inside a container without installing Go, lancedb, or anything else on the host. `yard install` remains as compatibility tooling for older placeholder-based configs, but stock `yard init` configs now run with embedded prompts and do not require an agents-dir substitution step.

The container is **headless-only** in Phase 7. The `knapford` service in `docker-compose.yaml` is a placeholder slot that points at the existing `cmd/knapford/main.go` placeholder binary and exposes port 8080. It does not start a real web service. Phase 6 fills it in.

## 2. Why this spec exists

Three things are missing today:

1. **There is no Dockerfile.** The four Go binaries (`tidmouth`, `sirtopham`, `yard`, `knapford`) require a working Go toolchain plus a non-trivial set of cgo dependencies (`liblancedb_go.so`, `sqlite_fts5`) on the host to build and run. Anyone who wants to use the railway on a server, in CI, or on a different developer laptop has to reproduce that build environment by hand. A multi-stage Docker image means the build environment is encoded once, in the repo.

2. **The lancedb shared library is wired with a host-absolute rpath.** The current Makefile builds with `-Wl,-rpath,/home/gernsback/source/sodoryard/lib/linux_amd64`. That path is baked into every binary at link time and does not exist inside any container. A Dockerfile that just copies the binaries in will produce a container that fails at startup with `error while loading shared libraries: liblancedb_go.so`. Phase 7 fixes this once: the runtime image puts `liblancedb_go.so` at `/usr/local/lib/`, the builder rebuilds with `-Wl,-rpath,/usr/local/lib`, and `ldconfig` updates the linker cache so the standard search path finds the library regardless of rpath.

3. **Older configs may still contain `{{SODORYARD_AGENTS_DIR}}` placeholders.** The container still ships the editable `agents/` directory at `/opt/yard/agents/` and `yard install` can still rewrite those older configs. The Dockerfile sets the env var so `yard install` inside the container is a single zero-flag invocation; on the host the operator passes the flag once.

## 3. Locked decisions

### 3.1 Scope — working headless container, Knapford slot is a placeholder

The Phase 7 deliverable is a Dockerfile and a `docker-compose.yaml` that produce a runnable image plus a useful one-shot CLI experience. Specifically:

- `docker compose build yard` produces a working image
- `docker compose run --rm yard yard init` initializes the mounted project
- `docker compose run --rm yard yard install` can still perform the agents-dir substitution for legacy placeholder-based configs
- `docker compose run --rm yard tidmouth run --role <r> --task <t>` runs a headless engine session
- `docker compose run --rm yard sirtopham chain --task <t>` runs a chain
- `docker compose up knapford` starts a container that immediately exits with the placeholder string (until Phase 6 ships a real long-running service)

**Why:** the user explicitly chose this scope ("Phase 7 too, or at least the stubs") with the expectation that Phase 6 lands later. A working headless container provides immediate value (CI runs, remote dev, throwaway dogfood environments) without coupling Phase 7 to Phase 6. The Knapford slot lets Phase 6 ship purely as binary work, no compose-file changes needed.

### 3.2 Cross-phase fix — new `yard install` command, not magic env var resolution

A new `cmd/yard/install.go` subcommand resolves the `{{SODORYARD_AGENTS_DIR}}` placeholder in `yard.yaml`:

```
yard install [flags]

Substitute {{SODORYARD_AGENTS_DIR}} in yard.yaml with the absolute path
to your sodoryard install's agents/ directory.

Flags:
  --sodoryard-agents-dir string   Path to sodoryard's agents/ directory.
                                  If empty, reads SODORYARD_AGENTS_DIR env var.
  --config string                 Override the yard.yaml path (default "yard.yaml")
```

`yard install` is the compatibility path for configs that still carry the agents-dir placeholder. `yard init` now emits built-in prompt selectors for stock roles, so no substitution is needed in the default path.

**Why:** spec 16 §3.4 locked "exactly two substitutions in init: `{{PROJECT_ROOT}}` and `{{PROJECT_NAME}}`." Resolving the agents dir inside `yard init` would break that decision and require Phase 5b to ship together with Phase 7. Splitting into a second command keeps the phases independent and preserves `yard install` as compatibility tooling for older placeholder-based configs, while fresh stock `yard init` configs run immediately without it.

The Dockerfile sets `ENV SODORYARD_AGENTS_DIR=/opt/yard/agents`, so inside the container `yard install` works with no flags. On the host, the operator runs `yard install --sodoryard-agents-dir /home/me/source/sodoryard/agents` once per project.

`yard install` is also useful on the host as the canonical alternative to manual `sed` — if it exists, the operator never needs to know which placeholder to find-and-replace.

**Phase 5b is not retroactively updated.** The Phase 5b plan that committed earlier this session uses `sed` for the smoke test. Phase 5b stays exactly as written; the next time Phase 5b is executed (after Phase 7 lands), the operator can use `yard install` instead of `sed` if they want, but the existing plan still works as written. No coupling.

### 3.3 Base image — `debian:bookworm-slim`

The runtime stage is `debian:bookworm-slim` (~80MB base, ~30MB compressed). It has `/bin/sh`, `apt`, `bash`, and the basic coreutils, which makes `docker exec -it <container> bash` work for live debugging.

**Why:** the railway is pre-1.0, the container will be iterated heavily during early Phase 6 dogfooding, and the first few times something fails inside the container the operator wants a shell. Distroless is the right answer once the container is stable, not during the period when its behavior is still being shaped. A future post-1.0 cleanup pass can swap to distroless. Image size savings (~60MB) are dwarfed by the lancedb shared library plus the four Go binaries (`bin/tidmouth` alone is 39MB), so the optimization buys very little.

**Constraints that ruled out alternatives:**
- **musl-based images (alpine):** `liblancedb_go.so` is compiled against glibc. Running it on musl crashes at startup. Hard constraint.
- **Multi-arch (linux/arm64):** `lib/linux_amd64/liblancedb_go.so` is the only architecture in the repo. amd64-only is a hard constraint until somebody produces an arm64 build of the lancedb cgo library, which is its own project.
- **Scratch:** no glibc, no shell, no debugging — premature and incompatible with the cgo library.

### 3.4 Compose architecture — two separate compose files, share `llm-net`

Phase 7 introduces a new `docker-compose.yaml` at the **repo root**. It does **not** modify or replace `ops/llm/docker-compose.yml`. They are independent compose files that share an external Docker network called `llm-net`.

```
sodoryard/
├── docker-compose.yaml         # NEW — railway services (yard, knapford-placeholder)
├── ops/
│   └── llm/
│       └── docker-compose.yml  # EXISTING — local LLM inference (qwen-coder, nomic-embed)
```

**Operator workflow:**

```bash
# Optional: start the local LLM services if dogfooding against local llama.cpp
cd ops/llm && docker compose up -d

# Always: build/run the railway container
cd /repo/root && docker compose build yard
cd <project> && PROJECT_DIR=$(pwd) docker compose -f /repo/root/docker-compose.yaml run --rm yard yard init
```

**Both compose files declare `llm-net` as `external: true`**, so the railway container can reach the LLM services by service name (`http://qwen-coder:12434`, `http://nomic-embed:12435`) when both are up. When the LLM compose is down, the railway container still runs — it just can't reach the local LLM services, which is fine for codex/anthropic provider work.

**Why:** the existing `ops/llm/docker-compose.yml` is a separate concern (GPU-heavy, optional, "the local inference services"). The railway has its own lifecycle and shouldn't bring up GPU containers every time you want `yard init` to run. Two compose files keep responsibilities clean and let Phase 6 (Knapford) drop into the same root compose without complicating the LLM compose.

**Alternatives considered:**
- **Single root compose with everything inlined.** Forces GPU startup whenever you want yard. Couples lifecycles. Drifts from the existing LLM compose.
- **Root compose uses `include:` to pull in `ops/llm/docker-compose.yml`.** Compose `include:` is from late 2023 and is footgun-prone in older CI environments. Marginal benefit over two-file independence.

### 3.5 lancedb library distribution — `/usr/local/lib/` + `ldconfig` + rebuild rpath

The runtime image stages `liblancedb_go.so` at `/usr/local/lib/liblancedb_go.so`, runs `ldconfig` to update the dynamic linker cache, and the builder stage rebuilds the Go binaries with `-Wl,-rpath,/usr/local/lib` so even if the cache is stale the binary's embedded rpath finds the library.

Concretely, in the runtime stage:

```dockerfile
COPY --from=builder /workspace/lib/linux_amd64/liblancedb_go.so /usr/local/lib/
RUN ldconfig
```

And the builder stage rebuilds with the corrected rpath:

```dockerfile
ENV CGO_LDFLAGS="-L/workspace/lib/linux_amd64 -llancedb_go -lm -ldl -lpthread -Wl,-rpath,/usr/local/lib"
RUN go build -tags sqlite_fts5 -o /out/tidmouth ./cmd/tidmouth
RUN go build -tags sqlite_fts5 -o /out/sirtopham ./cmd/sirtopham
RUN go build -tags sqlite_fts5 -o /out/yard ./cmd/yard
RUN go build -o /out/knapford ./cmd/knapford
```

**Why:** the standard Linux dynamic linker search path includes `/usr/local/lib` after `ldconfig` runs, so binaries find the library without needing `LD_LIBRARY_PATH` or any env var gymnastics. Rebuilding with the corrected rpath is belt-and-suspenders: even if `ldconfig` cache regeneration breaks for some reason, the binary's own rpath points at the right place. Both mechanisms are independent and converge on the same answer.

**Alternatives considered:**
- **`LD_LIBRARY_PATH` in the entrypoint.** Works but pollutes the operator's environment, breaks if the operator overrides the var, and requires every `docker exec` to reset it. Fragile.
- **Copy the .so to `/usr/lib/x86_64-linux-gnu/` instead of `/usr/local/lib/`.** Works but pollutes the distro-managed library path. `/usr/local/lib` is the standard "site-installed library" location and doesn't conflict with `apt`.
- **Static linking lancedb into the Go binaries.** Would eliminate the .so entirely. Requires the lancedb Go bindings to support static linking, which they do not (the .so is a thin C wrapper around a Rust library, no static archive ships in `lib/linux_amd64/`).

### 3.6 Image filesystem layout

| Path | Contents |
|---|---|
| `/usr/local/bin/tidmouth` | tidmouth binary (rebuilt in builder stage) |
| `/usr/local/bin/sirtopham` | sirtopham binary (rebuilt in builder stage) |
| `/usr/local/bin/yard` | yard binary (rebuilt in builder stage) |
| `/usr/local/bin/knapford` | knapford binary (placeholder until Phase 6) |
| `/usr/local/lib/liblancedb_go.so` | lancedb shared library |
| `/opt/yard/agents/*.md` | the 13 agent prompt files copied from `agents/` |
| `/project` | bind-mounted from host, the operator's project root, also `WORKDIR` |

The container has no `/sodoryard/`, no `/root/source/sodoryard/`, no encoded notion of the original repo layout. Everything the railway needs at runtime is at well-known paths:

- The **binaries** are on `$PATH` (`/usr/local/bin` is on the default Debian path)
- The **lancedb library** is on the dynamic linker search path (`/usr/local/lib` after `ldconfig`)
- The **agent prompts** are at the env var `SODORYARD_AGENTS_DIR=/opt/yard/agents`, which `yard install` reads
- The **project** is bind-mounted at `/project`, which is the container's `WORKDIR`, so a bare `yard init` operates on the mounted project automatically

**Why this layout:**
- `/usr/local/{bin,lib}` is the canonical site-installed location on Debian; standard tooling (`ldconfig`, `apt`) leaves it alone
- `/opt/yard/` is the canonical "third-party application" location; using it for agent prompts keeps them out of the Debian-managed tree
- `/project` is short, memorable, and clearly distinguishes "the operator's mounted project" from "the railway's installed files"

### 3.7 Builder image and frontend stage

The Dockerfile is multi-stage:

1. **`frontend-builder`** (`node:20-bookworm-slim`): runs `npm install && npm run build` in `/web/`, producing `/web/dist/`. Output is `/web/dist/`.
2. **`go-builder`** (`golang:1.22-bookworm`): copies the Go source plus the frontend build output, copies the lancedb static library from `lib/linux_amd64/`, runs the four `go build` commands with the corrected rpath. Output is `/out/{tidmouth,sirtopham,yard,knapford}`.
3. **`runtime`** (`debian:bookworm-slim`): copies binaries from `go-builder` to `/usr/local/bin/`, copies `liblancedb_go.so` from `go-builder` to `/usr/local/lib/`, runs `ldconfig`, copies the agents from `agents/` to `/opt/yard/agents/`, sets `ENV SODORYARD_AGENTS_DIR=/opt/yard/agents`, sets `WORKDIR /project`, sets `CMD ["yard", "--help"]`.

**Why three stages:**
- `frontend-builder` keeps Node out of the runtime image entirely
- `go-builder` keeps the ~500MB Go toolchain out of the runtime image
- `runtime` is the smallest viable image that contains only what the operator actually executes

The frontend stage produces `web/dist/` which the Go stage copies to `webfs/dist/` so `tidmouth`'s `webfs/embed.go` (the existing frontend embedding) finds it during compilation. After the Go binaries are linked, the embedded frontend lives inside `/usr/local/bin/tidmouth` itself — there's no separate frontend asset shipping in the runtime image.

### 3.8 `docker-compose.yaml` shape

The new root-level `docker-compose.yaml`:

```yaml
services:
  yard:
    build:
      context: .
      dockerfile: Dockerfile
    image: ponchione/yard:dev
    networks:
      - llm-net
    volumes:
      - ${PROJECT_DIR:-./}:/project
    environment:
      - YARD_PROJECT=/project
      # Override on the host shell to point at a different provider/credential setup.
      - SODORYARD_AGENTS_DIR=/opt/yard/agents

  knapford:
    image: ponchione/yard:dev
    command: ["knapford"]
    networks:
      - llm-net
    volumes:
      - ${PROJECT_DIR:-./}:/project
    ports:
      - "8080:8080"
    environment:
      - YARD_PROJECT=/project
    profiles:
      - knapford  # not started by `docker compose up` unless explicitly requested

networks:
  llm-net:
    external: true
```

Notes:

- The `knapford` service uses Compose **profiles** so it does NOT start with a default `docker compose up`. Operators who want to run the placeholder explicitly can `docker compose --profile knapford up knapford`. Once Phase 6 lands, the profile gate can be removed and `knapford` becomes a normal default-on service.
- Both services share the same image (the runtime stage builds all four binaries, the `command:` field selects which one runs).
- `${PROJECT_DIR:-./}` falls back to the current shell's working directory if `PROJECT_DIR` is unset, so a bare `docker compose run yard yard init` from inside the project works. Operators who run from the sodoryard checkout itself should `export PROJECT_DIR=/path/to/their/project` first.
- The `llm-net` network is declared external so this compose file does not own it. `ops/llm/docker-compose.yml` already creates it (`networks: llm-net: external: true` there too — both files are external consumers, the network is created by `docker network create llm-net` once).

**Operator workflow inside the container:**

```bash
# Build once
docker compose build yard

# In the project directory you want to bootstrap:
cd /home/me/source/myproject
PROJECT_DIR=$(pwd) docker compose -f /home/me/source/sodoryard/docker-compose.yaml run --rm yard yard init
PROJECT_DIR=$(pwd) docker compose -f /home/me/source/sodoryard/docker-compose.yaml run --rm yard yard install

# Run a chain inside the container
PROJECT_DIR=$(pwd) docker compose -f /home/me/source/sodoryard/docker-compose.yaml run --rm yard \
  sirtopham chain --task "do the thing" --max-steps 5
```

The `yard init && yard install` two-step happens once per project. After that the container is a fully self-contained operator surface for that project.

### 3.9 `.dockerignore`

A new `.dockerignore` at the repo root prevents host build artifacts and state directories from leaking into the build context:

```
.git/
bin/
web/node_modules/
webfs/dist/
.brain/
.yard/
.sirtopham/
*.log
*.tmp
docs/
ops/llm/models/
.idea/
.vscode/
```

**Notes:**
- `bin/` is excluded so the host's `make build` artifacts don't shadow the in-container builds
- `web/node_modules/` is excluded so the host's npm install state doesn't go into the build context (the frontend stage runs its own `npm install`)
- `webfs/dist/` is excluded so the host's frontend build output doesn't end up in the Go-stage build context (the frontend stage produces its own `web/dist/` and the Go stage copies it explicitly)
- `.brain/`, `.yard/`, `.sirtopham/` are excluded so per-project state never enters the build context
- `docs/` is excluded because the runtime image doesn't need spec docs
- `ops/llm/models/` is excluded because the GGUF model files are multi-GB and have nothing to do with the railway image
- **`templates/init/` is intentionally NOT excluded.** The Go stage embeds it via `//go:embed all:templates/init` from spec 16; excluding it would produce a yard binary that can't initialize projects because the embed would be empty. Anything embedded into a Go binary via `go:embed` must be present in the Docker build context.

(Implementation note: when adding new files to `.dockerignore`, double-check none of them are referenced by a `//go:embed` directive in the Go source.)

## 4. Component architecture

```
Repository root after Phase 7:

sodoryard/
├── Dockerfile                  # NEW — multi-stage: node, go-build, runtime
├── docker-compose.yaml         # NEW — yard + knapford-placeholder services
├── .dockerignore               # NEW — keep host artifacts out of build context
├── cmd/yard/
│   ├── main.go                 # existing (Phase 5b)
│   ├── init.go                 # existing (Phase 5b)
│   └── install.go              # NEW — yard install subcommand
├── internal/initializer/       # existing (Phase 5b)
├── ops/llm/docker-compose.yml  # UNCHANGED
└── ... everything else unchanged
```

`cmd/yard/install.go` is a thin cobra wrapper that calls a new function in `internal/initializer/` (let's call it `Install`) so the substitution logic is testable in isolation. The new function lives next to `internal/initializer/substitute.go` because it's the same family of placeholder-resolution code.

```
internal/initializer/
├── initializer.go        # existing, Run() entrypoint
├── templates.go          # existing
├── substitute.go         # existing
├── obsidian.go           # existing
├── gitignore.go          # existing
├── database.go           # existing
├── install.go            # NEW — Install() function
├── install_test.go       # NEW — unit tests for Install
├── ... (existing test files)
```

## 5. The `yard install` command

### 5.1 What it does

`yard install` reads the project's `yard.yaml` (or whichever file `--config` points at), finds every occurrence of `{{SODORYARD_AGENTS_DIR}}`, and replaces it with the resolved agents directory. The resolved directory is, in order of priority:

1. The value of `--sodoryard-agents-dir` if set
2. The value of `SODORYARD_AGENTS_DIR` env var if set
3. Error: "no agents directory provided; pass --sodoryard-agents-dir or set SODORYARD_AGENTS_DIR"

The substitution is **destructive** — it overwrites `yard.yaml` in place. It is **idempotent** — running `yard install` against an already-substituted yaml is a no-op (no occurrences of `{{SODORYARD_AGENTS_DIR}}` to replace, file content unchanged).

### 5.2 What it doesn't do

- **Does not validate that the agents directory exists.** The path is substituted as a string. If the operator points it at `/does/not/exist`, the resulting `yard.yaml` will have invalid `system_prompt` paths and `tidmouth run` will fail with a clear error message — that's a better failure mode than trying to validate filesystem state in `yard install` and getting it wrong on remote/symlinked/permission-restricted directories.
- **Does not touch other placeholders.** `{{PROJECT_ROOT}}` and `{{PROJECT_NAME}}` were already substituted by `yard init`. Any other unknown `{{...}}` token is left alone.
- **Does not run `yard init`** if the file doesn't exist. The two are independent commands. If `yard.yaml` doesn't exist, `yard install` errors with "yard.yaml not found; run 'yard init' first".
- **Does not write a backup.** The operator can `cp yard.yaml yard.yaml.bak` if they want one. Phase 7 keeps the surface minimal.

### 5.3 Example invocations

On the host (one-time setup for a project):

```bash
cd /home/me/source/myproject
yard init
yard install --sodoryard-agents-dir /home/me/source/sodoryard/agents
# yard.yaml now has /home/me/source/sodoryard/agents/coder.md (etc) instead of {{SODORYARD_AGENTS_DIR}}/coder.md
```

Inside the container (Dockerfile sets the env var):

```bash
docker compose run --rm yard yard init
docker compose run --rm yard yard install
# the container's SODORYARD_AGENTS_DIR=/opt/yard/agents env var resolves the placeholder
# yard.yaml now has /opt/yard/agents/coder.md (etc)
```

Re-run safety:

```bash
yard install --sodoryard-agents-dir /home/me/source/sodoryard/agents
# already substituted; no occurrences left to replace; file unchanged; exit 0
```

## 6. Acceptance criteria — "Phase 7 done"

Phase 7 is complete when **all** of the following are true:

1. `Dockerfile` exists at the repo root and `docker compose build yard` produces an image with no errors.
2. `docker-compose.yaml` exists at the repo root with the `yard` and `knapford` services defined per §3.8.
3. `.dockerignore` exists at the repo root and excludes the right things (verified by checking the build context size with `docker build --no-cache --progress=plain . 2>&1 | grep "transferring context"`).
4. `cmd/yard/install.go` exists, `yard install --help` prints meaningful usage, and `make all` builds it as part of `bin/yard`.
5. `internal/initializer/install.go` and `internal/initializer/install_test.go` exist with unit test coverage of the substitution path, the env-var fallback path, the missing-yaml error path, and the idempotent re-run path.
6. The runtime image runs `ldconfig` and the linker can find `liblancedb_go.so` — verified by `docker compose run --rm yard ldconfig -p | grep lancedb` returning a hit.
7. `docker compose run --rm yard tidmouth --help` prints help (proves the binary loads its cgo deps successfully).
8. `docker compose run --rm yard sirtopham --help` prints help (same proof for sirtopham).
9. `docker compose run --rm yard yard --help` prints help (same proof for yard).
10. End-to-end smoke: in a fresh `/tmp/yard-container-smoke-*` directory, `PROJECT_DIR=$(pwd) docker compose -f <repo>/docker-compose.yaml run --rm yard yard init && yard install` produces a working `yard.yaml` with the agents dir substituted to `/opt/yard/agents`.
11. End-to-end smoke continued: with the agents prompts shipped at `/opt/yard/agents` inside the container, `PROJECT_DIR=$(pwd) docker compose ... run --rm yard sirtopham chain --task "..."` runs a real chain and produces both an orchestrator and an engine receipt under `<smoke>/.brain/receipts/`. Same shape as the Phase 3 smoke chain, but inside the container.
12. `docker compose --profile knapford up knapford` starts the placeholder Knapford container, which exits immediately printing the placeholder string. (The point: the slot exists in compose, the binary is callable, the lifecycle is wired up — Phase 6 fills in the actual web service.)
13. `docs/specs/17-yard-containerization.md` (this file) is updated to match anything that drifted during implementation.
14. The Phase 7 commit stack is tagged `v0.7-containerization` (NOT `v1.0-sodor` — that tag is reserved for Phase 6 + Phase 7 shipped together).

## 7. Out of scope

- **Multi-arch images.** amd64 only. Arm64 needs a `lib/linux_arm64/liblancedb_go.so` that doesn't exist yet.
- **Distroless / scratch / alpine runtime.** debian-slim only.
- **A real Knapford service.** Knapford stays a placeholder until Phase 6.
- **Pushing the image to any registry.** No `docker push`, no GHCR config, no image tagging beyond the local `ponchione/yard:dev` tag in the compose file.
- **Tag `v1.0-sodor`.** Reserved for Phase 6 + Phase 7 shipped together. Phase 7 alone tags `v0.7-containerization`.
- **Image hardening / non-root user.** Phase 7 runs as `root` inside the container. Revisit when the image moves to distroless.
- **Health checks / liveness probes.** No long-running web service in Phase 7 to probe.
- **Buildx cache mounts / `--mount=type=cache` optimizations.** Standard multi-stage build only. Optimization belongs to a later "container build performance" pass once the Dockerfile shape is stable.
- **Wiring the container to the existing `local_services` config block.** Operators who want to use the local LLM compose set the env var or config field as they would on the host. Phase 7 does not auto-detect or wire up the LLM compose.
- **Updating `docs/specs/00-index.md`** to reference specs 10–17. Already-stale, out of scope.
- **Changing `cmd/tidmouth/init.go` or anything in Phase 5b.** Phase 7 is purely additive to Phase 5b. The Phase 5b plan that already committed stays exactly as written.

## 8. Open questions / future work

These are intentionally left unresolved by this spec. Each is a future decision, not a Phase 7 blocker.

- **Should the image ship a `:latest` tag in addition to `:dev`?** No registry yet, so the question is moot until Phase 7+ adds image publishing. Probably yes when it does.
- **Should there be a `Dockerfile.dev` variant** with the Go toolchain still present so operators can run `go test ./...` inside the container? Not in Phase 7. Operators use `make test` on the host.
- **Should `yard install` also run inside `yard init` automatically when the env var is set?** Per §3.2, no — that re-introduces the environment-conditional magic the brainstorming round (Q7-2) explicitly rejected. The two-command sequence `yard init && yard install` is the right shape regardless of host vs container.
- **Should the Phase 6 Knapford service inherit the same image layout** (binaries at `/usr/local/bin/`, agent prompts at `/opt/yard/agents/`)? Probably yes — Phase 6 should not introduce a parallel filesystem layout — but that's a Phase 6 design question, not a Phase 7 commitment.
- **Should the compose file declare a Postgres or Redis sidecar** for orchestrator state? Not in Phase 7. SQLite at `.yard/yard.db` is the only state surface and lives on the bind-mounted project volume. If multi-process orchestrator state ever needs a real database, that's its own future spec.

## 9. References

- `sodor-migration-roadmap.md` — Phase 7 description
- `docs/specs/16-yard-init.md` — `yard init` (the predecessor that introduces the placeholder this spec resolves)
- `Makefile` — current build with the host-absolute rpath that Phase 7 fixes inside the Dockerfile
- `ops/llm/docker-compose.yml` — the existing LLM services compose that Phase 7's compose coexists with (not modified)
- `cmd/yard/main.go` — cobra root that Phase 5b creates and Phase 7 adds `install` to
- `internal/initializer/substitute.go` — the substitution helper that Phase 7's `install.go` mirrors
- `webfs/embed.go` (if it exists) — the frontend embedding that the Dockerfile's Go stage relies on

---

**End of spec.** Implementation plan to follow via the writing-plans skill once this spec is approved.
