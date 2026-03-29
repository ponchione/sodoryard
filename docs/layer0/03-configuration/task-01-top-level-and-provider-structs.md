# Task 01: Top-Level and Provider Config Structs

**Epic:** 03 — Configuration Loading
**Status:** ⬚ Not started
**Dependencies:** Epic 01

---

## Description

Define the root `Config` struct in `internal/config/` with YAML struct tags. Include the top-level fields (project root, log level, log format, server port/host) and the provider routing section (default route, fallback route, provider map with type/base_url/model).

## Acceptance Criteria

- [ ] Root `Config` struct defined with YAML tags
- [ ] Top-level fields: `project_root`, `log_level`, `log_format`, `server_port`, `server_host`
- [ ] `Routing` struct with `Default` and `Fallback` route entries (provider + model)
- [ ] `Providers` map with per-provider `type`, `base_url`, `model` fields
- [ ] Structs compile and are exported from the package
