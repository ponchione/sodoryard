import { describe, expect, it, vi } from "vitest";
import * as shunterClient from "@shunter/client";

import {
  assertProjectMemoryContractCompatible,
  assertProjectMemoryRuntimeContractCompatible,
  fetchProjectMemoryRuntimeContract,
  projectMemoryContract,
  projectMemorySubscribeURL,
  queryRecentChainEventsDecoded,
  subscribeLiveRecentChainEvents,
  verifyProjectMemoryRuntimeContract,
} from "./client";

describe("project memory Shunter client", () => {
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
    expect(() => assertProjectMemoryContractCompatible()).not.toThrow();
  });

  it("resolves the vendored Shunter client package", () => {
    expect(shunterClient.createShunterClient).toBeTypeOf("function");
    expect(shunterClient.decodeDeclaredQueryResult).toBeTypeOf("function");
  });

  it("exports the recent chain events SDK helpers", () => {
    expect(queryRecentChainEventsDecoded).toBeTypeOf("function");
    expect(subscribeLiveRecentChainEvents).toBeTypeOf("function");
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
