import { contextBridge, ipcRenderer } from "electron";

import type { YardNotification } from "./types.js";

contextBridge.exposeInMainWorld("yardDesktop", {
  getPlatformInfo: () => ipcRenderer.sendSync("yard:getPlatformInfo"),
  openExternal: (url: string) => ipcRenderer.invoke("yard:openExternal", url),
  chooseProjectDirectory: () => ipcRenderer.invoke("yard:chooseProjectDirectory"),
  chooseProjectFiles: () => ipcRenderer.invoke("yard:chooseProjectFiles"),
  notify: (notification: YardNotification) => ipcRenderer.invoke("yard:notify", notification),
});
