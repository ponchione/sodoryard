/**
 * Converts REST API MessageView[] into ChatMessage[] with content blocks.
 * Groups adjacent assistant messages and tool messages into single ChatMessage
 * entries with structured blocks.
 */
import type { MessageView } from "@/types/api";
import type { ChatMessage, ContentBlock, ToolCallBlock } from "@/hooks/use-conversation";

interface StoredAssistantBlock {
  type?: string;
  text?: string;
  thinking?: string;
  id?: string;
  name?: string;
  input?: unknown;
}

interface ToolResultPresentation {
  text: string;
  success?: boolean;
}

export function messageViewsToChat(views: MessageView[]): ChatMessage[] {
  const result: ChatMessage[] = [];
  let currentAssistant: ChatMessage | null = null;

  for (const view of views) {
    switch (view.role) {
      case "user":
        if (currentAssistant) {
          result.push(currentAssistant);
          currentAssistant = null;
        }
        result.push({
          role: "user",
          content: view.content ?? "",
          blocks: [],
          isCompressed: view.is_compressed,
          isSummary: view.is_summary,
        });
        break;

      case "assistant": {
        if (!currentAssistant) {
          currentAssistant = {
            role: "assistant",
            content: "",
            blocks: [],
            isCompressed: view.is_compressed,
            isSummary: view.is_summary,
          };
        }

        const content = view.content ?? "";
        if (view.tool_name) {
          currentAssistant.blocks.push(buildToolCallBlock(view.tool_use_id ?? `tc-${view.id}`, view.tool_name, content));
          break;
        }

        const { blocks, text } = parseAssistantContent(content);
        currentAssistant.blocks.push(...blocks);
        currentAssistant.content += text;
        break;
      }

      case "tool": {
        if (currentAssistant) {
          const toolCallId = view.tool_use_id;
          if (toolCallId) {
            const presentation = presentToolResult(view.content ?? "");
            for (let i = currentAssistant.blocks.length - 1; i >= 0; i--) {
              const b = currentAssistant.blocks[i];
              if (b.kind === "tool_call" && b.toolCallId === toolCallId) {
                b.result = presentation.text;
                b.output = presentation.text;
                if (presentation.success !== undefined) {
                  b.success = presentation.success;
                }
                break;
              }
            }
          }
        }
        break;
      }

      case "system": {
        if (currentAssistant) {
          result.push(currentAssistant);
          currentAssistant = null;
        }
        result.push({
          role: "system",
          content: view.content ?? "",
          blocks: [],
          isCompressed: view.is_compressed,
          isSummary: view.is_summary,
        });
        break;
      }
    }
  }

  if (currentAssistant) {
    result.push(currentAssistant);
  }

  return result;
}

function buildToolCallBlock(toolCallId: string, toolName: string, rawArgs: string): ToolCallBlock {
  const block: ToolCallBlock = {
    kind: "tool_call",
    toolCallId,
    toolName,
    args: undefined,
    output: "",
    result: undefined,
    done: true,
    success: true,
  };
  if (rawArgs) {
    try {
      block.args = JSON.parse(rawArgs) as Record<string, unknown>;
    } catch {
      block.args = undefined;
    }
  }
  return block;
}

function parseAssistantContent(content: string): { blocks: ContentBlock[]; text: string } {
  if (!content) {
    return { blocks: [], text: "" };
  }

  try {
    const parsed = JSON.parse(content) as unknown;
    if (typeof parsed === "string") {
      const text = presentAssistantText(parsed);
      return { blocks: [{ kind: "text", text }], text };
    }
    if (Array.isArray(parsed)) {
      const blocks: ContentBlock[] = [];
      let text = "";
      for (const rawBlock of parsed) {
        const block = rawBlock as StoredAssistantBlock;
        switch (block.type) {
          case "text": {
            const value = presentAssistantText(block.text ?? "");
            if (!value) {
              continue;
            }
            blocks.push({ kind: "text", text: value });
            text += value;
            break;
          }
          case "thinking":
            if (block.thinking) {
              blocks.push({ kind: "thinking", text: block.thinking, done: true });
            }
            break;
          case "tool_use":
            if (block.id && block.name) {
              blocks.push({
                kind: "tool_call",
                toolCallId: block.id,
                toolName: block.name,
                args: isRecord(block.input) ? block.input : undefined,
                output: "",
                done: true,
                success: true,
              });
            }
            break;
          default:
            break;
        }
      }
      if (blocks.length > 0) {
        return { blocks, text };
      }
    }
  } catch {
    // Fall back to raw text below.
  }

  const text = presentAssistantText(content);
  return text ? { blocks: [{ kind: "text", text }], text } : { blocks: [], text: "" };
}

function presentAssistantText(text: string): string {
  const failed = parseAssistantTombstone(text, "[failed_assistant]", "Assistant output ended due to a stream failure before turn completion.");
  if (failed) {
    return failed;
  }
  const interrupted = parseAssistantTombstone(text, "[interrupted_assistant]", "Assistant output was interrupted before turn completion.");
  if (interrupted) {
    return interrupted;
  }
  return text;
}

function parseAssistantTombstone(text: string, marker: string, fallbackMessage: string): string | null {
  if (!text.startsWith(marker)) {
    return null;
  }
  const fields = parseKeyValueLines(text);
  const summary = fields.message || fallbackMessage;
  const partial = fields.partial_text?.trim();
  if (!partial) {
    return `Status: ${summary}`;
  }
  return `${summary}\n\nPartial output before termination:\n\n${partial}`;
}

function presentToolResult(text: string): ToolResultPresentation {
  if (!text.startsWith("[interrupted_tool_result]")) {
    return { text };
  }
  const fields = parseKeyValueLines(text);
  const reason = humanizeCleanupReason(fields.reason);
  const status = humanizeToolStatus(fields.status);
  const toolName = fields.tool || "tool";
  return {
    text: `Interrupted ${toolName} result. ${status}${reason ? ` (${reason})` : ""}.`,
    success: false,
  };
}

function parseKeyValueLines(text: string): Record<string, string> {
  const fields: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const idx = line.indexOf("=");
    if (idx <= 0) {
      continue;
    }
    fields[line.slice(0, idx)] = line.slice(idx + 1);
  }
  return fields;
}

function humanizeCleanupReason(reason?: string): string {
  switch (reason) {
    case "interrupt":
      return "user interrupted";
    case "cancel":
      return "turn cancelled";
    case "context_deadline_exceeded":
      return "deadline exceeded";
    case "stream_failure":
      return "stream failure";
    default:
      return reason ?? "";
  }
}

function humanizeToolStatus(status?: string): string {
  switch (status) {
    case "interrupted_during_execution":
      return "Execution stopped before completion";
    case "cancelled_before_execution":
      return "Execution never started";
    default:
      return "Execution did not complete";
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
