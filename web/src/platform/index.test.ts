import { afterEach, describe, expect, it, vi } from "vitest";

import {
  getYardPlatform,
  resetYardPlatformForTesting,
  toBackendURL,
  toBackendWebSocketURL,
  type YardDesktopBridge,
} from "./index";

function installDesktopBridge(bridge: YardDesktopBridge | undefined) {
  Object.defineProperty(window, "yardDesktop", {
    configurable: true,
    value: bridge,
  });
  resetYardPlatformForTesting();
}

describe("Yard platform adapter", () => {
  afterEach(() => {
    installDesktopBridge(undefined);
  });

  it("uses same-origin relative URLs in browser mode", () => {
    installDesktopBridge(undefined);

    expect(getYardPlatform().kind).toBe("browser");
    expect(toBackendURL("/api/health")).toBe("/api/health");
    expect(toBackendWebSocketURL("/api/ws")).toBe("ws://localhost:3000/api/ws");
  });

  it("uses the desktop-provided backend base URL", async () => {
    const openExternal = vi.fn();
    const chooseProjectFiles = vi.fn().mockResolvedValue(["README.md"]);
    installDesktopBridge({
      getPlatformInfo: () => ({
        kind: "desktop",
        appVersion: "0.0.0",
        backendBaseUrl: "http://127.0.0.1:5173",
        backendDirectUrl: "http://127.0.0.1:8090",
        capabilities: ["runtime_status"],
        projectMemory: {
          subscribeUrl: "ws://127.0.0.1:5173/api/project-memory/subscribe",
          token: "token-1",
        },
      }),
      openExternal,
      chooseProjectFiles,
    });

    expect(getYardPlatform().kind).toBe("desktop");
    expect(toBackendURL("/api/health")).toBe("http://127.0.0.1:5173/api/health");
    expect(toBackendWebSocketURL("/api/ws")).toBe("ws://127.0.0.1:5173/api/ws");
    await expect(getYardPlatform().chooseProjectFiles?.()).resolves.toEqual(["README.md"]);
  });
});
