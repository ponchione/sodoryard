import { useEffect, useMemo, useState, type CSSProperties, type FormEvent } from "react";
import { AlertTriangle, Brain, Check, Copy, Download, KeyRound, RefreshCw, Save, Wrench } from "lucide-react";
import { useProviders } from "@/hooks/use-providers";
import { useProjectInfo } from "@/hooks/use-project-info";
import { ApiError, api } from "@/lib/api";
import { formatTokenLimit } from "@/lib/model-capabilities";
import { getYardPlatform, type YardDesktopPlatformInfo, type YardProjectMemoryPlatform } from "@/platform";
import type { AppConfig, ProviderAuthStatus, ProviderModel, ProviderStatus } from "@/types/metrics";

const panelStyle: CSSProperties = {
  "--aug-tl": "10px",
  "--aug-br": "10px",
  "--aug-border-all": "1px",
  "--aug-border-bg": "#1a2a3a",
} as CSSProperties;

interface ProviderModelOption {
  id: string;
  name?: string;
  context_window?: number;
  supports_tools?: boolean;
  supports_thinking?: boolean;
}

interface ProviderOption {
  name: string;
  type: string;
  status?: string;
  healthy?: boolean;
  last_error?: string;
  auth?: ProviderAuthStatus;
  models: ProviderModelOption[];
}

interface DiagnosticsExport {
  generated_at?: string;
  [key: string]: unknown;
}

interface SettingsPlatformInfo {
  kind: "browser" | "desktop";
  appVersion?: string;
  yardVersion?: string;
  apiVersion?: string;
  backendBaseUrl: string;
  backendDirectUrl?: string;
  backendLaunchMode?: "managed" | "attached";
  projectRoot?: string;
  configPath?: string;
  projectMemory?: YardProjectMemoryPlatform;
}

function formatTimestamp(value?: string): string {
  if (!value) return "Never";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function formatBrainIndexStatus(status?: string): string {
  switch (status) {
    case "clean":
      return "Clean";
    case "stale":
      return "Stale";
    case "never_indexed":
      return "Never indexed";
    default:
      return status ?? "Unknown";
  }
}

function apiErrorMessage(error: unknown): string {
  if (error instanceof ApiError && error.body.trim()) return error.body.trim();
  return error instanceof Error ? error.message : "Request failed";
}

function modelFromRuntime(model: ProviderModel): ProviderModelOption {
  return {
    id: model.id,
    name: model.name,
    context_window: model.context_window,
    supports_tools: model.supports_tools,
    supports_thinking: model.supports_thinking,
  };
}

function modelFromID(id: string): ProviderModelOption {
  return { id, name: id };
}

function mergeModels(...groups: ProviderModelOption[][]): ProviderModelOption[] {
  const merged = new Map<string, ProviderModelOption>();
  for (const group of groups) {
    for (const model of group) {
      const current = merged.get(model.id);
      merged.set(model.id, { ...current, ...model });
    }
  }
  return Array.from(merged.values());
}

function buildProviderOptions(providers: ProviderStatus[], config: AppConfig | null): ProviderOption[] {
  const byName = new Map<string, ProviderOption>();

  for (const provider of config?.providers ?? []) {
    byName.set(provider.name, {
      name: provider.name,
      type: provider.type,
      status: provider.status,
      healthy: provider.healthy,
      last_error: provider.last_error,
      auth: provider.auth,
      models: (provider.models ?? []).map(modelFromID),
    });
  }

  for (const provider of providers) {
    const existing = byName.get(provider.name);
    byName.set(provider.name, {
      name: provider.name,
      type: provider.type || existing?.type || "unknown",
      status: provider.status || existing?.status,
      healthy: provider.healthy ?? existing?.healthy,
      last_error: provider.last_error ?? existing?.last_error,
      auth: provider.auth ?? existing?.auth,
      models: mergeModels(existing?.models ?? [], provider.models.map(modelFromRuntime)),
    });
  }

  const ensureModel = (providerName?: string, modelID?: string) => {
    if (!providerName || !modelID) return;
    const provider = byName.get(providerName);
    if (!provider) return;
    provider.models = mergeModels(provider.models, [modelFromID(modelID)]);
  };
  ensureModel(config?.default_provider, config?.default_model);
  ensureModel(config?.fallback_provider, config?.fallback_model);

  return Array.from(byName.values());
}

function providerStatusClass(provider: ProviderOption): string {
  if (provider.healthy === true || provider.status === "available") return "bg-accent/20 text-accent glow-green";
  if (provider.healthy === false || provider.status === "unavailable") {
    return "bg-destructive/20 text-destructive glow-red";
  }
  return "bg-muted-foreground/10 text-muted-foreground";
}

function credentialStatusLabel(auth?: ProviderAuthStatus): string {
  if (!auth) return "provider credentials unavailable";
  if (auth.has_access_token) {
    if (auth.expires_at) {
      const expiresAt = new Date(auth.expires_at);
      if (!Number.isNaN(expiresAt.getTime()) && expiresAt.getTime() <= Date.now()) {
        return "provider access token expired";
      }
    }
    return "provider access token ready";
  }
  if (auth.has_refresh_token) return "provider refresh token only";
  return "provider credentials missing";
}

function credentialRemediationCommand(remediation?: string): string {
  if (!remediation) return "";
  const quoted = /`([^`]+)`/.exec(remediation);
  if (quoted?.[1]) return quoted[1].trim();
  const yardCommand = /(yard\s+auth\s+login\s+codex)/i.exec(remediation);
  if (yardCommand?.[1]) return yardCommand[1].trim();
  const claudeCommand = /(claude\s+login)/i.exec(remediation);
  return claudeCommand?.[1]?.trim() ?? "";
}

function modelLabel(model: ProviderModelOption): string {
  if (model.name && model.name !== model.id) return `${model.name} (${model.id})`;
  return model.id;
}

function diagnosticsFilename(generatedAt?: string): string {
  const date = generatedAt ? new Date(generatedAt) : new Date();
  const safeDate = Number.isNaN(date.getTime()) ? new Date() : date;
  const stamp = safeDate.toISOString().replace(/[-:]/g, "").replace(/\.\d{3}Z$/, "Z");
  return `yard-diagnostics-${stamp}.json`;
}

function downloadDiagnostics(body: DiagnosticsExport) {
  const blob = new Blob([JSON.stringify(body, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = diagnosticsFilename(body.generated_at);
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

function platformInfoFallback(): SettingsPlatformInfo {
  const platform = getYardPlatform();
  return {
    kind: platform.kind,
    backendBaseUrl: platform.backendBaseUrl,
    projectMemory: platform.projectMemory,
  };
}

function settingsPlatformInfo(info: YardDesktopPlatformInfo | null, fallback: SettingsPlatformInfo): SettingsPlatformInfo {
  if (!info) return fallback;
  return {
    kind: info.kind,
    appVersion: info.appVersion,
    yardVersion: info.yardVersion,
    apiVersion: info.apiVersion,
    backendBaseUrl: info.backendBaseUrl,
    backendDirectUrl: info.backendDirectUrl,
    backendLaunchMode: info.backendLaunchMode,
    projectRoot: info.projectRoot,
    configPath: info.configPath,
    projectMemory: info.projectMemory,
  };
}

function projectMemoryLabel(projectMemory?: YardProjectMemoryPlatform): string {
  if (!projectMemory) return "Unavailable";
  const parts = [
    projectMemory.backend,
    projectMemory.module,
    projectMemory.shunterVersion,
    projectMemory.defaultSubprotocol,
  ].filter(Boolean);
  return parts.length > 0 ? parts.join(" / ") : "Available";
}

export function SettingsPage() {
  const { providers, loading: provLoading, error: providerError, refresh: refreshProviders } = useProviders();
  const { project, loading: projLoading } = useProjectInfo();
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [configLoading, setConfigLoading] = useState(true);
  const [draftProvider, setDraftProvider] = useState("");
  const [draftModel, setDraftModel] = useState("");
  const [saving, setSaving] = useState(false);
  const [saveMessage, setSaveMessage] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [diagnosticsExporting, setDiagnosticsExporting] = useState(false);
  const [diagnosticsMessage, setDiagnosticsMessage] = useState<string | null>(null);
  const [diagnosticsError, setDiagnosticsError] = useState<string | null>(null);
  const [platformInfo, setPlatformInfo] = useState<SettingsPlatformInfo>(() => platformInfoFallback());
  const [providersRefreshing, setProvidersRefreshing] = useState(false);
  const [providersRefreshMessage, setProvidersRefreshMessage] = useState<string | null>(null);
  const [providersRefreshError, setProvidersRefreshError] = useState<string | null>(null);

  useEffect(() => {
    api
      .get<AppConfig>("/api/config")
      .then((c) => {
        setConfig(c);
        setDraftProvider(c.default_provider);
        setDraftModel(c.default_model);
        setConfigLoading(false);
      })
      .catch(() => setConfigLoading(false));
  }, []);

  useEffect(() => {
    let cancelled = false;
    const platform = getYardPlatform();
    const fallback = platformInfoFallback();
    if (!platform.getAppInfo) {
      setPlatformInfo(fallback);
      return;
    }
    platform
      .getAppInfo()
      .then((info) => {
        if (!cancelled) setPlatformInfo(settingsPlatformInfo(info, fallback));
      })
      .catch(() => {
        if (!cancelled) setPlatformInfo(fallback);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const providerOptions = useMemo(() => buildProviderOptions(providers, config), [config, providers]);
  const selectedProvider = providerOptions.find((provider) => provider.name === draftProvider);
  const selectedModels = selectedProvider?.models ?? [];
  const dirty = Boolean(config) && (
    draftProvider !== config?.default_provider || draftModel !== config?.default_model
  );

  const handleProviderChange = (providerName: string) => {
    const provider = providerOptions.find((candidate) => candidate.name === providerName);
    setDraftProvider(providerName);
    setDraftModel((current) => {
      if (provider?.models.some((model) => model.id === current)) return current;
      return provider?.models[0]?.id ?? "";
    });
    setSaveMessage(null);
    setSaveError(null);
  };

  const handleModelChange = (modelID: string) => {
    setDraftModel(modelID);
    setSaveMessage(null);
    setSaveError(null);
  };

  const saveDefaultRoute = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!draftProvider) return;
    setSaving(true);
    setSaveMessage(null);
    setSaveError(null);
    try {
      const updated = await api.put<AppConfig>("/api/config", {
        default_provider: draftProvider,
        default_model: draftModel,
      });
      setConfig(updated);
      setDraftProvider(updated.default_provider);
      setDraftModel(updated.default_model);
      setSaveMessage("Default route saved");
    } catch (error) {
      setSaveError(apiErrorMessage(error));
    } finally {
      setSaving(false);
    }
  };

  const exportDiagnostics = async () => {
    setDiagnosticsExporting(true);
    setDiagnosticsMessage(null);
    setDiagnosticsError(null);
    try {
      const body = await api.post<DiagnosticsExport>("/api/diagnostics/export", {});
      downloadDiagnostics(body);
      setDiagnosticsMessage("Diagnostics export downloaded");
    } catch (error) {
      setDiagnosticsError(apiErrorMessage(error));
    } finally {
      setDiagnosticsExporting(false);
    }
  };

  const refreshProviderStatus = async () => {
    setProvidersRefreshing(true);
    setProvidersRefreshMessage(null);
    setProvidersRefreshError(null);
    try {
      await refreshProviders();
      setProvidersRefreshMessage("Provider status refreshed");
    } catch (error) {
      setProvidersRefreshError(apiErrorMessage(error));
    } finally {
      setProvidersRefreshing(false);
    }
  };

  return (
    <div className="flex-1 overflow-y-auto px-4 py-6">
      <div className="mx-auto max-w-4xl space-y-6">
        <header className="border-b border-border pb-4">
          <h1 className="text-xl font-bold uppercase tracking-widest text-primary text-glow-cyan">
            Settings
          </h1>
          {config && (
            <p className="mt-1 text-xs text-muted-foreground">
              {config.default_provider}:{config.default_model}
            </p>
          )}
        </header>

        <section className="space-y-2">
          <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
            Project
          </h2>
          {projLoading ? (
            <p className="text-xs text-muted-foreground">Loading...</p>
          ) : project ? (
            <div
              data-augmented-ui="tl-clip br-clip border"
              className="space-y-1 border-0 bg-muted p-3 text-sm"
              style={panelStyle}
            >
              <SettingsRow label="Name" value={project.name} />
              <SettingsRow label="Path" value={project.root_path} mono />
              {project.language && <SettingsRow label="Language" value={project.language} />}
              <SettingsRow label="Last indexed" value={formatTimestamp(project.last_indexed_at)} />
              <SettingsRow label="Indexed commit" value={project.last_indexed_commit ?? "Unknown"} mono />
              {project.brain_index && (
                <>
                  <SettingsRow
                    label="Brain index"
                    value={formatBrainIndexStatus(project.brain_index.status)}
                    valueClassName={
                      project.brain_index.status === "stale"
                        ? "text-destructive"
                        : project.brain_index.status === "clean"
                          ? "text-accent"
                          : "text-muted-foreground"
                    }
                  />
                  <SettingsRow label="Brain indexed" value={formatTimestamp(project.brain_index.last_indexed_at)} />
                  {project.brain_index.status === "stale" && (
                    <>
                      <SettingsRow label="Brain stale since" value={formatTimestamp(project.brain_index.stale_since)} />
                      <SettingsRow
                        label="Brain stale reason"
                        value={project.brain_index.stale_reason ?? "Unknown"}
                        mono
                      />
                    </>
                  )}
                </>
              )}
            </div>
          ) : (
            <p className="text-xs text-muted-foreground">No project info available</p>
          )}
        </section>

        <section className="space-y-2">
          <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
            Desktop Runtime
          </h2>
          <div
            data-augmented-ui="tl-clip br-clip border"
            className="space-y-1 border-0 bg-muted p-3 text-sm"
            style={panelStyle}
          >
            <SettingsRow label="App" value={platformInfo.kind} />
            <SettingsRow label="App version" value={platformInfo.appVersion ?? "Unknown"} mono />
            <SettingsRow label="Yard backend" value={platformInfo.yardVersion ?? "Unknown"} mono />
            <SettingsRow label="API" value={platformInfo.apiVersion ?? "Unknown"} mono />
            <SettingsRow label="Backend mode" value={platformInfo.backendLaunchMode ?? "same-origin"} />
            <SettingsRow label="Backend URL" value={platformInfo.backendDirectUrl ?? platformInfo.backendBaseUrl} mono />
            {platformInfo.configPath && <SettingsRow label="Config" value={platformInfo.configPath} mono />}
            <SettingsRow label="Project Memory" value={projectMemoryLabel(platformInfo.projectMemory)} />
          </div>
        </section>

        <section className="space-y-2">
          <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
            Runtime Routing
          </h2>
          {configLoading ? (
            <p className="text-xs text-muted-foreground">Loading...</p>
          ) : config ? (
            <div
              data-augmented-ui="tl-clip br-clip border"
              className="space-y-4 border-0 bg-muted p-3"
              style={panelStyle}
            >
              <form className="space-y-3" onSubmit={saveDefaultRoute}>
                <div className="grid gap-3 md:grid-cols-2">
                  <label className="grid gap-1 text-xs">
                    <span className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
                      Default Provider
                    </span>
                    <select
                      value={draftProvider}
                      onChange={(event) => handleProviderChange(event.target.value)}
                      className="h-9 border border-border bg-background px-2 text-sm text-foreground outline-none focus:border-primary"
                    >
                      {providerOptions.map((provider) => (
                        <option key={provider.name} value={provider.name}>
                          {provider.name}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label className="grid gap-1 text-xs">
                    <span className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
                      Default Model
                    </span>
                    <select
                      value={draftModel}
                      onChange={(event) => handleModelChange(event.target.value)}
                      disabled={selectedModels.length === 0}
                      className="h-9 border border-border bg-background px-2 text-sm text-foreground outline-none focus:border-primary disabled:opacity-50"
                    >
                      {selectedModels.length === 0 ? (
                        <option value="">No model metadata</option>
                      ) : (
                        selectedModels.map((model) => (
                          <option key={model.id} value={model.id}>
                            {modelLabel(model)}
                          </option>
                        ))
                      )}
                    </select>
                  </label>
                </div>
                <div className="grid gap-2 text-xs md:grid-cols-2">
                  <ReadonlySetting
                    label="Current default"
                    value={`${config.default_provider}:${config.default_model}`}
                  />
                  <ReadonlySetting
                    label="Fallback"
                    value={
                      config.fallback_provider || config.fallback_model
                        ? `${config.fallback_provider ?? "unknown"}:${config.fallback_model ?? "unknown"}`
                        : "Not configured"
                    }
                  />
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <button
                    type="submit"
                    disabled={saving || !dirty || !draftProvider}
                    className="inline-flex items-center gap-2 border border-primary/60 px-3 py-2 text-xs font-medium uppercase tracking-widest text-primary hover:bg-primary/10 disabled:pointer-events-none disabled:opacity-50"
                  >
                    <Save size={14} aria-hidden="true" />
                    {saving ? "Saving" : "Save Default"}
                  </button>
                  <span className="text-[10px] uppercase tracking-widest text-muted-foreground">
                    Backend validated
                  </span>
                </div>
                {saveMessage && <StatusMessage tone="success" text={saveMessage} />}
                {saveError && <StatusMessage tone="danger" text={saveError} />}
              </form>

              <div className="space-y-1 text-[10px] text-muted-foreground/70">
                <div>
                  Agent: max {config.agent.max_iterations} iterations, extended thinking{" "}
                  {config.agent.extended_thinking ? "on" : "off"}
                </div>
                <div>Tool output max tokens: {config.agent.tool_output_max_tokens}</div>
                <div>
                  Anthropic prompt cache markers: system {config.agent.cache_system_prompt ? "on" : "off"},
                  context {config.agent.cache_assembled_context ? "on" : "off"}, history{" "}
                  {config.agent.cache_conversation_history ? "on" : "off"}
                </div>
                {config.agent.tool_result_store_root && (
                  <div className="break-all">
                    Persisted tool result store: {config.agent.tool_result_store_root}
                  </div>
                )}
              </div>
            </div>
          ) : (
            <p className="text-xs text-destructive">Config unavailable</p>
          )}
        </section>

        <section className="space-y-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
              Providers
            </h2>
            <button
              type="button"
              onClick={() => void refreshProviderStatus()}
              disabled={providersRefreshing || provLoading}
              className="inline-flex items-center gap-2 border border-border px-2 py-1 text-[10px] font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
            >
              <RefreshCw size={12} aria-hidden="true" />
              {providersRefreshing ? "Refreshing" : "Refresh"}
            </button>
          </div>
          {providerError && <StatusMessage tone="danger" text={providerError} />}
          {providersRefreshMessage && <StatusMessage tone="success" text={providersRefreshMessage} />}
          {providersRefreshError && <StatusMessage tone="danger" text={providersRefreshError} />}
          {provLoading && providerOptions.length === 0 ? (
            <p className="text-xs text-muted-foreground">Loading...</p>
          ) : providerOptions.length === 0 ? (
            <p className="text-xs text-muted-foreground">No providers configured</p>
          ) : (
            <div className="grid gap-2 lg:grid-cols-2">
              {providerOptions.map((provider) => (
                <ProviderCard key={provider.name} provider={provider} />
              ))}
            </div>
          )}
        </section>

        <section className="space-y-2">
          <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
            Diagnostics
          </h2>
          <div
            data-augmented-ui="tl-clip br-clip border"
            className="space-y-3 border-0 bg-muted p-3"
            style={panelStyle}
          >
            <button
              type="button"
              onClick={exportDiagnostics}
              disabled={diagnosticsExporting}
              className="inline-flex items-center gap-2 border border-primary/60 px-3 py-2 text-xs font-medium uppercase tracking-widest text-primary hover:bg-primary/10 disabled:pointer-events-none disabled:opacity-50"
            >
              <Download size={14} aria-hidden="true" />
              {diagnosticsExporting ? "Exporting" : "Export Diagnostics"}
            </button>
            {diagnosticsMessage && <StatusMessage tone="success" text={diagnosticsMessage} />}
            {diagnosticsError && <StatusMessage tone="danger" text={diagnosticsError} />}
          </div>
        </section>
      </div>
    </div>
  );
}

function SettingsRow({
  label,
  value,
  mono = false,
  valueClassName = "",
}: {
  label: string;
  value: string;
  mono?: boolean;
  valueClassName?: string;
}) {
  return (
    <div className="grid gap-1 text-xs sm:grid-cols-[10rem_minmax(0,1fr)]">
      <span className="text-muted-foreground">{label}</span>
      <span className={`min-w-0 break-words sm:text-right ${mono ? "font-mono text-[11px]" : ""} ${valueClassName}`}>
        {value}
      </span>
    </div>
  );
}

function ReadonlySetting({ label, value }: { label: string; value: string }) {
  return (
    <div className="border border-border/80 bg-background/50 px-3 py-2">
      <div className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">{label}</div>
      <div className="mt-1 break-all font-mono text-xs text-foreground">{value}</div>
      <div className="mt-1 text-[10px] uppercase tracking-widest text-muted-foreground/60">Read only</div>
    </div>
  );
}

function StatusMessage({ tone, text }: { tone: "success" | "danger"; text: string }) {
  const success = tone === "success";
  return (
    <div
      className={`flex items-start gap-2 border px-3 py-2 text-xs ${
        success ? "border-accent/50 bg-accent/10 text-accent" : "border-destructive/50 bg-destructive/10 text-destructive"
      }`}
    >
      {success ? <Check size={14} aria-hidden="true" /> : <AlertTriangle size={14} aria-hidden="true" />}
      <span className="break-words">{text}</span>
    </div>
  );
}

function ProviderCard({ provider }: { provider: ProviderOption }) {
  return (
    <div
      data-augmented-ui="tl-clip br-clip border"
      className="space-y-3 border-0 bg-muted p-3"
      style={panelStyle}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium text-sm">{provider.name}</span>
            <span className="bg-muted-foreground/10 px-1.5 py-0.5 text-[10px] text-muted-foreground">
              {provider.type}
            </span>
          </div>
          {provider.last_error && (
            <p className="mt-1 break-words text-[11px] text-destructive">{provider.last_error}</p>
          )}
        </div>
        <span className={`shrink-0 px-2 py-0.5 text-[10px] font-medium ${providerStatusClass(provider)}`}>
          {provider.status ?? "unknown"}
        </span>
      </div>

      <ProviderCredentialDetails auth={provider.auth} providerName={provider.name} />

      {provider.models.length > 0 ? (
        <div className="space-y-1">
          <div className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">Models</div>
          <div className="grid gap-1">
            {provider.models.map((model) => (
              <div key={model.id} className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
                <span className="min-w-0 flex-1 truncate text-foreground">{model.id}</span>
                {model.context_window ? (
                  <span className="shrink-0 text-[10px]">{formatTokenLimit(model.context_window)} ctx</span>
                ) : null}
                {model.supports_tools && <Wrench size={12} aria-label="tools" />}
                {model.supports_thinking && <Brain size={12} aria-label="thinking" />}
              </div>
            ))}
          </div>
        </div>
      ) : (
        <p className="text-xs text-muted-foreground">No model metadata</p>
      )}
    </div>
  );
}

function ProviderCredentialDetails({ auth, providerName }: { auth?: ProviderAuthStatus; providerName: string }) {
  const [copyStatus, setCopyStatus] = useState<string | null>(null);

  if (!auth) {
    return (
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <KeyRound size={13} aria-hidden="true" />
        <span>provider credentials unavailable</span>
      </div>
    );
  }

  const remediationCommand = credentialRemediationCommand(auth.remediation);
  const copyRemediationCommand = async () => {
    if (!remediationCommand) return;
    try {
      await navigator.clipboard.writeText(remediationCommand);
      setCopyStatus("Command copied");
    } catch {
      setCopyStatus("Copy unavailable");
    }
  };

  return (
    <div className="space-y-1 border border-border/70 bg-background/40 p-2 text-xs">
      <div className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
        Provider Credentials
      </div>
      <div className="flex items-center gap-2 text-foreground">
        <KeyRound size={13} aria-hidden="true" />
        <span>{credentialStatusLabel(auth)}</span>
      </div>
      <div className="grid gap-1 text-[11px] text-muted-foreground">
        {auth.mode && <span>mode {auth.mode}</span>}
        {auth.source && <span>source {auth.source}</span>}
        {auth.store_path && <span className="break-all font-mono">store {auth.store_path}</span>}
        {auth.source_path && <span className="break-all font-mono">source path {auth.source_path}</span>}
        {auth.active_provider && <span>active provider {auth.active_provider}</span>}
        {auth.expires_at && <span>expires {formatTimestamp(auth.expires_at)}</span>}
        {auth.last_refresh && <span>last refresh {formatTimestamp(auth.last_refresh)}</span>}
        {auth.detail && <span>{auth.detail}</span>}
        {auth.remediation && <span className="text-warning">{auth.remediation}</span>}
      </div>
      {remediationCommand && (
        <button
          type="button"
          onClick={() => void copyRemediationCommand()}
          className="mt-1 inline-flex items-center gap-1 border border-border px-2 py-1 font-mono text-[10px] text-primary hover:bg-muted"
          aria-label={`Copy ${providerName} provider credential remediation command`}
        >
          <Copy size={11} aria-hidden="true" />
          {copyStatus ?? remediationCommand}
        </button>
      )}
    </div>
  );
}
