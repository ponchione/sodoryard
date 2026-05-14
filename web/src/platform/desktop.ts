import type { YardPlatform } from "./types";

export function createDesktopPlatform(): YardPlatform | null {
  const bridge = window.yardDesktop;
  const info = bridge?.getPlatformInfo();
  if (!bridge || !info) return null;
  return {
    kind: "desktop",
    backendBaseUrl: info.backendBaseUrl,
    desktopSessionToken: info.desktopSessionToken,
    projectMemory: info.projectMemory,
    openExternal: (url) => bridge.openExternal(url),
    chooseProjectDirectory: bridge.chooseProjectDirectory ? () => bridge.chooseProjectDirectory!() : undefined,
    chooseProjectFiles: bridge.chooseProjectFiles ? () => bridge.chooseProjectFiles!() : undefined,
    notify: bridge.notify ? (notification) => bridge.notify!(notification) : undefined,
    getAppInfo: async () => info,
  };
}
