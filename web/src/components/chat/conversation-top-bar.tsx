import type { AppConfig, ProviderModel, ProviderStatus } from "@/types/metrics";

export function ConversationTopBar({
  connectionStatus,
  conversationId,
  config,
  selectedProvider,
  selectedModel,
  selectableProviders,
  selectedProviderModels,
  overrideActive,
  metricsOpen,
  inspectorOpen,
  onModelOverrideChange,
  onToggleMetrics,
  onToggleInspector,
}: {
  connectionStatus: string;
  conversationId: string | null;
  config: AppConfig | null;
  selectedProvider: string;
  selectedModel: string;
  selectableProviders: ProviderStatus[];
  selectedProviderModels: ProviderModel[];
  overrideActive: boolean;
  metricsOpen: boolean;
  inspectorOpen: boolean;
  onModelOverrideChange: (provider: string, model: string) => void;
  onToggleMetrics: () => void;
  onToggleInspector: () => void;
}) {
  return (
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
                onModelOverrideChange(provider, nextModel);
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
              onChange={(e) => onModelOverrideChange(selectedProvider, e.target.value)}
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
            {overrideActive && (
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
          onClick={onToggleMetrics}
          className={`p-1 text-xs ${metricsOpen ? "bg-muted text-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`}
          title="Conversation metrics"
        >
          📊
        </button>
        <button
          type="button"
          onClick={onToggleInspector}
          className={`p-1 text-xs ${inspectorOpen ? "bg-muted text-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`}
          title="Context inspector"
        >
          🔍
        </button>
      </div>
    </div>
  );
}
