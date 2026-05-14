import { app, BrowserWindow, dialog, ipcMain, Notification, shell } from "electron";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { startBackendRuntime, stopBackendRuntime, type BackendRuntime } from "./backend.js";
import { toProjectAbsolutePath, toProjectRelativeFilePaths } from "./project-paths.js";
import {
  readDesktopState,
  updateDesktopState,
  type DesktopState,
  type DesktopWindowState,
} from "./state.js";
import type { YardNotification } from "./types.js";
import {
  defaultZoomFactor,
  nextZoomFactor,
  normalizeZoomFactor,
  zoomCommandForInput,
} from "./zoom.js";

const currentFile = fileURLToPath(import.meta.url);
const currentDir = path.dirname(currentFile);
let mainWindow: BrowserWindow | undefined;
let runtime: BackendRuntime | undefined;
let userDataDir = "";
let desktopState: DesktopState = {};
let quitting = false;

interface ValidatePathsResponse {
  accepted: string[];
  rejected: Array<{ path: string; reason: string }>;
}

async function main() {
  await app.whenReady();
  const packagedMode = process.env.YARD_DESKTOP_PACKAGED === "1";
  userDataDir = app.getPath("userData");
  desktopState = readDesktopState(userDataDir);
  registerIPCHandlers();
  mainWindow = createWindow(desktopState.window, desktopState.zoomFactor);
  bindWindowLifecycle(mainWindow);
  await showStatus("Starting Yard Desktop", "Starting the local Yard backend...");
  try {
    runtime = await startBackendRuntime({
      appVersion: app.getVersion(),
      appPath: app.getAppPath(),
      devMode: !packagedMode,
      projectDir: process.env.YARD_PROJECT_DIR || desktopState.recentProjectRoot,
      configPath: process.env.YARD_CONFIG,
    });
    rememberProjectRoot(runtime.platform.projectRoot);
    watchManagedBackend(runtime);
    await showStatus("Starting Yard Desktop", "Opening the renderer...");
    await mainWindow.loadURL(rendererRouteURL(runtime.rendererBaseUrl, "/dashboard"));
  } catch (error) {
    const message = error instanceof Error ? error.message : "Unknown startup error";
    await showStatus("Yard Desktop failed to start", message);
  }
}

function createWindow(windowState?: DesktopWindowState, zoomFactor = defaultZoomFactor): BrowserWindow {
  const win = new BrowserWindow({
    x: windowState?.x,
    y: windowState?.y,
    width: windowState?.width ?? 1320,
    height: windowState?.height ?? 900,
    minWidth: 980,
    minHeight: 680,
    title: "Yard",
    show: true,
    webPreferences: {
      preload: path.join(currentDir, "preload.js"),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
    },
  });
  applyWindowZoom(win, zoomFactor);
  if (windowState?.maximized) win.maximize();
  return win;
}

function bindWindowLifecycle(win: BrowserWindow) {
  win.on("close", () => persistWindowState(win));
  win.on("closed", () => {
    if (mainWindow === win) mainWindow = undefined;
  });
  win.webContents.on("before-input-event", (event, input) => {
    const command = zoomCommandForInput(input);
    if (!command) return;
    event.preventDefault();
    setWindowZoom(win, nextZoomFactor(win.webContents.getZoomFactor(), command));
  });
}

async function showStatus(title: string, detail: string) {
  if (!mainWindow) return;
  const html = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>${escapeHTML(title)}</title>
    <style>
      :root {
        color-scheme: dark;
        font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
        background: #111315;
        color: #f7f4ec;
      }
      body {
        align-items: center;
        display: flex;
        justify-content: center;
        margin: 0;
        min-height: 100vh;
      }
      main {
        max-width: 680px;
        padding: 40px;
      }
      h1 {
        font-size: 26px;
        font-weight: 650;
        letter-spacing: 0;
        margin: 0 0 12px;
      }
      p {
        color: #c9c3b6;
        font-size: 15px;
        line-height: 1.6;
        margin: 0;
      }
    </style>
  </head>
  <body>
    <main>
      <h1>${escapeHTML(title)}</h1>
      <p>${escapeHTML(detail)}</p>
    </main>
  </body>
</html>`;
  await mainWindow.loadURL(`data:text/html;charset=utf-8,${encodeURIComponent(html)}`);
}

function registerIPCHandlers() {
  ipcMain.on("yard:getPlatformInfo", (event) => {
    event.returnValue = runtime?.platform ?? null;
  });
  ipcMain.handle("yard:openExternal", async (_event, rawURL: string) => {
    const url = new URL(String(rawURL));
    if (!["http:", "https:", "mailto:"].includes(url.protocol)) {
      throw new Error(`Unsupported external URL protocol: ${url.protocol}`);
    }
    await shell.openExternal(url.toString());
  });
  ipcMain.handle("yard:chooseProjectDirectory", async () => {
    const result = await dialog.showOpenDialog({
      defaultPath: desktopState.recentProjectRoot ?? runtime?.platform.projectRoot,
      properties: ["openDirectory"],
      title: "Open Yard project",
    });
    const selected = result.canceled ? null : result.filePaths[0] ?? null;
    if (selected) rememberProjectRoot(selected);
    return selected;
  });
  ipcMain.handle("yard:chooseProjectFiles", async () => {
    const projectRoot = runtime?.platform.projectRoot ?? desktopState.recentProjectRoot;
    const result = await dialog.showOpenDialog({
      defaultPath: projectRoot,
      properties: ["openFile", "multiSelections"],
      title: "Attach project files",
    });
    if (result.canceled) return [];
    return toProjectRelativeFilePaths(projectRoot, result.filePaths);
  });
  ipcMain.handle("yard:openProjectPath", async (_event, projectPath: string) => {
    const acceptedPath = await validateProjectPath(projectPath, "open_editor");
    const absPath = resolveProjectPath(acceptedPath);
    const error = await shell.openPath(absPath);
    if (error) throw new Error(error);
  });
  ipcMain.handle("yard:revealProjectPath", async (_event, projectPath: string) => {
    const acceptedPath = await validateProjectPath(projectPath, "reveal");
    const absPath = resolveProjectPath(acceptedPath);
    shell.showItemInFolder(absPath);
  });
  ipcMain.handle("yard:notify", async (_event, notification: YardNotification) => {
    if (!Notification.isSupported()) return;
    new Notification({
      title: notification.title,
      body: notification.body,
    }).show();
  });
}

function resolveProjectPath(projectPath: string): string {
  const projectRoot = runtime?.platform.projectRoot ?? desktopState.recentProjectRoot;
  const absPath = toProjectAbsolutePath(projectRoot, String(projectPath));
  if (!absPath) throw new Error("Invalid project path");
  return absPath;
}

async function validateProjectPath(projectPath: string, purpose: "open_editor" | "reveal"): Promise<string> {
  if (!runtime) throw new Error("Yard backend is not ready");
  const response = await fetch(new URL("/api/project/validate-paths", runtime.directBaseUrl), {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ purpose, paths: [String(projectPath)] }),
  });
  if (!response.ok) {
    throw new Error(`Project path validation failed: ${response.status} ${response.statusText}`);
  }
  const payload = await response.json() as ValidatePathsResponse;
  if (payload.accepted.length > 0) return payload.accepted[0];
  const rejection = payload.rejected[0];
  throw new Error(rejection ? `${rejection.path}: ${rejection.reason}` : "Project path was not accepted");
}

function escapeHTML(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function watchManagedBackend(current: BackendRuntime) {
  const child = current.process;
  if (!child) return;
  child.once("exit", (code, signal) => {
    if (runtime?.process === child) runtime.process = undefined;
    if (quitting) return;
    const reason = signal ? `signal ${signal}` : `exit code ${code ?? "unknown"}`;
    void showStatus(
      "Yard backend stopped",
      `The managed backend exited with ${reason}. Close and reopen Yard Desktop to start it again.`,
    );
  });
}

function rememberProjectRoot(projectRoot: string | undefined) {
  if (!projectRoot || userDataDir === "") return;
  desktopState = updateDesktopState(userDataDir, { recentProjectRoot: projectRoot });
}

function persistWindowState(win = mainWindow) {
  if (!win || win.isDestroyed() || userDataDir === "") return;
  const bounds = win.getNormalBounds();
  desktopState = updateDesktopState(userDataDir, {
    window: {
      x: bounds.x,
      y: bounds.y,
      width: bounds.width,
      height: bounds.height,
      maximized: win.isMaximized(),
    },
  });
}

function applyWindowZoom(win: BrowserWindow, zoomFactor: unknown) {
  win.webContents.setZoomFactor(normalizeZoomFactor(zoomFactor) ?? defaultZoomFactor);
}

function setWindowZoom(win: BrowserWindow, zoomFactor: unknown) {
  const next = normalizeZoomFactor(zoomFactor) ?? defaultZoomFactor;
  win.webContents.setZoomFactor(next);
  if (userDataDir !== "") {
    desktopState = updateDesktopState(userDataDir, { zoomFactor: next });
  }
}

app.on("window-all-closed", () => {
  quitting = true;
  stopBackendRuntime(runtime);
  if (process.platform !== "darwin") app.quit();
});

app.on("before-quit", () => {
  quitting = true;
  persistWindowState();
  stopBackendRuntime(runtime);
});

app.on("activate", () => {
  if (BrowserWindow.getAllWindows().length === 0) {
    mainWindow = createWindow(desktopState.window, desktopState.zoomFactor);
    bindWindowLifecycle(mainWindow);
    if (runtime) void mainWindow.loadURL(rendererRouteURL(runtime.rendererBaseUrl, "/dashboard"));
  }
});

void main();

function rendererRouteURL(baseUrl: string, route: string): string {
  return new URL(route, baseUrl.endsWith("/") ? baseUrl : `${baseUrl}/`).toString();
}
