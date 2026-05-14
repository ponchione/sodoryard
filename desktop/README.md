# Yard Desktop

Electron shell for the Yard web app and local Go backend.

Development entrypoint:

```bash
make desktop-dev
```

The dev command builds `bin/yard`, starts or attaches to a local `yard serve --dev`
backend on `127.0.0.1:8090`, waits for the Vite renderer on
`http://localhost:5173`, and exposes desktop runtime metadata to the renderer
through the preload bridge.

The shell stores window bounds and the last selected project root in Electron's
user data directory. `YARD_PROJECT_DIR` still wins for development runs.
Electron opens the React app on `/dashboard`; browser mode can still use the
existing chat route at `/`. The dashboard links into the `/launch` preview
workbench for assembling, previewing, and starting backend launch requests.

Useful environment overrides:

```bash
YARD_BACKEND_URL=http://127.0.0.1:8090  # attach instead of spawning
YARD_RENDERER_URL=http://localhost:5173 # renderer dev server
YARD_BINARY=/path/to/yard               # sidecar binary
YARD_PROJECT_DIR=/path/to/project       # backend working directory
YARD_CONFIG=/path/to/yard.yaml          # backend config
```

Validation:

```bash
make desktop-test
make desktop-build
make desktop-package
```

`make desktop-package` creates a local unpacked package at
`desktop/out/Yard-linux-x64/` on Linux. The `yard-desktop` launcher sets the
sidecar binary path and library path before starting Electron in packaged mode.
This is not an installer or AppImage yet.
