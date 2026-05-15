import { afterEach, describe, expect, it, vi } from "vitest";
import * as shunterClient from "@shunter/client";
import { resetYardPlatformForTesting, type YardDesktopBridge } from "@/platform";

import {
  assertProjectMemoryContractCompatible,
  assertProjectMemoryRuntimeContractCompatible,
  createProjectMemoryClient,
  fetchProjectMemoryRuntimeContract,
  projectMemoryConnectionErrorMessage,
  projectMemorySubscriptionsUnavailableReason,
  projectMemoryContract,
  projectMemorySubscribeURL,
  queryChainEventsDecoded,
  queryRecentChainEventsDecoded,
  subscribeLiveChainEvents,
  subscribeLiveRecentChainEvents,
  verifyProjectMemoryRuntimeContract,
} from "./client";

function installDesktopBridge(bridge: YardDesktopBridge | undefined) {
  Object.defineProperty(window, "yardDesktop", {
    configurable: true,
    value: bridge,
  });
  resetYardPlatformForTesting();
}

describe("project memory Shunter client", () => {
  afterEach(() => {
    installDesktopBridge(undefined);
  });

  it("builds the subscribe URL under the dedicated project memory route", () => {
    expect(projectMemorySubscribeURL("http://localhost:5173")).toBe(
      "ws://localhost:5173/api/project-memory/subscribe",
    );
    expect(projectMemorySubscribeURL("https://yard.example")).toBe(
      "wss://yard.example/api/project-memory/subscribe",
    );
  });

  it("asserts the checked-in generated contract metadata", () => {
    expect(projectMemoryContract.moduleName).toBe("yard_project_memory");
    expect(projectMemoryContract.moduleVersion).toBe("0.13.0");
    expect(projectMemoryContract.protocol.defaultSubprotocol).toBe("v2.bsatn.shunter");
    expect(projectMemoryContract.protocol.supportedSubprotocols).toContain("v1.bsatn.shunter");
    expect(() => assertProjectMemoryContractCompatible()).not.toThrow();
  });

  it("resolves the vendored Shunter client package", () => {
    expect(shunterClient.createShunterClient).toBeTypeOf("function");
    expect(shunterClient.decodeDeclaredQueryResult).toBeTypeOf("function");
  });

  it("exports the recent chain events SDK helpers", () => {
    expect(queryRecentChainEventsDecoded).toBeTypeOf("function");
    expect(subscribeLiveRecentChainEvents).toBeTypeOf("function");
    expect(queryChainEventsDecoded).toBeTypeOf("function");
    expect(subscribeLiveChainEvents).toBeTypeOf("function");
  });

  it("fetches and verifies the backend contract metadata", async () => {
    const fetcher: typeof fetch = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      expect(input).toBe("/api/project-memory/contract");
      expect(init?.headers).toEqual({ Accept: "application/json" });
      return Promise.resolve(new Response(JSON.stringify({
        module: { name: "yard_project_memory", version: "0.13.0" },
      })));
    });

    await expect(verifyProjectMemoryRuntimeContract(fetcher)).resolves.toMatchObject({
      module: { name: "yard_project_memory", version: "0.13.0" },
    });
  });

  it("uses desktop project-memory URL and token when the preload bridge provides them", () => {
    installDesktopBridge({
      getPlatformInfo: () => ({
        kind: "desktop",
        appVersion: "0.0.0",
        backendBaseUrl: "http://localhost:5173",
        backendDirectUrl: "http://localhost:8090",
        capabilities: ["project_memory_protocol", "project_memory_subscriptions"],
        projectMemory: {
          subscribeUrl: "ws://localhost:5173/api/project-memory/subscribe",
          token: "desktop-token",
        },
      }),
      openExternal: vi.fn(),
    });

    expect(projectMemorySubscribeURL()).toBe("ws://localhost:5173/api/project-memory/subscribe");
    const client = createProjectMemoryClient({ reconnect: false });
    expect(client).toBeTruthy();
  });

  it("explains unavailable desktop live subscriptions before opening a socket", () => {
    installDesktopBridge({
      getPlatformInfo: () => ({
        kind: "desktop",
        appVersion: "0.0.0",
        backendBaseUrl: "http://localhost:5173",
        capabilities: ["project_memory_contract"],
        projectMemory: {
          subscribeUrl: "ws://localhost:5173/api/project-memory/subscribe",
        },
      }),
      openExternal: vi.fn(),
    });

    expect(projectMemorySubscriptionsUnavailableReason()).toBe(
      "Project Memory live subscriptions are not advertised by this backend.",
    );
    expect(() => createProjectMemoryClient({ reconnect: false })).toThrow(
      "Project Memory live subscriptions are not advertised by this backend.",
    );
  });

  it("adds the websocket target to browser transport failures", () => {
    installDesktopBridge({
      getPlatformInfo: () => ({
        kind: "desktop",
        appVersion: "0.0.0",
        backendBaseUrl: "http://localhost:5173",
        capabilities: ["project_memory_protocol", "project_memory_subscriptions"],
        projectMemory: {
          subscribeUrl: "ws://localhost:5173/api/project-memory/subscribe",
        },
      }),
      openExternal: vi.fn(),
    });

    expect(projectMemoryConnectionErrorMessage(new Error("WebSocket failed before opening."))).toBe(
      "WebSocket failed before opening. Target: ws://localhost:5173/api/project-memory/subscribe.",
    );
  });

  it("rejects stale backend contract metadata", () => {
    expect(() => assertProjectMemoryRuntimeContractCompatible({
      module: { name: "yard_project_memory", version: "0.11.0" },
    })).toThrow(/contract mismatch/);
  });

  it("reports failed backend contract fetches", async () => {
    const fetcher: typeof fetch = vi.fn(() => (
      Promise.resolve(new Response("missing", { status: 503, statusText: "Service Unavailable" }))
    ));

    await expect(fetchProjectMemoryRuntimeContract(fetcher)).rejects.toThrow(
      "Project memory contract request failed: 503 Service Unavailable",
    );
  });
});
