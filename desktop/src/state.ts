import fs from "node:fs";
import path from "node:path";

import { normalizeZoomFactor } from "./zoom.js";

export interface DesktopWindowState {
  x?: number;
  y?: number;
  width: number;
  height: number;
  maximized?: boolean;
}

export interface DesktopState {
  recentProjectRoot?: string;
  window?: DesktopWindowState;
  zoomFactor?: number;
}

const stateFilename = "desktop-state.json";
const minWindowWidth = 980;
const minWindowHeight = 680;

export function readDesktopState(userDataDir: string): DesktopState {
  try {
    const raw = fs.readFileSync(statePath(userDataDir), "utf8");
    return normalizeDesktopState(JSON.parse(raw) as unknown);
  } catch {
    return {};
  }
}

export function writeDesktopState(userDataDir: string, state: DesktopState) {
  fs.mkdirSync(userDataDir, { recursive: true });
  fs.writeFileSync(statePath(userDataDir), `${JSON.stringify(normalizeDesktopState(state), null, 2)}\n`, "utf8");
}

export function updateDesktopState(userDataDir: string, patch: Partial<DesktopState>): DesktopState {
  const next = normalizeDesktopState({ ...readDesktopState(userDataDir), ...patch });
  writeDesktopState(userDataDir, next);
  return next;
}

export function normalizeDesktopState(value: unknown): DesktopState {
  if (!isRecord(value)) return {};
  const state: DesktopState = {};
  if (typeof value.recentProjectRoot === "string" && value.recentProjectRoot.trim() !== "") {
    state.recentProjectRoot = value.recentProjectRoot.trim();
  }
  const windowState = normalizeWindowState(value.window);
  if (windowState) state.window = windowState;
  const zoomFactor = normalizeZoomFactor(value.zoomFactor);
  if (zoomFactor !== undefined) state.zoomFactor = zoomFactor;
  return state;
}

export function normalizeWindowState(value: unknown): DesktopWindowState | undefined {
  if (!isRecord(value)) return undefined;
  const width = normalizeDimension(value.width, minWindowWidth);
  const height = normalizeDimension(value.height, minWindowHeight);
  const out: DesktopWindowState = { width, height };
  const x = normalizePosition(value.x);
  const y = normalizePosition(value.y);
  if (x !== undefined) out.x = x;
  if (y !== undefined) out.y = y;
  if (typeof value.maximized === "boolean") out.maximized = value.maximized;
  return out;
}

function statePath(userDataDir: string): string {
  return path.join(userDataDir, stateFilename);
}

function normalizeDimension(value: unknown, minimum: number): number {
  return typeof value === "number" && Number.isFinite(value) ? Math.max(minimum, Math.trunc(value)) : minimum;
}

function normalizePosition(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? Math.trunc(value) : undefined;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}
