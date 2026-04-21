@RTK.md

- Keep edits narrow; do not refactor unrelated code.
- Read `README.md` (especially the current status / next session section) before resuming in-flight work.
- Prefer `make test` and `make build`; they carry the required CGO/LanceDB settings.
- If running Go directly, use `-tags sqlite_fts5`.
- Backend entrypoint: `./cmd/tidmouth`. Frontend: `web/`. Production build embeds `web/dist` into `webfs/dist`.
- Local dev: `make dev-backend` and `make dev-frontend`.
- For retrieval/runtime validation, build the index before `serve`.
- Confirm the configured provider/model before websocket or runtime smoke tests.
- Treat `yard.yaml`, `.yard/`, and `.brain/` as project-local config/state; avoid churn unless the task requires it.
