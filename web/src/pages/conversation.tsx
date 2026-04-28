import { useState, useRef, useEffect, useMemo, type KeyboardEvent } from "react";
import { useParams, useLocation, useNavigate } from "react-router-dom";
import {
  useConversation,
  type ChatMessage,
  type ContentBlock,
  type TurnUsage,
} from "@/hooks/use-conversation";
import { useContextReport } from "@/hooks/use-context-report";
import { useProviders } from "@/hooks/use-providers";
import { api } from "@/lib/api";
import { messageViewsToChat } from "@/lib/history";
import type { Conversation, MessageView } from "@/types/api";
import type { AppConfig, ContextReport, ConversationMetrics } from "@/types/metrics";
import { Button } from "@/components/ui/button";
import { ThinkingBlock } from "@/components/chat/thinking-block";
import { ToolCallCard } from "@/components/chat/tool-call-card";
import { TurnUsageBadge } from "@/components/chat/turn-usage-badge";
import { MarkdownContent } from "@/components/chat/markdown-content";
import { ContextInspector } from "@/components/inspector/context-inspector";
import { ConversationMetricsPanel } from "@/components/chat/conversation-metrics";
import { getDisplayBlocks } from "@/lib/tool-transcript";

const conversationPageSessionState = {
  inspectorOpen: false,
};

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
    lastContextDebug,
    sendMessage,
    setModelOverride,
    cancel,
    loadHistory,
  } = useConversation(convId);

  const [input, setInput] = useState("");
  const [inspectorOpen, setInspectorOpen] = useState(() => conversationPageSessionState.inspectorOpen);
  const [metricsOpen, setMetricsOpen] = useState(false);
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [conversationMeta, setConversationMeta] = useState<Conversation | null>(null);
  const [selectedProvider, setSelectedProvider] = useState<string>("");
  const [selectedModel, setSelectedModel] = useState<string>("");
  // B3 fix: hydrate the last-turn usage badge on page reload. The WS-driven
  // lastTurnUsage only populates on live turn_complete events, so a page
  // refresh leaves the badge blank. We fetch the aggregate metrics once per
  // convId and use its last_turn field as a fallback.
  const [hydratedLastTurn, setHydratedLastTurn] = useState<TurnUsage | null>(null);
  const [historyLatestTurnPending, setHistoryLatestTurnPending] = useState(false);
  const ctxReport = useContextReport(
    convId,
    historyLatestTurnPending || isStreaming || agentState !== "idle",
    inspectorOpen,
  );
  const { providers } = useProviders();

  // Derived: prefer the live state value over hydrated, so ongoing turns
  // override stale page-load data.
  const displayLastTurnUsage: TurnUsage | null = lastTurnUsage ?? hydratedLastTurn;

  // Feed live context_debug events into the inspector.
  useEffect(() => {
    if (lastContextDebug) {
      setHistoryLatestTurnPending(false);
      ctxReport.setLiveReport(lastContextDebug as unknown as ContextReport);
    }
  }, [lastContextDebug]); // eslint-disable-line react-hooks/exhaustive-deps
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    historyLoaded.current = false;
    setHistoryLatestTurnPending(false);
  }, [convId]);

  // B3 fix: fetch aggregate metrics once per conversation, extract last_turn,
  // and hydrate the usage badge so it's populated on reload even before a new
  // live turn_complete event fires.
  useEffect(() => {
    if (!convId) {
      setHydratedLastTurn(null);
      return;
    }
    let cancelled = false;
    api
      .get<ConversationMetrics>(`/api/metrics/conversation/${convId}`)
      .then((data) => {
        if (cancelled) return;
        if (data.last_turn) {
          setHydratedLastTurn({
            turnNumber: data.last_turn.turn_number,
            iterationCount: data.last_turn.iteration_count,
            inputTokens: data.last_turn.tokens_in,
            outputTokens: data.last_turn.tokens_out,
            duration: data.last_turn.latency_ms * 1_000_000, // ms -> ns
          });
        } else {
          setHydratedLastTurn(null);
        }
      })
      .catch((err) => {
        console.error("Failed to hydrate last-turn usage:", err);
      });
    return () => {
      cancelled = true;
    };
  }, [convId]);

  useEffect(() => {
    conversationPageSessionState.inspectorOpen = inspectorOpen;
  }, [inspectorOpen]);

  useEffect(() => {
    let cancelled = false;
    api
      .get<AppConfig>("/api/config")
      .then((data) => {
        if (!cancelled) {
          setConfig(data);
        }
      })
      .catch((err) => {
        console.error("Failed to load app config:", err);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    if (!convId) {
      setConversationMeta(null);
      return () => {
        cancelled = true;
      };
    }
    api
      .get<Conversation>(`/api/conversations/${convId}`)
      .then((data) => {
        if (!cancelled) {
          setConversationMeta(data);
        }
      })
      .catch((err) => {
        console.error("Failed to load conversation metadata:", err);
      });
    return () => {
      cancelled = true;
    };
  }, [convId]);

  // Load existing conversation history on mount.
  useEffect(() => {
    if (convId && !historyLoaded.current) {
      historyLoaded.current = true;
      api
        .get<MessageView[]>(`/api/conversations/${convId}/messages`)
        .then((views) => {
          const chatMessages = messageViewsToChat(views);
          loadHistory(chatMessages);
          const maxTurn = views.reduce((max, view) => Math.max(max, view.turn_number ?? 0), 0);
          const latestTurnHasAssistant = views.some(
            (view) => view.turn_number === maxTurn && view.role === "assistant",
          );
          setHistoryLatestTurnPending(maxTurn > 0 && !latestTurnHasAssistant);
          ctxReport.setHistoryTurns(maxTurn);
        })
        .catch((err) => {
          console.error("Failed to load conversation history:", err);
        });
    }
  }, [convId, loadHistory, ctxReport]);

  const runtimeProvider = config?.default_provider ?? "";
  const runtimeModel = config?.default_model ?? "";

  const selectableProviders = useMemo(() => {
    const dedupeModels = <T extends { id: string }>(models: T[]): T[] => (
      models.filter((model, index, all) => all.findIndex((candidate) => candidate.id === model.id) === index)
    );

    if (runtimeProvider && runtimeModel) {
      const provider = providers.find((item) => item.name === runtimeProvider);
      const model = provider?.models.find((item) => item.id === runtimeModel) ?? {
        id: runtimeModel,
        name: runtimeModel,
        context_window: 0,
        supports_tools: false,
        supports_thinking: false,
      };
      if (provider) {
        return [{ ...provider, models: [model] }];
      }
    }

    return providers
      .map((provider) => {
        const models = dedupeModels(provider.models);
        if (models.length > 0) {
          return { ...provider, models };
        }
        if (provider.name === selectedProvider && selectedModel) {
          return {
            ...provider,
            models: [{
              id: selectedModel,
              name: selectedModel,
              context_window: 0,
              supports_tools: false,
              supports_thinking: false,
            }],
          };
        }
        return null;
      })
      .filter((provider): provider is NonNullable<typeof provider> => provider !== null);
  }, [providers, runtimeModel, runtimeProvider, selectedModel, selectedProvider]);

  const selectedProviderModels = useMemo(
    () => selectableProviders.find((provider) => provider.name === selectedProvider)?.models ?? [],
    [selectableProviders, selectedProvider],
  );

  useEffect(() => {
    const fallbackProvider = runtimeProvider;
    const fallbackModel = runtimeModel;
    const nextProvider = fallbackProvider || conversationMeta?.provider || "";
    const nextModel = fallbackModel || conversationMeta?.model || "";

    if (!nextProvider || !nextModel) {
      return;
    }

    setSelectedProvider(nextProvider);
    setSelectedModel(nextModel);
  }, [conversationMeta?.model, conversationMeta?.provider, runtimeModel, runtimeProvider]);

  useEffect(() => {
    if (!selectedProvider) {
      return;
    }
    const provider = providers.find((item) => item.name === selectedProvider);
    if (!provider) {
      return;
    }
    if (provider.models.some((model) => model.id === selectedModel)) {
      return;
    }
    if (provider.models.length > 0) {
      setSelectedModel(provider.models[0].id);
      return;
    }
    if (config?.default_provider === selectedProvider && config.default_model) {
      setSelectedModel(config.default_model);
    }
  }, [config?.default_model, config?.default_provider, providers, selectedModel, selectedProvider]);

  const isConversationOverrideActive = !!(
    config &&
    selectedProvider &&
    selectedModel &&
    (selectedProvider !== config.default_provider || selectedModel !== config.default_model)
  );

  const messageOverride = useMemo(
    () => isConversationOverrideActive
      ? { provider: selectedProvider, model: selectedModel }
      : undefined,
    [isConversationOverrideActive, selectedModel, selectedProvider],
  );

  const handleModelOverrideChange = (provider: string, model: string) => {
    const nextProvider = runtimeProvider || provider;
    const nextModel = runtimeModel || model;
    setSelectedProvider(nextProvider);
    setSelectedModel(nextModel);
    if (!runtimeProvider || !runtimeModel) {
      setModelOverride(nextProvider, nextModel);
    }
  };

  // Send initial message once when navigating from home with text.
  useEffect(() => {
    if (initialMessage && !sentInitial.current && connectionStatus === "connected") {
      sentInitial.current = true;
      sendMessage(initialMessage, messageOverride);
    }
  }, [initialMessage, connectionStatus, messageOverride, sendMessage]);

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
    sendMessage(text, messageOverride);
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
    <div className="flex flex-1 overflow-hidden">
      {/* Main chat column */}
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Top bar with toggle buttons */}
        <div className="flex items-center justify-between border-b border-border px-4 py-1.5 gap-3">
          <div className="flex items-center gap-3 min-w-0">
            <div className="text-xs text-muted-foreground shrink-0">
              {connectionStatus !== "connected"
                ? connectionStatus === "connecting" ? "Connecting…" : "Disconnected — reconnecting…"
                : conversationId ? `${conversationId.slice(0, 8)}…` : "New conversation"}
            </div>
            {config && selectedProvider && (
              <div className="flex items-center gap-2 min-w-0">
                <select
                  value={selectedProvider}
                  onChange={(e) => {
                    const provider = e.target.value;
                    const providerModels = selectableProviders.find((item) => item.name === provider)?.models ?? [];
                    const nextModel = providerModels[0]?.id ?? selectedModel;
                    handleModelOverrideChange(provider, nextModel);
                  }}
                  className="h-7 rounded border border-border bg-input px-2 text-xs text-foreground"
                  aria-label="Conversation provider"
                  disabled={selectableProviders.length <= 1}
                >
                  {selectableProviders.map((provider) => (
                    <option key={provider.name} value={provider.name}>
                      {provider.name}
                    </option>
                  ))}
                </select>
                <select
                  value={selectedModel}
                  onChange={(e) => handleModelOverrideChange(selectedProvider, e.target.value)}
                  className="h-7 max-w-56 rounded border border-border bg-input px-2 text-xs text-foreground"
                  aria-label="Conversation model"
                  disabled={selectedProviderModels.length <= 1}
                >
                  {selectedProviderModels.map((model) => (
                    <option key={model.id} value={model.id}>
                      {model.id}
                    </option>
                  ))}
                </select>
                {isConversationOverrideActive && (
                  <span className="shrink-0 bg-primary/15 px-2 py-1 text-[10px] font-medium uppercase tracking-widest text-primary">
                    override
                  </span>
                )}
              </div>
            )}
          </div>
          <div className="flex items-center gap-1 shrink-0">
            <button
              type="button"
              onClick={() => setMetricsOpen(!metricsOpen)}
              className={`p-1 text-xs ${metricsOpen ? "bg-muted text-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`}
              title="Conversation metrics"
            >
              📊
            </button>
            <button
              type="button"
              onClick={() => setInspectorOpen(!inspectorOpen)}
              className={`p-1 text-xs ${inspectorOpen ? "bg-muted text-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`}
              title="Context inspector"
            >
              🔍
            </button>
          </div>
        </div>

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
              {i === lastAssistantIdx && !isStreaming && displayLastTurnUsage && (
                <div className="flex justify-start mt-0.5">
                  <div className="max-w-[85%]">
                    <TurnUsageBadge usage={displayLastTurnUsage} />
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
                <span className="inline-block h-2 w-2 bg-primary pulse-glow" />
                {agentStateLabel(agentState)}
              </div>
            )}

          {/* Error banner */}
          {error && (
            <div
              data-augmented-ui="tl-clip border"
              className="border-0 bg-destructive/10 px-4 py-3 text-sm text-destructive"
              style={{
                "--aug-tl": "8px",
                "--aug-border-all": "1px",
                "--aug-border-bg": "#ff1744",
              } as React.CSSProperties}
            >
              {error}
            </div>
          )}

          <div ref={messagesEndRef} />
        </div>
      </div>

      {/* Metrics panel (below messages, above input) */}
      {metricsOpen && convId && (
        <div className="border-t border-border px-4 py-2 max-h-60 overflow-y-auto">
          <div className="mx-auto max-w-3xl">
            <ConversationMetricsPanel
              conversationId={convId}
              refreshKey={lastTurnUsage?.turnNumber}
            />
          </div>
        </div>
      )}

      {/* Input area */}
      <div className="border-t border-border p-4">
        <div className="mx-auto flex max-w-3xl gap-2">
          <div
            data-augmented-ui="tl-clip br-clip border"
            className="flex flex-1"
            style={{
              "--aug-tl": "10px",
              "--aug-br": "10px",
              "--aug-border-all": "1px",
              "--aug-border-bg": "#00e5ff60",
            } as React.CSSProperties}
          >
            <textarea
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Type a message… (Enter to send, Shift+Enter for newline)"
              className="flex-1 resize-none border-0 bg-input px-3 py-2 text-sm outline-none placeholder:text-muted-foreground"
              rows={1}
              disabled={isStreaming}
              autoFocus
            />
          </div>
          {isStreaming ? (
            <Button
              variant="destructive"
              onClick={cancel}
              data-augmented-ui="tl-clip br-clip border"
              className="border-0 bg-destructive/20 text-destructive hover:bg-destructive/30"
              style={{
                "--aug-tl": "6px",
                "--aug-br": "6px",
                "--aug-border-all": "1px",
                "--aug-border-bg": "#ff1744",
              } as React.CSSProperties}
            >
              Cancel
            </Button>
          ) : (
            <Button
              onClick={handleSend}
              disabled={!input.trim()}
              data-augmented-ui="tl-clip br-clip border"
              className="border-0 bg-primary text-primary-foreground hover:bg-primary/80"
              style={{
                "--aug-tl": "6px",
                "--aug-br": "6px",
                "--aug-border-all": "1px",
                "--aug-border-bg": "#00e5ff",
              } as React.CSSProperties}
            >
              Send
            </Button>
          )}
        </div>
      </div>
      </div>{/* end main chat column */}

      {/* Context Inspector panel (right side) */}
      {inspectorOpen && (
        <ContextInspector ctx={ctxReport} onClose={() => setInspectorOpen(false)} />
      )}
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
            <span className="ml-0.5 inline-block h-4 w-1.5 bg-primary pulse-glow" />
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
  const displayBlocks = getDisplayBlocks(message.blocks);

  // System messages — amber dashed border.
  if (isSystem) {
    return (
      <div className="flex justify-center">
        <div className="max-w-[85%] border border-dashed border-[#ffab00]/40 bg-muted/30 px-4 py-2 text-xs text-muted-foreground italic">
          {isCompressed && (
            <span className="mr-1.5 inline-block bg-muted-foreground/20 px-1 py-0.5 text-[10px] font-medium not-italic">
              compressed
            </span>
          )}
          {message.content}
        </div>
      </div>
    );
  }

  // User messages — augmented with br-clip, cyan border.
  if (!isUser && displayBlocks.length === 0) {
    return null;
  }

  if (isUser) {
    return (
      <div className={`flex ${isUser ? "justify-end" : "justify-start"}`}>
        <div
          data-augmented-ui={isUser ? "br-clip border" : undefined}
          className={`max-w-[85%] whitespace-pre-wrap px-4 py-2.5 text-sm ${
            isUser
              ? "bg-primary/10 text-foreground"
              : isCompressed
                ? "bg-muted/50 text-muted-foreground italic border border-dashed border-[#ffab00]/40"
                : "bg-muted text-foreground"
          }`}
          style={
            isUser
              ? ({
                  "--aug-br": "12px",
                  "--aug-border-all": "1px",
                  "--aug-border-bg": "#00e5ff60",
                } as React.CSSProperties)
              : undefined
          }
        >
          {isCompressed && (
            <span className="mr-1.5 inline-block bg-muted-foreground/20 px-1 py-0.5 text-[10px] font-medium not-italic">
              compressed
            </span>
          )}
          {message.content}
        </div>
      </div>
    );
  }

  // Assistant messages with blocks — augmented with tl-clip, green border.
  return (
    <div className="flex justify-start">
      <div
        data-augmented-ui="tl-clip border"
        className={`max-w-[85%] px-4 py-2.5 text-sm ${
          isCompressed
            ? "bg-muted/50 text-muted-foreground border border-dashed border-[#ffab00]/40"
            : "bg-muted text-foreground"
        }`}
        style={{
          "--aug-tl": "12px",
          "--aug-border-all": "1px",
          "--aug-border-bg": "#00e67640",
        } as React.CSSProperties}
      >
        {isCompressed && (
          <span className="mb-1.5 inline-block bg-muted-foreground/20 px-1 py-0.5 text-[10px] font-medium">
            compressed
          </span>
        )}
        {displayBlocks.map((block, i) => {
          const isLastBlock = i === displayBlocks.length - 1;
          return (
            <div key={i} data-augmented-ui-reset>
              <BlockRenderer
                block={block}
                streaming={streaming && isLastBlock}
              />
            </div>
          );
        })}
      </div>
    </div>
  );
}
