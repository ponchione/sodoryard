# Yard Desktop

Electron shell for the Yard web app and local Go backend.

Development entrypoint:

```bash
make bootstrap
make desktop-dev
```

`make bootstrap` checks the required Go/Node/CGO/LanceDB source-build
prerequisites, installs the web and desktop npm dependencies, and verifies that
the generated Project Memory bindings are current.

The dev command builds `bin/yard`, starts or attaches to a local `yard serve --dev`
backend on `127.0.0.1:8090`, waits for the Vite renderer on
`http://127.0.0.1:5173`, and exposes desktop runtime metadata to the renderer
through the preload bridge.

The shell stores window bounds and the last selected project root in Electron's
user data directory. `YARD_PROJECT_DIR` still wins for development runs.
Electron opens the React app on `/dashboard`; browser mode can still use the
existing chat route at `/`. The dashboard links into the `/launch` preview
workbench for assembling, previewing, and starting backend launch requests.

Useful environment overrides:

```bash
YARD_BACKEND_URL=http://127.0.0.1:8090  # attach instead of spawning
YARD_RENDERER_URL=http://127.0.0.1:5173 # renderer dev server
YARD_BINARY=/path/to/yard               # sidecar binary
YARD_PROJECT_DIR=/path/to/project       # backend working directory
YARD_CONFIG=/path/to/yard.yaml          # backend config
```

If `127.0.0.1:5173` is busy, `make desktop-dev` chooses the next free renderer
port and passes that URL to Electron. Use
`make desktop-dev DESKTOP_RENDERER_PORT=<port>` to choose a different starting
port.

Validation:

```bash
make doctor-dev
make desktop-test
make desktop-build
make desktop-package
make desktop-install-user
```

`make desktop-package` creates a local unpacked package at
`desktop/out/Yard-linux-x64/` on Linux. The `yard-desktop` launcher sets the
sidecar binary path and library path before starting Electron in packaged mode.
This is the supported source-first desktop artifact for active development.

On Linux, `make desktop-install-user` builds that unpacked package and installs
a user-local `yard-desktop.desktop` launcher plus app icon under
`~/.local/share/`. After it runs, open the application menu for "Yard Desktop"
and pin it to the taskbar or dock. Re-run the same target after source changes
to rebuild the app in place while keeping the pinned launcher stable.
