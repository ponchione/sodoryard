// @vitest-environment node
import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  createProjectMemoryClient,
  queryRecentChainEventsDecoded,
  queryRecentChainsDecoded,
  verifyProjectMemoryRuntimeContract,
  type ProjectMemoryClient,
} from "@/lib/project-memory/client";

declare const process: { env: Record<string, string | undefined> };

const baseURL = process.env.PROJECT_MEMORY_SMOKE_BASE_URL ?? "";
const chainID = process.env.PROJECT_MEMORY_SMOKE_CHAIN_ID ?? "projectmemory-sdk-smoke-chain";
const describeSmoke = baseURL ? describe : describe.skip;

function absoluteFetch(base: string): typeof fetch {
  return ((input: RequestInfo | URL, init?: RequestInit) => {
    const target = typeof input === "string" ? new URL(input, base).toString() : input;
    return fetch(target, init);
  }) as typeof fetch;
}

function subscribeURL(base: string): string {
  const url = new URL("/api/project-memory/subscribe", base);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url.toString();
}

describeSmoke("Project Memory Shunter SDK runtime smoke", () => {
  let client: ProjectMemoryClient | null = null;

  beforeAll(async () => {
    const contract = await verifyProjectMemoryRuntimeContract(absoluteFetch(baseURL));
    expect(contract.module?.name).toBe("yard_project_memory");
    expect(contract.module?.version).toBe("0.13.0");

    client = createProjectMemoryClient({
      url: subscribeURL(baseURL),
      reconnect: false,
    });
    await client.connect();
  });

  afterAll(async () => {
    await client?.dispose();
  });

  it("runs generated declared query helpers against the mounted runtime", async () => {
    if (!client) throw new Error("project memory client was not connected");

    const [chainsResult, eventsResult] = await Promise.all([
      queryRecentChainsDecoded(client.runDeclaredQuery),
      queryRecentChainEventsDecoded(client.runDeclaredQuery),
    ]);

    const chainRows = chainsResult.tables.find((table) => table.tableName === "chains")?.rows ?? [];
    const eventRows = eventsResult.tables.find((table) => table.tableName === "events")?.rows ?? [];

    expect(chainRows.some((row) => row.id === chainID)).toBe(true);
    expect(eventRows.some((row) => row.chainId === chainID && row.eventType === "step_started")).toBe(true);
    expect(eventRows.some((row) => row.chainId === chainID && row.eventType === "approval_required")).toBe(true);
  });
});
