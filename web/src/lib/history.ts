/**
 * Converts REST API MessageView[] into ChatMessage[] with content blocks.
 * Groups adjacent assistant messages and tool messages into single ChatMessage
 * entries with structured blocks.
 */
import type { MessageView } from "@/types/api";
import type { ChatMessage, ContentBlock } from "@/hooks/use-conversation";

export function messageViewsToChat(views: MessageView[]): ChatMessage[] {
  const result: ChatMessage[] = [];
  let currentAssistant: ChatMessage | null = null;

  for (const view of views) {
    switch (view.role) {
      case "user":
        // Flush any in-progress assistant message.
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
        // Start or continue an assistant message.
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

        // If this assistant message has a tool_name, it's a tool_use block.
        if (view.tool_name) {
          const block: ContentBlock = {
            kind: "tool_call",
            toolCallId: view.tool_use_id ?? `tc-${view.id}`,
            toolName: view.tool_name,
            args: undefined,
            output: "",
            result: undefined,
            done: true,
            success: true,
          };
          // Try to parse content as JSON args.
          if (content) {
            try {
              block.args = JSON.parse(content) as Record<string, unknown>;
            } catch {
              // Content is the raw text, not JSON args.
              block.args = undefined;
            }
          }
          currentAssistant.blocks.push(block);
        } else if (content) {
          // Regular text content.
          currentAssistant.blocks.push({ kind: "text", text: content });
          currentAssistant.content += content;
        }
        break;
      }

      case "tool": {
        // Tool result — attach to the matching tool_call block.
        if (currentAssistant) {
          const toolCallId = view.tool_use_id;
          if (toolCallId) {
            // Find matching tool_call block.
            for (let i = currentAssistant.blocks.length - 1; i >= 0; i--) {
              const b = currentAssistant.blocks[i];
              if (b.kind === "tool_call" && b.toolCallId === toolCallId) {
                b.result = view.content ?? "";
                b.output = view.content ?? "";
                break;
              }
            }
          }
        }
        break;
      }

      case "system": {
        // Flush any in-progress assistant message.
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

  // Flush final assistant message.
  if (currentAssistant) {
    result.push(currentAssistant);
  }

  return result;
}
