import type { ReceiptSummary } from "@/types/chains";

export interface ParsedReceiptDocument {
  body: string;
  frontmatter: Record<string, string>;
  rawFrontmatter: string;
}

export function receiptRoute(chainID: string, step?: string): string {
  const base = `/receipts/${encodeURIComponent(chainID)}`;
  const trimmedStep = step?.trim() ?? "";
  return trimmedStep ? `${base}/${encodeURIComponent(trimmedStep)}` : base;
}

export function receiptRouteForSummary(chainID: string, receipt: ReceiptSummary): string {
  return receiptRoute(chainID, receipt.step);
}

export function receiptProjectPaths(parsed: ParsedReceiptDocument): string[] {
  const keys = ["changed_files", "files_changed"];
  const seen = new Set<string>();
  const out: string[] = [];
  for (const key of keys) {
    for (const path of splitReceiptPathList(parsed.frontmatter[key] ?? "")) {
      if (seen.has(path)) continue;
      seen.add(path);
      out.push(path);
    }
  }
  return out;
}

export function parseReceiptDocument(content: string): ParsedReceiptDocument {
  const normalized = content.replace(/\r\n/g, "\n");
  if (!normalized.startsWith("---\n")) {
    return { body: content, frontmatter: {}, rawFrontmatter: "" };
  }

  const end = normalized.indexOf("\n---", 4);
  if (end === -1) {
    return { body: content, frontmatter: {}, rawFrontmatter: "" };
  }

  const rawFrontmatter = normalized.slice(4, end);
  const bodyStart = normalized.startsWith("\n", end + 4) ? end + 5 : end + 4;
  return {
    body: normalized.slice(bodyStart),
    frontmatter: parseSimpleFrontmatter(rawFrontmatter),
    rawFrontmatter,
  };
}

function parseSimpleFrontmatter(raw: string): Record<string, string> {
  const values: Record<string, string[]> = {};
  let currentKey = "";

  for (const line of raw.split("\n")) {
    const keyValue = /^([A-Za-z0-9_-]+):\s*(.*)$/.exec(line);
    if (keyValue) {
      currentKey = keyValue[1];
      const value = cleanFrontmatterValue(keyValue[2]);
      values[currentKey] = value ? [value] : [];
      continue;
    }

    const listItem = /^\s*-\s+(.+)$/.exec(line);
    if (currentKey && listItem) {
      values[currentKey].push(cleanFrontmatterValue(listItem[1]));
    }
  }

  return Object.fromEntries(
    Object.entries(values).map(([key, parts]) => [key, parts.filter(Boolean).join(", ") || "<empty>"]),
  );
}

function cleanFrontmatterValue(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) return "";
  if (trimmed.startsWith("[") && trimmed.endsWith("]")) {
    return trimmed
      .slice(1, -1)
      .split(",")
      .map((part) => stripQuotes(part.trim()))
      .filter(Boolean)
      .join(", ");
  }
  return stripQuotes(trimmed);
}

function stripQuotes(value: string): string {
  return value.replace(/^["']|["']$/g, "");
}

function splitReceiptPathList(value: string): string[] {
  return value
    .split(",")
    .map((part) => stripQuotes(part.trim()))
    .filter((part) => part !== "" && part !== "<empty>");
}
