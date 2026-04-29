import { useState, useRef, useEffect, useMemo } from "react";
import { useParams, useLocation, useNavigate } from "react-router-dom";
import {
  useConversation,
  type TurnUsage,
} from "@/hooks/use-conversation";
import { useContextReport } from "@/hooks/use-context-report";
import { useProviders } from "@/hooks/use-providers";
import { api } from "@/lib/api";
import { messageViewsToChat } from "@/lib/history";
import type { Conversation, MessageView } from "@/types/api";
import type { AppConfig, ContextReport, ConversationMetrics } from "@/types/metrics";
import { ContextInspector } from "@/components/inspector/context-inspector";
import { ConversationMetricsPanel } from "@/components/chat/conversation-metrics";
import { ConversationComposer } from "@/components/chat/conversation-composer";
import { ConversationMessageList } from "@/components/chat/conversation-message-list";
import { ConversationTopBar } from "@/components/chat/conversation-top-bar";

const conversationPageSessionState = {
  inspectorOpen: false,
};

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

  return (
    <div className="flex flex-1 overflow-hidden">
      {/* Main chat column */}
      <div className="flex flex-1 flex-col overflow-hidden">
        <ConversationTopBar
          connectionStatus={connectionStatus}
          conversationId={conversationId}
          config={config}
          selectedProvider={selectedProvider}
          selectedModel={selectedModel}
          selectableProviders={selectableProviders}
          selectedProviderModels={selectedProviderModels}
          overrideActive={isConversationOverrideActive}
          metricsOpen={metricsOpen}
          inspectorOpen={inspectorOpen}
          onModelOverrideChange={handleModelOverrideChange}
          onToggleMetrics={() => setMetricsOpen(!metricsOpen)}
          onToggleInspector={() => setInspectorOpen(!inspectorOpen)}
        />

        <ConversationMessageList
          messages={messages}
          streamingText={streamingText}
          isStreaming={isStreaming}
          agentState={agentState}
          error={error}
          usage={displayLastTurnUsage}
          messagesEndRef={messagesEndRef}
        />

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

        <ConversationComposer
          input={input}
          isStreaming={isStreaming}
          onInputChange={setInput}
          onSend={handleSend}
          onCancel={cancel}
        />
      </div>

      {/* Context Inspector panel (right side) */}
      {inspectorOpen && (
        <ContextInspector ctx={ctxReport} onClose={() => setInspectorOpen(false)} />
      )}
    </div>
  );
}
