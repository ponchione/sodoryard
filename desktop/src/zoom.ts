import type { Input } from "electron";

export const defaultZoomFactor = 1;
export const minZoomFactor = 0.7;
export const maxZoomFactor = 1.8;
export const zoomStep = 0.1;

export type ZoomCommand = "in" | "out" | "reset";

type ZoomShortcutInput = Pick<Input, "type" | "key" | "code" | "control" | "meta" | "alt">;

export function normalizeZoomFactor(value: unknown): number | undefined {
  if (typeof value !== "number" || !Number.isFinite(value)) return undefined;
  return roundZoom(Math.min(maxZoomFactor, Math.max(minZoomFactor, value)));
}

export function nextZoomFactor(current: unknown, command: ZoomCommand): number {
  const value = normalizeZoomFactor(current) ?? defaultZoomFactor;
  switch (command) {
    case "in":
      return normalizeZoomFactor(value + zoomStep) ?? defaultZoomFactor;
    case "out":
      return normalizeZoomFactor(value - zoomStep) ?? defaultZoomFactor;
    case "reset":
      return defaultZoomFactor;
  }
}

export function zoomCommandForInput(input: ZoomShortcutInput): ZoomCommand | null {
  if (input.type !== "keyDown") return null;
  if (!input.control && !input.meta) return null;
  if (input.alt) return null;

  const key = input.key.toLowerCase();
  if (key === "+" || key === "=" || key === "add" || input.code === "Equal" || input.code === "NumpadAdd") {
    return "in";
  }
  if (key === "-" || key === "_" || key === "subtract" || input.code === "Minus" || input.code === "NumpadSubtract") {
    return "out";
  }
  if (key === "0" || input.code === "Digit0" || input.code === "Numpad0") {
    return "reset";
  }
  return null;
}

function roundZoom(value: number): number {
  return Math.round(value * 100) / 100;
}
