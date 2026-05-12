import { useState, useRef, useEffect } from "react";
import { useParams, useLocation, useNavigate } from "react-router-dom";
import {
  useConversation,
  type TurnUsage,
} from "@/hooks/use-conversation";
import { useContextReport } from "@/hooks/use-context-report";
import { api } from "@/lib/api";
import { messageViewsToChat } from "@/lib/history";
import type { MessageView } from "@/types/api";
import type { AppConfig, ContextReport, ConversationMetrics } from "@/types/metrics";
import { ContextInspector } from "@/components/inspector/context-inspector";
import { ConversationMetricsPanel } from "@/components/chat/conversation-metrics";
import { ConversationComposer } from "@/components/chat/conversation-composer";
import { ConversationMessageList } from "@/components/chat/conversation-message-list";
import { ConversationTopBar } from "@/components/chat/conversation-top-bar";

const conversationPageSessionState = {
  inspectorOpen: false,
};

const HISTORY_MESSAGE_LIMIT = 200;

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
    cancel,
    loadHistory,
  } = useConversation(convId);

  const [input, setInput] = useState("");
  const [inspectorOpen, setInspectorOpen] = useState(() => conversationPageSessionState.inspectorOpen);
  const [metricsOpen, setMetricsOpen] = useState(false);
  const [config, setConfig] = useState<AppConfig | null>(null);
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
  const { setHistoryTurns, setLiveReport } = ctxReport;
  // Derived: prefer the live state value over hydrated, so ongoing turns
  // override stale page-load data.
  const displayLastTurnUsage: TurnUsage | null = lastTurnUsage ?? hydratedLastTurn;

  // Feed live context_debug events into the inspector.
  useEffect(() => {
    if (lastContextDebug) {
      setHistoryLatestTurnPending(false);
      setLiveReport(lastContextDebug as unknown as ContextReport);
    }
  }, [lastContextDebug, setLiveReport]);
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

  // Load existing conversation history on mount.
  useEffect(() => {
    if (convId && !historyLoaded.current) {
      historyLoaded.current = true;
      api
        .get<MessageView[]>(`/api/conversations/${convId}/messages?limit=${HISTORY_MESSAGE_LIMIT}`)
        .then((views) => {
          const chatMessages = messageViewsToChat(views);
          loadHistory(chatMessages);
          const maxTurn = views.reduce((max, view) => Math.max(max, view.turn_number ?? 0), 0);
          const latestTurnHasAssistant = views.some(
            (view) => view.turn_number === maxTurn && view.role === "assistant",
          );
          setHistoryLatestTurnPending(maxTurn > 0 && !latestTurnHasAssistant);
          setHistoryTurns(maxTurn);
        })
        .catch((err) => {
          console.error("Failed to load conversation history:", err);
        });
    }
  }, [convId, loadHistory, setHistoryTurns]);

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
    if (sendMessage(text)) {
      setInput("");
    }
  };

  return (
    <div className="flex flex-1 overflow-hidden">
      {/* Main chat column */}
      <div className="flex flex-1 flex-col overflow-hidden">
        <ConversationTopBar
          connectionStatus={connectionStatus}
          conversationId={conversationId}
          config={config}
          metricsOpen={metricsOpen}
          inspectorOpen={inspectorOpen}
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
          canSend={connectionStatus === "connected"}
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
