import type { YardPlatform } from "./types";

export function createDesktopPlatform(): YardPlatform | null {
  if (typeof window === "undefined") return null;
  const bridge = window.yardDesktop;
  const info = bridge?.getPlatformInfo();
  if (!bridge || !info) return null;
  return {
    kind: "desktop",
    backendBaseUrl: info.backendBaseUrl,
    desktopSessionToken: info.desktopSessionToken,
    capabilities: info.capabilities,
    projectMemory: info.projectMemory,
    openExternal: (url) => bridge.openExternal(url),
    chooseProjectDirectory: bridge.chooseProjectDirectory ? () => bridge.chooseProjectDirectory!() : undefined,
    chooseProjectFiles: bridge.chooseProjectFiles ? () => bridge.chooseProjectFiles!() : undefined,
    openProjectPath: bridge.openProjectPath ? (path) => bridge.openProjectPath!(path) : undefined,
    revealProjectPath: bridge.revealProjectPath ? (path) => bridge.revealProjectPath!(path) : undefined,
    notify: bridge.notify ? (notification) => bridge.notify!(notification) : undefined,
    getAppInfo: async () => info,
  };
}
