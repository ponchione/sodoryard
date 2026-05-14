# Yard Desktop

Electron shell for the Yard web app and local Go backend.

Development entrypoint:

```bash
make desktop-dev
```

The dev command builds `bin/yard`, starts or attaches to a local `yard serve --dev`
backend on `127.0.0.1:8090`, waits for the Vite renderer on
`http://localhost:5173` when it is already running, and exposes desktop runtime
metadata to the renderer through the preload bridge.

Useful environment overrides:

```bash
YARD_BACKEND_URL=http://127.0.0.1:8090  # attach instead of spawning
YARD_RENDERER_URL=http://localhost:5173 # renderer dev server
YARD_BINARY=/path/to/yard               # sidecar binary
YARD_PROJECT_DIR=/path/to/project       # backend working directory
YARD_CONFIG=/path/to/yard.yaml          # backend config
```
