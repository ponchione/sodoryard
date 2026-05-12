import {
  assertGeneratedContractCompatible,
  createShunterClient,
  type ConnectionStateListener,
  type ReconnectOptions,
  type ShunterClient,
  type TokenSource,
} from "@shunter/client";

import {
  shunterContract,
  shunterProtocol,
  type ShunterSubprotocol,
} from "@/generated/yard-project-memory";

export {
  queryRecentChainsDecoded,
  subscribeLiveRecentChains,
  subscribeLiveRecentChainsHandle,
} from "@/generated/yard-project-memory";

export type {
  ChainsRow,
  LiveRecentChainsViewRow,
  RecentChainsQueryRow,
  RecentChainsQueryRows,
} from "@/generated/yard-project-memory";

export const projectMemoryContract = shunterContract;

export interface ProjectMemoryClientOptions {
  url?: string;
  token?: TokenSource;
  reconnect?: ReconnectOptions | false;
  onStateChange?: ConnectionStateListener<typeof shunterProtocol>;
}

export type ProjectMemoryClient = ShunterClient<typeof shunterProtocol>;

export function assertProjectMemoryContractCompatible() {
  return assertGeneratedContractCompatible(shunterContract, {
    moduleName: "yard_project_memory",
    moduleVersion: "0.12.0",
  });
}

export function createProjectMemoryClient(options: ProjectMemoryClientOptions = {}): ProjectMemoryClient {
  assertProjectMemoryContractCompatible();
  return createShunterClient({
    url: options.url ?? projectMemorySubscribeURL(),
    protocol: shunterProtocol,
    contract: shunterContract,
    token: options.token,
    reconnect: options.reconnect ?? { enabled: true, resubscribe: true },
    onStateChange: options.onStateChange,
  });
}

export function projectMemorySubscribeURL(origin = window.location.origin): string {
  const url = new URL("/api/project-memory/subscribe", origin);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url.toString();
}

export type ProjectMemorySubprotocol = ShunterSubprotocol;
