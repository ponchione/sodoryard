import { spawn, type ChildProcess } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { setTimeout as sleep } from "node:timers/promises";

import type {
  BackendLaunchMode,
  DesktopCapabilitiesResponse,
  DesktopPlatformInfo,
  ProjectMemoryTokenResponse,
} from "./types.js";

export interface BackendOptions {
  appVersion: string;
  appPath: string;
  projectDir?: string;
  configPath?: string;
  devMode?: boolean;
}

export interface BackendRuntime {
  mode: BackendLaunchMode;
  directBaseUrl: string;
  rendererBaseUrl: string;
  process?: ChildProcess;
  platform: DesktopPlatformInfo;
}

const defaultBackendPort = 8090;
const defaultRendererURL = "http://localhost:5173";

export async function startBackendRuntime(options: BackendOptions): Promise<BackendRuntime> {
  const directBaseUrl = normalizeBaseURL(
    process.env.YARD_BACKEND_URL || `http://127.0.0.1:${backendPort()}`,
  );
  const rendererURL = process.env.YARD_RENDERER_URL || defaultRendererURL;
  const devMode = options.devMode ?? true;
  const rendererBaseUrl = await selectRendererBaseURL(rendererURL, directBaseUrl, devMode);
  const explicitBackend = Boolean(process.env.YARD_BACKEND_URL);
  let child: ChildProcess | undefined;
  let mode: BackendLaunchMode = "attached";

  if (!(await isHealthy(directBaseUrl))) {
    if (explicitBackend) {
      throw new Error(`Yard backend is not reachable at ${directBaseUrl}`);
    }
    child = spawnBackend(options.appPath, directBaseUrl, {
      configPath: options.configPath,
      devMode,
      projectDir: options.projectDir,
    });
    mode = "managed";
    await waitForHealth(directBaseUrl, 45_000);
  }

  const capabilities = await fetchCapabilities(directBaseUrl);
  const projectMemoryToken = await mintProjectMemoryTokenIfAvailable(directBaseUrl, capabilities);
  const platform = toDesktopPlatformInfo({
    appVersion: options.appVersion,
    directBaseUrl,
    rendererBaseUrl,
    mode,
    capabilities,
    projectMemoryToken,
  });
  return { mode, directBaseUrl, rendererBaseUrl, process: child, platform };
}

export function stopBackendRuntime(runtime: BackendRuntime | undefined) {
  if (!runtime?.process || runtime.process.killed) return;
  runtime.process.kill("SIGTERM");
}

async function selectRendererBaseURL(rendererURL: string, directBaseUrl: string, devMode: boolean): Promise<string> {
  if (!devMode) return directBaseUrl;
  if (process.env.YARD_RENDERER_URL) {
    await waitForHTTP(rendererURL, 45_000);
    return normalizeBaseURL(rendererURL);
  }
  if (await isHTTPAvailable(rendererURL)) {
    return normalizeBaseURL(rendererURL);
  }
  return directBaseUrl;
}

function spawnBackend(
  appPath: string,
  directBaseUrl: string,
  options: {
    configPath?: string;
    devMode: boolean;
    projectDir?: string;
  },
): ChildProcess {
  const yardBinary = resolveYardBinary(appPath);
  const url = new URL(directBaseUrl);
  const args = yardArgs(url, options);
  const projectDir = options.projectDir || process.env.YARD_PROJECT_DIR || resolveRepoRoot(appPath);
  const child = spawn(yardBinary, args, {
    cwd: projectDir,
    env: { ...process.env, NO_COLOR: "1" },
    stdio: ["ignore", "pipe", "pipe"],
  });
  child.stdout?.on("data", (data: Buffer) => process.stdout.write(`[yard] ${data.toString()}`));
  child.stderr?.on("data", (data: Buffer) => process.stderr.write(`[yard] ${data.toString()}`));
  return child;
}

function yardArgs(url: URL, options: { configPath?: string; devMode: boolean }): string[] {
  const args: string[] = [];
  const configPath = options.configPath || process.env.YARD_CONFIG;
  if (configPath) args.push("--config", configPath);
  args.push("serve");
  if (options.devMode) args.push("--dev");
  args.push("--no-open-browser");
  args.push("--host", url.hostname, "--port", String(Number(url.port) || defaultBackendPort));
  return args;
}

function resolveYardBinary(appPath: string): string {
  if (process.env.YARD_BINARY) return process.env.YARD_BINARY;
  const candidates = [
    path.join(resolveRepoRoot(appPath), "bin", process.platform === "win32" ? "yard.exe" : "yard"),
    path.join(resolveRepoRoot(process.cwd()), "bin", process.platform === "win32" ? "yard.exe" : "yard"),
  ];
  for (const candidate of candidates) {
    if (fs.existsSync(candidate)) return candidate;
  }
  return "yard";
}

function resolveRepoRoot(fromPath: string): string {
  const normalized = fs.existsSync(path.join(fromPath, "package.json")) ? fromPath : process.cwd();
  if (path.basename(normalized) === "desktop") return path.dirname(normalized);
  return path.resolve(normalized, "..");
}

function backendPort(): number {
  const parsed = Number(process.env.YARD_BACKEND_PORT || "");
  return Number.isInteger(parsed) && parsed > 0 ? parsed : defaultBackendPort;
}

function normalizeBaseURL(raw: string): string {
  const url = new URL(raw);
  url.pathname = url.pathname.replace(/\/+$/, "");
  url.search = "";
  url.hash = "";
  return url.toString().replace(/\/$/, "");
}

async function waitForHealth(baseUrl: string, timeoutMs: number) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (await isHealthy(baseUrl)) return;
    await sleep(350);
  }
  throw new Error(`Timed out waiting for Yard backend at ${baseUrl}`);
}

async function waitForHTTP(url: string, timeoutMs: number) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (await isHTTPAvailable(url)) return;
    await sleep(350);
  }
  throw new Error(`Timed out waiting for renderer at ${url}`);
}

async function isHealthy(baseUrl: string): Promise<boolean> {
  try {
    const response = await fetch(new URL("/api/health", baseUrl));
    return response.ok;
  } catch {
    return false;
  }
}

async function isHTTPAvailable(rawURL: string): Promise<boolean> {
  try {
    const response = await fetch(rawURL);
    return response.ok;
  } catch {
    return false;
  }
}

async function fetchCapabilities(baseUrl: string): Promise<DesktopCapabilitiesResponse> {
  const response = await fetch(new URL("/api/desktop/capabilities", baseUrl), {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(`Desktop capabilities request failed: ${response.status} ${response.statusText}`);
  }
  return response.json() as Promise<DesktopCapabilitiesResponse>;
}

async function mintProjectMemoryTokenIfAvailable(
  baseUrl: string,
  capabilities: DesktopCapabilitiesResponse,
): Promise<string | undefined> {
  if (!capabilities.capabilities?.includes("project_memory_protocol")) return undefined;
  const response = await fetch(new URL("/api/project-memory/token", baseUrl), {
    method: "POST",
    headers: { Accept: "application/json" },
  });
  if (!response.ok) return undefined;
  const payload = await response.json() as ProjectMemoryTokenResponse;
  return payload.token;
}

function toDesktopPlatformInfo(input: {
  appVersion: string;
  directBaseUrl: string;
  rendererBaseUrl: string;
  mode: BackendLaunchMode;
  capabilities: DesktopCapabilitiesResponse;
  projectMemoryToken?: string;
}): DesktopPlatformInfo {
  const projectMemory = input.capabilities.project_memory;
  return {
    kind: "desktop",
    appVersion: input.appVersion,
    yardVersion: input.capabilities.yard_version,
    apiVersion: input.capabilities.api_version,
    backendBaseUrl: input.rendererBaseUrl,
    backendDirectUrl: input.directBaseUrl,
    backendLaunchMode: input.mode,
    projectRoot: input.capabilities.project_root,
    configPath: input.capabilities.config_path,
    recentProjectRoot: input.capabilities.project_root,
    capabilities: input.capabilities.capabilities ?? [],
    projectMemory: projectMemory ? {
      backend: projectMemory.backend,
      module: projectMemory.module,
      schemaVersion: projectMemory.schema_version,
      contractVersion: projectMemory.contract_version,
      shunterVersion: projectMemory.shunter_version,
      defaultSubprotocol: projectMemory.default_subprotocol,
      supportedSubprotocols: projectMemory.supported_subprotocols,
      subscribeUrl: projectMemorySubscribeURL(input.rendererBaseUrl),
      token: input.projectMemoryToken,
      generatedBindingHash: projectMemory.generated_binding_hash,
    } : undefined,
  };
}

function projectMemorySubscribeURL(baseUrl: string): string {
  const url = new URL("/api/project-memory/subscribe", baseUrl);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url.toString();
}
