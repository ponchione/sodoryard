import type { ReceiptSummary } from "@/types/chains";

export interface ParsedReceiptDocument {
  body: string;
  frontmatter: Record<string, string>;
  rawFrontmatter: string;
}

export interface ReceiptSection {
  title: string;
  content: string;
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

export function receiptFollowUpSections(parsed: ParsedReceiptDocument): ReceiptSection[] {
  const sections = receiptHeadingSections(parsed.body);
  const wanted = [
    { title: "Concerns", keys: ["concerns"] },
    { title: "Next Steps", keys: ["next steps", "next step", "follow ups", "follow-up actions", "follow up actions"] },
  ];

  const out: ReceiptSection[] = [];
  for (const section of wanted) {
    for (const key of section.keys) {
      const content = sections.get(key);
      if (content) {
        out.push({ title: section.title, content });
        break;
      }
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

function receiptHeadingSections(body: string): Map<string, string> {
  const sections = new Map<string, string>();
  let currentTitle = "";
  let currentLines: string[] = [];

  const flush = () => {
    if (!currentTitle) return;
    const content = currentLines.join("\n").trim();
    if (content) sections.set(normalizeSectionTitle(currentTitle), content);
  };

  for (const line of body.replace(/\r\n/g, "\n").split("\n")) {
    const heading = /^##(?!#)\s+(.+?)\s*#*\s*$/.exec(line);
    if (heading) {
      flush();
      currentTitle = heading[1];
      currentLines = [];
      continue;
    }
    if (currentTitle) currentLines.push(line);
  }

  flush();
  return sections;
}

function normalizeSectionTitle(title: string): string {
  return title
    .replace(/[`*_#]/g, "")
    .toLowerCase()
    .replace(/\s+/g, " ")
    .trim();
}
