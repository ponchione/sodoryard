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
import { getYardPlatform, toBackendURL, toBackendWebSocketURL } from "@/platform";

export {
  decodeEventsRow,
  queryChainEventsDecoded,
  queryRecentChainEventsDecoded,
  queryRecentChainsDecoded,
  subscribeLiveChainEvents,
  subscribeLiveChainEventsHandle,
  subscribeLiveRecentChainEvents,
  subscribeLiveRecentChainEventsHandle,
  subscribeLiveRecentChains,
  subscribeLiveRecentChainsHandle,
} from "@/generated/yard-project-memory";

export type {
  ChainEventsParams,
  ChainEventsQueryRow,
  ChainEventsQueryRows,
  ChainsRow,
  EventsRow,
  LiveChainEventsParams,
  LiveChainEventsViewRow,
  LiveRecentChainEventsViewRow,
  LiveRecentChainsViewRow,
  RecentChainEventsQueryRow,
  RecentChainEventsQueryRows,
  RecentChainsQueryRow,
  RecentChainsQueryRows,
} from "@/generated/yard-project-memory";

export const projectMemoryContract = shunterContract;
export const projectMemoryModuleName = "yard_project_memory";
export const projectMemoryModuleVersion = "0.13.0";

export interface ProjectMemoryClientOptions {
  url?: string;
  token?: TokenSource;
  reconnect?: ReconnectOptions | false;
  onStateChange?: ConnectionStateListener<typeof shunterProtocol>;
}

export type ProjectMemoryClient = ShunterClient<typeof shunterProtocol>;

export interface ProjectMemoryRuntimeContract {
  module?: {
    name?: string;
    version?: string;
  };
}

export function assertProjectMemoryContractCompatible() {
  return assertGeneratedContractCompatible(shunterContract, {
    moduleName: projectMemoryModuleName,
    moduleVersion: projectMemoryModuleVersion,
  });
}

export async function fetchProjectMemoryRuntimeContract(fetcher: typeof fetch = fetch): Promise<ProjectMemoryRuntimeContract> {
  const response = await fetcher(toBackendURL("/api/project-memory/contract"), {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(`Project memory contract request failed: ${response.status} ${response.statusText}`);
  }
  return response.json() as Promise<ProjectMemoryRuntimeContract>;
}

export function assertProjectMemoryRuntimeContractCompatible(
  contract: ProjectMemoryRuntimeContract,
): ProjectMemoryRuntimeContract {
  const moduleName = contract.module?.name;
  const moduleVersion = contract.module?.version;
  if (moduleName !== projectMemoryModuleName || moduleVersion !== projectMemoryModuleVersion) {
    throw new Error(
      `Project memory contract mismatch: expected ${projectMemoryModuleName} ${projectMemoryModuleVersion}, got ${moduleName ?? "unknown"} ${moduleVersion ?? "unknown"}`,
    );
  }
  return contract;
}

export async function verifyProjectMemoryRuntimeContract(fetcher?: typeof fetch): Promise<ProjectMemoryRuntimeContract> {
  return assertProjectMemoryRuntimeContractCompatible(await fetchProjectMemoryRuntimeContract(fetcher));
}

export function createProjectMemoryClient(options: ProjectMemoryClientOptions = {}): ProjectMemoryClient {
  assertProjectMemoryContractCompatible();
  return createShunterClient({
    url: options.url ?? projectMemorySubscribeURL(),
    protocol: shunterProtocol,
    contract: shunterContract,
    token: options.token ?? getYardPlatform().projectMemory?.token,
    reconnect: options.reconnect ?? { enabled: true, resubscribe: true },
    onStateChange: options.onStateChange,
  });
}

export function projectMemorySubscribeURL(origin?: string): string {
  if (!origin) {
    const configuredURL = getYardPlatform().projectMemory?.subscribeUrl;
    if (configuredURL) return configuredURL;
    return toBackendWebSocketURL("/api/project-memory/subscribe");
  }
  const originURL = new URL(origin);
  const base = `${originURL.protocol}//${originURL.host}`;
  const url = new URL("/api/project-memory/subscribe", base);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url.toString();
}

export type ProjectMemorySubprotocol = ShunterSubprotocol;
