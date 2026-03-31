import { useState, useRef, useEffect, type KeyboardEvent } from "react";
import { useParams, useLocation, useNavigate } from "react-router-dom";
import { useConversation } from "@/hooks/use-conversation";
import { Button } from "@/components/ui/button";

export function ConversationPage() {
  const { id } = useParams<{ id: string }>();
  const location = useLocation();
  const navigate = useNavigate();
  const initialMessage = (location.state as { initialMessage?: string } | null)?.initialMessage;
  const sentInitial = useRef(false);

  const convId = id === "new" ? undefined : id;

  const {
    messages,
    streamingText,
    isStreaming,
    agentState,
    error,
    connectionStatus,
    conversationId,
    sendMessage,
    cancel,
  } = useConversation(convId);

  const [input, setInput] = useState("");
  const messagesEndRef = useRef<HTMLDivElement>(null);

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
            <MessageBubble key={i} role={msg.role} content={msg.content} />
          ))}

          {/* Streaming assistant response */}
          {streamingText && (
            <MessageBubble role="assistant" content={streamingText} streaming />
          )}

          {/* Agent status while streaming with no text yet */}
          {isStreaming && !streamingText && agentState && (
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

// ── Message bubble ──────────────────────────────────────────────────

function MessageBubble({
  role,
  content,
  streaming = false,
}: {
  role: "user" | "assistant";
  content: string;
  streaming?: boolean;
}) {
  const isUser = role === "user";

  return (
    <div className={`flex ${isUser ? "justify-end" : "justify-start"}`}>
      <div
        className={`max-w-[85%] whitespace-pre-wrap rounded-lg px-4 py-2.5 text-sm ${
          isUser
            ? "bg-primary text-primary-foreground"
            : "bg-muted text-foreground"
        }`}
      >
        {content}
        {streaming && (
          <span className="ml-0.5 inline-block h-4 w-0.5 animate-pulse bg-current" />
        )}
      </div>
    </div>
  );
}
