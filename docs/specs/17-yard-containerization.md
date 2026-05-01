# 17 — Yard Containerization

**Status:** Revised for no-legacy cleanup planning
**Owner:** Mitchell Ponchione
**Last Updated:** 2026-05-01
**Roadmap phase:** 7 contract reset
**Depends on:** `yard` as the only operator-facing CLI, current internal `tidmouth` spawn contract, existing `ops/llm/docker-compose.yml`
**Defers:** any future internal-engine rename or any separate Knapford service

---

## 1. Goal

Package the railway as a runnable Docker image and a repo-root `docker-compose.yaml` so an operator can mount any project directory and use the supported `yard` surface inside a container without installing Go, LanceDB, or Node on the host.

The no-legacy container contract is:
- operators use `yard` only
- there is no `yard install` compatibility flow
- there is no operator-facing `sirtopham` CLI inside the container
- `tidmouth` may still exist in the image, but only as an internal engine binary needed by chain spawning
- no separate `knapford` service is part of the container contract; the command center is served by `yard serve`

## 2. Why this spec exists

Three things still matter for containerization after the no-legacy reset:

1. A reproducible Docker build is still the right way to ship the Go binaries, embedded frontend, SQLite/FTS5 support, and LanceDB shared library without host setup drift.
2. The LanceDB shared library still needs a correct in-container runtime path.
3. Chain execution still needs the current internal engine spawn contract to keep working while public CLI duplication is removed.

What no longer matters:
- placeholder-driven config migration
- preserving old operator examples built around `sirtopham chain` or `tidmouth run`
- reserving a compose slot for a placeholder-only Knapford service

## 3. Locked decisions

### 3.1 Supported container surface

The supported operator flow inside the container is:
- `docker compose build yard`
- `docker compose run --rm yard yard init`
- `docker compose run --rm yard yard index`
- `docker compose run --rm yard yard chain start --role <r> --task <t>`
- `docker compose run --rm yard yard chain start --task <t>`
- `docker compose run --rm yard yard brain index`
- `docker compose run --rm yard yard serve`

The operator does not invoke `tidmouth` or `sirtopham` directly as part of the documented container workflow.

### 3.2 No compatibility install step

The container design does not preserve `yard install`, `{{SODORYARD_AGENTS_DIR}}`, or any placeholder-substitution command. A fresh `yard init` output must already be runnable without a second compatibility step.

Why:
- the repo-wide no-legacy mandate removes compatibility-only command surfaces rather than carrying them forward
- a two-step `yard init && yard install` flow encodes a migration story, not the desired steady-state operator experience
- container docs should describe the supported product shape, not historical migration helpers

### 3.3 Internal engine binary remains an implementation detail for now

Current chain execution still shells out to `tidmouth` internally via `internal/runtime/orchestrator.go` and `internal/spawn/spawn_agent.go`. This spec does not redesign that internal contract.

Container consequence:
- the runtime image may still ship `tidmouth`
- `tidmouth` is not documented as an operator command
- successful `yard chain start` in the container remains the proof that the internal engine contract is intact

### 3.4 `sirtopham` is out of scope for container UX

No container workflow, compose example, acceptance criterion, or help text in this spec depends on invoking `sirtopham` directly.

If an implementation still ships the binary temporarily during cleanup sequencing, that does not make it part of the supported container surface.

### 3.5 Command center lives in `yard serve`

This spec no longer reserves a placeholder service slot for `knapford`. Command-center work is active product scope in [[20-command-center-ui]], but it is implemented inside the existing `yard serve` web/API server and embedded frontend.

Why:
- placeholder-only compose services are legacy surface area with no runtime value
- a separate Knapford service would need its own spec and runtime contract before it entered container scope
- keeping a dead slot in compose would directly contradict the no-legacy mandate

### 3.6 Base image and runtime shape

The runtime stage remains Debian-based and glibc-compatible. The spec still assumes a multi-stage build that:
- builds the frontend separately
- builds Go binaries with `sqlite_fts5`
- stages `liblancedb_go.so` into a standard runtime location
- embeds the web assets into the Go binary

This is unchanged in principle from the earlier containerization design; only the supported CLI surface is narrowed.

### 3.7 Compose architecture

Phase 7 still uses a repo-root `docker-compose.yaml` that is independent from `ops/llm/docker-compose.yml` and may share the external `llm-net` network.

That separation remains correct because the local LLM stack and the railway container have different lifecycles.

## 4. Component architecture

Target repo shape after the no-legacy contract reset:

```text
sodoryard/
├── Dockerfile                  # multi-stage build for the supported runtime image
├── docker-compose.yaml         # yard service only
├── .dockerignore               # excludes host artifacts and state
├── cmd/yard/                   # only documented/operator-facing CLI
├── cmd/tidmouth/               # retained only while internal engine spawn still needs it
├── internal/runtime/           # shared runtime builders
├── internal/spawn/             # subprocess orchestration
├── ops/llm/docker-compose.yml  # separate local-LLM stack
└── ... everything else as needed
```

Not part of the target contract:
- `cmd/sirtopham/` as a supported container-facing CLI surface
- `cmd/yard/install.go`
- a placeholder-only `knapford` compose service

## 5. Docker image requirements

### 5.1 Runtime contents

The runtime image should contain exactly what is needed for the supported container UX and current internal chain execution:
- `yard`
- any retained internal binary still required by the orchestrator (`tidmouth` today)
- `liblancedb_go.so`
- Node.js and npm for mounted-project commands that expect Node tooling
- global `@openai/codex` CLI installation as an available runtime tool; Yard's Codex auth/runtime path does not shell out to it
- embedded frontend assets via the built Go binary
- the files needed for project initialization and runtime execution

Node/npm and the Codex CLI do not make additional operator-facing Yard commands. They are available inside the container for mounted-project commands and operator workflows that expect Node tooling; the supported container control surface remains `yard`.

### 5.2 Filesystem layout

Preferred layout remains:
- `/usr/local/bin/yard`
- `/usr/local/bin/tidmouth` only if still needed internally
- `/usr/local/lib/liblancedb_go.so`
- `/project` as the bind-mounted project root and `WORKDIR`

No contract in this spec depends on `/opt/yard/agents` or a `SODORYARD_AGENTS_DIR` environment variable.

### 5.3 LanceDB runtime linkage

The runtime image must place `liblancedb_go.so` somewhere the dynamic linker can find it reliably, such as `/usr/local/lib`, and the build should use a matching runtime search path.

The important acceptance property is functional, not stylistic:
- the containerized binaries must load and run without host-absolute linker paths

## 6. Compose file shape

The repo-root compose file should define a `yard` service and nothing placeholder-only.

Illustrative shape:

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

networks:
  llm-net:
    external: true
```

Notes:
- command-center UI work remains inside `yard serve` and does not add a compose service
- a separate Knapford service could be added later under a different spec if it ever becomes necessary
- if the implementation temporarily ships extra binaries in the image, that does not enlarge the documented operator surface
- `${PROJECT_DIR:-./}` remains a reasonable default for one-shot CLI use from the mounted project

## 7. Operator workflow

```bash
# Build once
cd /repo/root
docker compose build yard

# Initialize a project
cd /home/me/source/myproject
PROJECT_DIR=$(pwd) docker compose -f /repo/root/docker-compose.yaml run --rm yard yard init

# Build indexes
PROJECT_DIR=$(pwd) docker compose -f /repo/root/docker-compose.yaml run --rm yard yard index
PROJECT_DIR=$(pwd) docker compose -f /repo/root/docker-compose.yaml run --rm yard yard brain index

# Run one agent step as a one-step chain
PROJECT_DIR=$(pwd) docker compose -f /repo/root/docker-compose.yaml run --rm yard \
  yard chain start --role thomas --task "do the thing"

# Run a chain
PROJECT_DIR=$(pwd) docker compose -f /repo/root/docker-compose.yaml run --rm yard \
  yard chain start --task "do the thing"
```

No documented operator workflow in this spec includes `yard install`, `yard run`, `tidmouth run`, or `sirtopham chain`.

## 8. Acceptance criteria

Phase 7 containerization matches the no-legacy contract when all of the following are true:

1. `Dockerfile` exists at the repo root and `docker compose build yard` succeeds.
2. `docker-compose.yaml` exists at the repo root with a supported `yard` service.
3. `.dockerignore` excludes the usual host artifacts and project state.
4. `docker compose run --rm yard yard --help` works inside the built image.
5. `docker compose run --rm yard yard init` works in a fresh mounted project without any follow-up compatibility command.
6. `docker compose run --rm yard yard index` works in the container.
7. `docker compose run --rm yard yard chain start --task "..."` can start a real chain in the container, proving the internal engine-spawn path still works.
8. `liblancedb_go.so` is resolvable in the runtime image and does not depend on a host-absolute path.
9. No acceptance criterion requires `yard install`, `sirtopham`, or a placeholder `knapford` service.
10. This spec and the README describe `yard` as the only operator-facing container CLI.

## 9. Out of scope

- renaming the retained internal `tidmouth` engine binary
- replacing the current subprocess spawn contract in the same slice
- a separate Knapford service
- placeholder Knapford compose entries
- compatibility commands or migration aliases
- pushing images to a registry
- multi-arch image support
- distroless/scratch/alpine runtime experiments
- sidecar databases or additional orchestration infrastructure

## 10. References

- `README.md`
- `docs/specs/13_Headless_Run_Command.md`
- `docs/specs/18-unified-yard-cli.md`
- `internal/runtime/orchestrator.go`
- `internal/spawn/spawn_agent.go`
- `ops/llm/docker-compose.yml`

---

**End of spec.** This revision resets the written Phase 7 contract to the no-legacy target state before any code deletion work proceeds.
