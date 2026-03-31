import { useState, useRef, useEffect, type KeyboardEvent } from "react";
import { useParams, useLocation, useNavigate } from "react-router-dom";
import {
  useConversation,
  type ChatMessage,
  type ContentBlock,
} from "@/hooks/use-conversation";
import { api } from "@/lib/api";
import { messageViewsToChat } from "@/lib/history";
import type { MessageView } from "@/types/api";
import { Button } from "@/components/ui/button";
import { ThinkingBlock } from "@/components/chat/thinking-block";
import { ToolCallCard } from "@/components/chat/tool-call-card";
import { TurnUsageBadge } from "@/components/chat/turn-usage-badge";
import { MarkdownContent } from "@/components/chat/markdown-content";

export function ConversationPage() {
  const { id } = useParams<{ id: string }>();
  const location = useLocation();
  const navigate = useNavigate();
  const initialMessage = (location.state as { initialMessage?: string } | null)?.initialMessage;
  const sentInitial = useRef(false);
  const historyLoaded = useRef(false);

  const convId = id === "new" ? undefined : id;

  const {
    messages,
    streamingText,
    isStreaming,
    agentState,
    error,
    connectionStatus,
    conversationId,
    lastTurnUsage,
    sendMessage,
    cancel,
    loadHistory,
  } = useConversation(convId);

  const [input, setInput] = useState("");
  const messagesEndRef = useRef<HTMLDivElement>(null);

  // Load existing conversation history on mount.
  useEffect(() => {
    if (convId && !historyLoaded.current) {
      historyLoaded.current = true;
      api
        .get<MessageView[]>(`/api/conversations/${convId}/messages`)
        .then((views) => {
          const chatMessages = messageViewsToChat(views);
          loadHistory(chatMessages);
        })
        .catch((err) => {
          console.error("Failed to load conversation history:", err);
        });
    }
  }, [convId, loadHistory]);

  // Send initial message once when navigating from home with text.
  useEffect(() => {
    if (initialMessage && !sentInitial.current && connectionStatus === "connected") {
      sentInitial.current = true;
      sendMessage(initialMessage);
    }
  }, [initialMessage, connectionStatus, sendMessage]);

  // When backend creates a conversation, update the URL without re-mounting.
  useEffect(() => {
    if (conversationId && id === "new") {
      navigate(`/c/${conversationId}`, { replace: true });
    }
  }, [conversationId, id, navigate]);

  // Auto-scroll to bottom on new content.
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, streamingText]);

  const handleSend = () => {
    const text = input.trim();
    if (!text || isStreaming) return;
    setInput("");
    sendMessage(text);
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  // Find the last assistant message index to attach usage badge.
  const lastAssistantIdx = (() => {
    for (let i = messages.length - 1; i >= 0; i--) {
      if (messages[i].role === "assistant") return i;
    }
    return -1;
  })();

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      {/* Connection indicator */}
      {connectionStatus !== "connected" && (
        <div className="border-b border-border bg-muted px-4 py-1.5 text-center text-xs text-muted-foreground">
          {connectionStatus === "connecting" ? "Connecting…" : "Disconnected — reconnecting…"}
        </div>
      )}

      {/* Message area */}
      <div className="flex-1 overflow-y-auto px-4 py-6">
        <div className="mx-auto max-w-3xl space-y-4">
          {messages.length === 0 && !isStreaming && (
            <p className="py-12 text-center text-muted-foreground">
              Send a message to start
            </p>
          )}

          {messages.map((msg, i) => (
            <div key={i}>
              <MessageBubble
                message={msg}
                streaming={isStreaming && i === messages.length - 1 && msg.role === "assistant"}
              />
              {/* Usage badge after last assistant message when turn is done */}
              {i === lastAssistantIdx && !isStreaming && lastTurnUsage && (
                <div className="flex justify-start mt-0.5">
                  <div className="max-w-[85%]">
                    <TurnUsageBadge usage={lastTurnUsage} />
                  </div>
                </div>
              )}
            </div>
          ))}

          {/* Agent status while streaming with no content yet */}
          {isStreaming &&
            !streamingText &&
            agentState &&
            (messages.length === 0 ||
              messages[messages.length - 1].role !== "assistant" ||
              messages[messages.length - 1].blocks.length === 0) && (
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <span className="inline-block h-2 w-2 animate-pulse rounded-full bg-primary" />
                {agentState === "thinking" && "Thinking…"}
                {agentState === "executing_tools" && "Running tools…"}
                {agentState === "idle" && "Processing…"}
              </div>
            )}

          {/* Error banner */}
          {error && (
            <div className="rounded-lg border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
              {error}
            </div>
          )}

          <div ref={messagesEndRef} />
        </div>
      </div>

      {/* Input area */}
      <div className="border-t border-border p-4">
        <div className="mx-auto flex max-w-3xl gap-2">
          <textarea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Type a message… (Enter to send, Shift+Enter for newline)"
            className="flex-1 resize-none rounded-lg border border-input bg-background px-3 py-2 text-sm outline-none ring-ring/50 placeholder:text-muted-foreground focus-visible:ring-2"
            rows={1}
            disabled={isStreaming}
            autoFocus
          />
          {isStreaming ? (
            <Button variant="destructive" onClick={cancel}>
              Cancel
            </Button>
          ) : (
            <Button onClick={handleSend} disabled={!input.trim()}>
              Send
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}

// ── Block renderer ───────────────────────────────────────────────────

function BlockRenderer({ block, streaming }: { block: ContentBlock; streaming: boolean }) {
  switch (block.kind) {
    case "thinking":
      return <ThinkingBlock block={block} />;
    case "tool_call":
      return <ToolCallCard block={block} />;
    case "text":
      return (
        <div>
          <MarkdownContent content={block.text} />
          {streaming && (
            <span className="ml-0.5 inline-block h-4 w-0.5 animate-pulse bg-current" />
          )}
        </div>
      );
  }
}

// ── Message bubble ───────────────────────────────────────────────────

function MessageBubble({
  message,
  streaming = false,
}: {
  message: ChatMessage;
  streaming?: boolean;
}) {
  const isUser = message.role === "user";
  const isSystem = message.role === "system";
  const isCompressed = message.isCompressed || message.isSummary;

  // System messages.
  if (isSystem) {
    return (
      <div className="flex justify-center">
        <div className="max-w-[85%] rounded-lg border border-dashed border-border bg-muted/30 px-4 py-2 text-xs text-muted-foreground italic">
          {isCompressed && (
            <span className="mr-1.5 inline-block rounded bg-muted-foreground/20 px-1 py-0.5 text-[10px] font-medium not-italic">
              compressed
            </span>
          )}
          {message.content}
        </div>
      </div>
    );
  }

  // User messages or messages with no blocks — simple text.
  if (isUser || message.blocks.length === 0) {
    return (
      <div className={`flex ${isUser ? "justify-end" : "justify-start"}`}>
        <div
          className={`max-w-[85%] whitespace-pre-wrap rounded-lg px-4 py-2.5 text-sm ${
            isUser
              ? "bg-primary text-primary-foreground"
              : isCompressed
                ? "bg-muted/50 text-muted-foreground italic border border-dashed border-border"
                : "bg-muted text-foreground"
          }`}
        >
          {isCompressed && (
            <span className="mr-1.5 inline-block rounded bg-muted-foreground/20 px-1 py-0.5 text-[10px] font-medium not-italic">
              compressed
            </span>
          )}
          {message.content}
        </div>
      </div>
    );
  }

  // Assistant messages with blocks — render each block with markdown.
  return (
    <div className="flex justify-start">
      <div
        className={`max-w-[85%] rounded-lg px-4 py-2.5 text-sm ${
          isCompressed
            ? "bg-muted/50 text-muted-foreground border border-dashed border-border"
            : "bg-muted text-foreground"
        }`}
      >
        {isCompressed && (
          <span className="mb-1.5 inline-block rounded bg-muted-foreground/20 px-1 py-0.5 text-[10px] font-medium">
            compressed
          </span>
        )}
        {message.blocks.map((block, i) => {
          const isLastBlock = i === message.blocks.length - 1;
          return (
            <BlockRenderer
              key={i}
              block={block}
              streaming={streaming && isLastBlock}
            />
          );
        })}
      </div>
    </div>
  );
}
