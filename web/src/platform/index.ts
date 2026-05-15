import { createBrowserPlatform } from "./browser";
import { createDesktopPlatform } from "./desktop";
import type { YardPlatform } from "./types";

let cachedPlatform: YardPlatform | null = null;

export function getYardPlatform(): YardPlatform {
  cachedPlatform ??= createDesktopPlatform() ?? createBrowserPlatform();
  return cachedPlatform;
}

export function resetYardPlatformForTesting() {
  cachedPlatform = null;
}

export function toBackendURL(pathOrURL: string): string {
  if (isAbsoluteURL(pathOrURL)) return pathOrURL;
  const baseURL = getYardPlatform().backendBaseUrl;
  if (!baseURL) return pathOrURL;
  return new URL(pathOrURL, withTrailingSlash(baseURL)).toString();
}

export function toBackendWebSocketURL(pathOrURL: string): string {
  if (pathOrURL.startsWith("ws://") || pathOrURL.startsWith("wss://")) return pathOrURL;
  const baseURL = getYardPlatform().backendBaseUrl || window.location.origin;
  const url = new URL(pathOrURL, withTrailingSlash(baseURL));
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url.toString();
}

export function desktopSessionHeaders(): HeadersInit {
  const token = getYardPlatform().desktopSessionToken;
  return token ? { "X-Yard-Desktop-Session": token } : {};
}

function isAbsoluteURL(value: string): boolean {
  try {
    new URL(value);
    return true;
  } catch {
    return false;
  }
}

function withTrailingSlash(baseURL: string): string {
  return baseURL.endsWith("/") ? baseURL : `${baseURL}/`;
}

export type {
  YardDesktopBridge,
  YardDesktopPlatformInfo,
  YardPlatform,
  YardProjectMemoryPlatform,
} from "./types";
