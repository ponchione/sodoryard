import { app, BrowserWindow, dialog, ipcMain, Notification, shell } from "electron";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { startBackendRuntime, stopBackendRuntime, type BackendRuntime } from "./backend.js";
import type { YardNotification } from "./types.js";

const currentFile = fileURLToPath(import.meta.url);
const currentDir = path.dirname(currentFile);
let mainWindow: BrowserWindow | undefined;
let runtime: BackendRuntime | undefined;

async function main() {
  await app.whenReady();
  registerIPCHandlers();
  mainWindow = createWindow();
  await showStatus("Starting Yard Desktop", "Starting the local Yard backend...");
  try {
    runtime = await startBackendRuntime({
      appVersion: app.getVersion(),
      appPath: app.getAppPath(),
    });
    await showStatus("Starting Yard Desktop", "Opening the renderer...");
    await mainWindow.loadURL(runtime.rendererBaseUrl);
  } catch (error) {
    const message = error instanceof Error ? error.message : "Unknown startup error";
    await showStatus("Yard Desktop failed to start", message);
  }
}

function createWindow(): BrowserWindow {
  return new BrowserWindow({
    width: 1320,
    height: 900,
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
      properties: ["openDirectory"],
      title: "Open Yard project",
    });
    return result.canceled ? null : result.filePaths[0] ?? null;
  });
  ipcMain.handle("yard:notify", async (_event, notification: YardNotification) => {
    if (!Notification.isSupported()) return;
    new Notification({
      title: notification.title,
      body: notification.body,
    }).show();
  });
}

function escapeHTML(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

app.on("window-all-closed", () => {
  stopBackendRuntime(runtime);
  if (process.platform !== "darwin") app.quit();
});

app.on("before-quit", () => {
  stopBackendRuntime(runtime);
});

app.on("activate", () => {
  if (BrowserWindow.getAllWindows().length === 0) {
    mainWindow = createWindow();
    if (runtime) void mainWindow.loadURL(runtime.rendererBaseUrl);
  }
});

void main();
