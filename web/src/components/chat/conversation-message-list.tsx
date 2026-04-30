import type { CSSProperties, RefObject } from "react";
import { MessageBubble } from "@/components/chat/message-bubble";
import { TurnUsageBadge } from "@/components/chat/turn-usage-badge";
import type { ChatMessage, TurnUsage } from "@/hooks/use-conversation";

const MAX_RENDERED_MESSAGES = 200;

function agentStateLabel(state: string): string {
  switch (state) {
    case "assembling_context":
      return "Assembling context…";
    case "waiting_for_llm":
      return "Waiting for model…";
    case "executing_tools":
      return "Running tools…";
    case "compressing":
      return "Compressing history…";
    case "idle":
    default:
      return "Processing…";
  }
}

export function ConversationMessageList({
  messages,
  streamingText,
  isStreaming,
  agentState,
  error,
  usage,
  messagesEndRef,
}: {
  messages: ChatMessage[];
  streamingText: string;
  isStreaming: boolean;
  agentState: string | null;
  error: string | null;
  usage: TurnUsage | null;
  messagesEndRef: RefObject<HTMLDivElement | null>;
}) {
  const renderedMessages =
    messages.length > MAX_RENDERED_MESSAGES
      ? messages.slice(messages.length - MAX_RENDERED_MESSAGES)
      : messages;
  const renderedOffset = messages.length - renderedMessages.length;
  const lastAssistantIdx = (() => {
    for (let i = renderedMessages.length - 1; i >= 0; i--) {
      if (renderedMessages[i].role === "assistant") return i;
    }
    return -1;
  })();

  return (
    <div className="flex-1 overflow-y-auto px-4 py-6">
      <div className="mx-auto max-w-3xl space-y-4">
        {messages.length === 0 && !isStreaming && (
          <p className="py-12 text-center text-muted-foreground">
            Send a message to start
          </p>
        )}

        {renderedMessages.map((msg, i) => {
          const messageIndex = renderedOffset + i;
          return (
            <div key={messageIndex}>
              <MessageBubble
                message={msg}
                streaming={isStreaming && messageIndex === messages.length - 1 && msg.role === "assistant"}
              />
              {i === lastAssistantIdx && !isStreaming && usage && (
                <div className="flex justify-start mt-0.5">
                  <div className="max-w-[85%]">
                    <TurnUsageBadge usage={usage} />
                  </div>
                </div>
              )}
            </div>
          );
        })}

        {isStreaming &&
          !streamingText &&
          agentState &&
          (messages.length === 0 ||
            messages[messages.length - 1].role !== "assistant" ||
            messages[messages.length - 1].blocks.length === 0) && (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <span className="inline-block h-2 w-2 bg-primary pulse-glow" />
              {agentStateLabel(agentState)}
            </div>
          )}

        {error && (
          <div
            data-augmented-ui="tl-clip border"
            className="border-0 bg-destructive/10 px-4 py-3 text-sm text-destructive"
            style={{
              "--aug-tl": "8px",
              "--aug-border-all": "1px",
              "--aug-border-bg": "#ff1744",
            } as CSSProperties}
          >
            {error}
          </div>
        )}

        <div ref={messagesEndRef} />
      </div>
    </div>
  );
}
