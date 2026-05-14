import { useState } from "react";
import { Link } from "react-router-dom";
import {
  Activity,
  AlertTriangle,
  BarChart3,
  FolderTree,
  GitBranch,
  MessageSquare,
  Play,
  Power,
  Rocket,
  Settings,
  Square,
  Terminal,
} from "lucide-react";
import { useApiResource } from "@/hooks/use-api-resource";
import { ApiError, api } from "@/lib/api";
import { useProjectMemoryChains } from "@/hooks/use-project-memory-chains";
import { chainStatusClass } from "@/lib/chain-status";
import { formatModelCapabilitySummary, formatTokenLimit } from "@/lib/model-capabilities";
import type { ConversationSummary } from "@/types/api";
import type { ChainSummary, RuntimeIndexStatus, RuntimeStatus } from "@/types/chains";

function formatDate(value?: string): string {
  if (!value) return "unknown";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function taskLabel(chain: ChainSummary): string {
  return chain.source_task || chain.source_specs.join(", ") || "No task recorded";
}

function indexSummary(index: RuntimeIndexStatus): string {
  if (index.stale_reason) return `${index.status}: ${index.stale_reason}`;
  if (index.last_indexed_at) return `${index.status}: ${formatDate(index.last_indexed_at)}`;
  return index.status;
}

function projectMemoryLabel(status: string, rows: number | null, events: number | null): string {
  if (status !== "connected") return status;
  const parts = ["connected"];
  if (rows !== null) parts.push(`${rows} chains`);
  if (events !== null) parts.push(`${events} events`);
  return parts.join(" / ");
}

function recentConversationTitle(conversation: ConversationSummary): string {
  return conversation.title?.trim() || "Untitled";
}

interface LocalServiceStatus {
  name: string;
  healthy: boolean;
  reachable: boolean;
  models_ready: boolean;
  required: boolean;
  base_url?: string;
  detail?: string;
}

interface LocalServicesStatus {
  mode: string;
  compose_file?: string;
  project_dir?: string;
  docker_available: boolean;
  daemon_available: boolean;
  compose_available: boolean;
  compose_file_exists: boolean;
  services: LocalServiceStatus[];
  required_services?: string[];
  problems?: string[];
  remediation?: string[];
}

interface LocalServicesCommandResponse {
  status: LocalServicesStatus;
  message: string;
  error?: string;
}

interface LocalServicesLogsResponse {
  tail: number;
  logs: string;
}

function localServicesSummary(status: LocalServicesStatus | null, runtimeMode?: string): string {
  if (runtimeMode === "disabled") return "disabled";
  if (!status) return "unknown";
  if (status.mode === "disabled") return "disabled";
  if ((status.problems ?? []).length > 0) return "attention";
  if (status.services.length === 0) return status.mode || "configured";
  const required = status.services.filter((service) => service.required);
  if (required.length > 0 && required.every((service) => service.healthy)) return "ready";
  if (status.services.every((service) => service.healthy)) return "ready";
  return "not ready";
}

function apiErrorMessage(error: unknown): string {
  if (error instanceof ApiError && error.body.trim()) return error.body.trim();
  return error instanceof Error ? error.message : "Request failed";
}

export function DashboardPage() {
  const { data: runtime, loading: runtimeLoading, error: runtimeError, refresh: refreshRuntime } = (
    useApiResource<RuntimeStatus | null>("/api/runtime/status", null)
  );
  const { data: chains, loading: chainsLoading, error: chainsError, refresh: refreshChains } = (
    useApiResource<ChainSummary[]>("/api/chains?limit=5", [])
  );
  const { data: conversations, loading: conversationsLoading, error: conversationsError } = (
    useApiResource<ConversationSummary[]>("/api/conversations?limit=5", [])
  );
  const {
    data: localServices,
    loading: localServicesLoading,
    error: localServicesError,
    refresh: refreshLocalServices,
  } = useApiResource<LocalServicesStatus | null>("/api/runtime/local-services", null);
  const projectMemory = useProjectMemoryChains({ onChanged: refreshChains });
  const [localServicesAction, setLocalServicesAction] = useState<"up" | "down" | "logs" | null>(null);
  const [localServicesMessage, setLocalServicesMessage] = useState<string | null>(null);
  const [localServicesActionError, setLocalServicesActionError] = useState<string | null>(null);
  const [localServicesLogs, setLocalServicesLogs] = useState<string | null>(null);
  const localServicesDisabled = runtime?.local_services_status === "disabled" || localServices?.mode === "disabled";

  const refreshAll = () => {
    void refreshRuntime();
    void refreshChains();
    void refreshLocalServices();
    void projectMemory.refresh();
  };

  const runLocalServicesCommand = async (action: "up" | "down") => {
    setLocalServicesAction(action);
    setLocalServicesMessage(null);
    setLocalServicesActionError(null);
    try {
      const result = await api.post<LocalServicesCommandResponse>(`/api/runtime/local-services/${action}`, {});
      setLocalServicesMessage(result.message);
      setLocalServicesLogs(null);
      await refreshLocalServices();
      await refreshRuntime();
    } catch (error) {
      setLocalServicesActionError(apiErrorMessage(error));
    } finally {
      setLocalServicesAction(null);
    }
  };

  const loadLocalServicesLogs = async () => {
    setLocalServicesAction("logs");
    setLocalServicesMessage(null);
    setLocalServicesActionError(null);
    try {
      const result = await api.get<LocalServicesLogsResponse>("/api/runtime/local-services/logs?tail=80");
      setLocalServicesLogs(result.logs || "No logs returned.");
    } catch (error) {
      setLocalServicesActionError(apiErrorMessage(error));
    } finally {
      setLocalServicesAction(null);
    }
  };

  return (
    <div className="flex-1 overflow-y-auto px-4 py-6">
      <div className="mx-auto flex max-w-6xl flex-col gap-5">
        <header className="flex flex-col gap-3 border-b border-border pb-4 md:flex-row md:items-end md:justify-between">
          <div className="min-w-0">
            <h1 className="text-xl font-bold uppercase tracking-widest text-primary text-glow-cyan">
              Dashboard
            </h1>
            <p className="mt-1 truncate text-xs text-muted-foreground">
              {runtime ? `${runtime.project_name} / ${runtime.project_root}` : "Runtime status loading"}
            </p>
            {runtime && (
              <p className="mt-1 text-xs text-muted-foreground">
                {runtime.provider}:{runtime.model} / {formatTokenLimit(runtime.context_window)} context / auth{" "}
                {runtime.auth_status}
              </p>
            )}
          </div>
          <div className="flex flex-wrap gap-2">
            <ActionLink to="/launch" icon={<Rocket size={15} aria-hidden="true" />} label="Launch" />
            <ActionLink to="/project" icon={<FolderTree size={15} aria-hidden="true" />} label="Project" />
            <ActionLink to="/" icon={<MessageSquare size={15} aria-hidden="true" />} label="Chat" />
            <ActionLink to="/chains" icon={<GitBranch size={15} aria-hidden="true" />} label="Chains" />
            <ActionLink to="/metrics" icon={<BarChart3 size={15} aria-hidden="true" />} label="Metrics" />
            <button
              type="button"
              onClick={refreshAll}
              className="inline-flex items-center gap-2 border border-border px-3 py-2 text-xs font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary"
            >
              <Activity size={15} aria-hidden="true" />
              Refresh
            </button>
          </div>
        </header>

        {runtimeError && <StatusBanner tone="danger" text={runtimeError} />}
        {chainsError && <StatusBanner tone="danger" text={chainsError} />}
        {conversationsError && <StatusBanner tone="danger" text={conversationsError} />}
        {localServicesError && <StatusBanner tone="danger" text={localServicesError} />}
        {projectMemory.error && <StatusBanner tone="warning" text={`project memory: ${projectMemory.error}`} />}

        {runtime && runtime.warnings.length > 0 && (
          <section className="border border-warning/50 bg-warning/5 p-3">
            <h2 className="flex items-center gap-2 text-[10px] font-semibold uppercase tracking-widest text-warning">
              <AlertTriangle size={14} aria-hidden="true" />
              Readiness
            </h2>
            <div className="mt-2 grid gap-1 text-xs text-warning">
              {runtime.warnings.map((warning) => (
                <p key={warning.message}>{warning.message}</p>
              ))}
            </div>
          </section>
        )}

        <section className="grid gap-3 md:grid-cols-2 xl:grid-cols-5">
          <RuntimeMetric label="Active Chains" value={runtime?.active_chains ?? (runtimeLoading ? "..." : "0")} />
          <RuntimeMetric label="Code Index" value={runtime ? indexSummary(runtime.code_index) : "..."} />
          <RuntimeMetric label="Brain Index" value={runtime ? indexSummary(runtime.brain_index) : "..."} />
          <RuntimeMetric
            label="Local Services"
            value={localServicesLoading ? "..." : `${runtime?.local_services_status ?? "..."} / ${localServicesSummary(localServices, runtime?.local_services_status)}`}
          />
          <RuntimeMetric
            label="Project Memory"
            value={projectMemoryLabel(projectMemory.status, projectMemory.rowCount, projectMemory.eventCount)}
          />
        </section>

        <section className="border border-border">
          <div className="flex flex-col gap-3 border-b border-border bg-muted px-3 py-2 md:flex-row md:items-center md:justify-between">
            <div className="min-w-0">
              <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
                Readiness Actions
              </h2>
              <p className="mt-1 text-xs text-muted-foreground">
                Local services {localServicesSummary(localServices, runtime?.local_services_status)}
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <button
                type="button"
                onClick={() => void runLocalServicesCommand("up")}
                disabled={localServicesAction !== null || localServicesDisabled}
                className="inline-flex items-center gap-2 border border-border px-3 py-2 text-xs font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
              >
                <Power size={14} aria-hidden="true" />
                {localServicesAction === "up" ? "Starting" : "Start"}
              </button>
              <button
                type="button"
                onClick={() => void runLocalServicesCommand("down")}
                disabled={localServicesAction !== null || localServicesDisabled}
                className="inline-flex items-center gap-2 border border-border px-3 py-2 text-xs font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
              >
                <Square size={14} aria-hidden="true" />
                {localServicesAction === "down" ? "Stopping" : "Stop"}
              </button>
              <button
                type="button"
                onClick={() => void loadLocalServicesLogs()}
                disabled={localServicesAction !== null || localServicesDisabled}
                className="inline-flex items-center gap-2 border border-border px-3 py-2 text-xs font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
              >
                <Terminal size={14} aria-hidden="true" />
                {localServicesAction === "logs" ? "Loading Logs" : "Logs"}
              </button>
            </div>
          </div>
          <div className="grid gap-3 p-3 text-xs text-muted-foreground xl:grid-cols-[minmax(0,1fr)_minmax(260px,0.55fr)]">
            <div className="min-w-0">
              {localServicesActionError && <p className="mb-2 text-destructive">{localServicesActionError}</p>}
              {localServicesMessage && <p className="mb-2 text-accent">{localServicesMessage}</p>}
              {localServicesLoading && <p>Loading local service status...</p>}
              {!localServicesLoading && localServices && (
                <div className="grid gap-2">
                  <div className="grid gap-2 md:grid-cols-4">
                    <ReadinessFlag label="Docker" ok={localServices.docker_available} />
                    <ReadinessFlag label="Daemon" ok={localServices.daemon_available} />
                    <ReadinessFlag label="Compose" ok={localServices.compose_available} />
                    <ReadinessFlag label="Compose File" ok={localServices.compose_file_exists} />
                  </div>
                  <div className="divide-y divide-border/70 border border-border">
                    {localServices.services.length === 0 && (
                      <p className="px-3 py-2">No managed local services configured.</p>
                    )}
                    {localServices.services.map((service) => (
                      <div
                        key={service.name}
                        className="grid gap-2 px-3 py-2 md:grid-cols-[minmax(0,1fr)_90px_90px_90px]"
                      >
                        <div className="min-w-0">
                          <span className="font-mono text-foreground">{service.name}</span>
                          {service.required && <span className="ml-2 text-[10px] uppercase tracking-widest">required</span>}
                          {service.detail && <div className="mt-1 truncate text-muted-foreground">{service.detail}</div>}
                        </div>
                        <span className={service.healthy ? "text-accent" : "text-destructive"}>
                          {service.healthy ? "healthy" : "unhealthy"}
                        </span>
                        <span>{service.reachable ? "reachable" : "offline"}</span>
                        <span>{service.models_ready ? "models" : "no models"}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
            <div className="grid gap-2">
              {(localServices?.problems ?? []).map((problem) => (
                <p key={problem} className="text-warning">{problem}</p>
              ))}
              {(localServices?.remediation ?? []).map((line) => (
                <p key={line}>{line}</p>
              ))}
              {localServicesLogs && (
                <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-words border border-border bg-background p-2 font-mono text-[11px] text-foreground">
                  {localServicesLogs}
                </pre>
              )}
            </div>
          </div>
        </section>

        {runtime && (
          <section className="border border-border">
            <div className="border-b border-border bg-muted px-3 py-2">
              <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
                Model
              </h2>
            </div>
            <div className="grid gap-2 px-3 py-3 text-xs text-muted-foreground md:grid-cols-[minmax(0,1fr)_auto]">
              <p className="min-w-0">{formatModelCapabilitySummary(runtime.model_capabilities)}</p>
              {runtime.model_capabilities.max_output_tokens > 0 && (
                <p>{formatTokenLimit(runtime.model_capabilities.max_output_tokens)} output</p>
              )}
            </div>
          </section>
        )}

        <div className="grid gap-5 xl:grid-cols-[minmax(0,1.6fr)_minmax(280px,0.9fr)]">
          <section className="min-w-0 border border-border">
            <SectionHeader title="Recent Chains" link="/chains" linkLabel="View all" />
            {chainsLoading && <EmptyLine text="Loading chains..." />}
            {!chainsLoading && chains.length === 0 && <EmptyLine text="No chains recorded." />}
            <div className="divide-y divide-border/70">
              {chains.map((chain) => (
                <Link
                  key={chain.id}
                  to={`/chains/${chain.id}`}
                  className="grid gap-2 px-3 py-3 text-xs hover:bg-muted/40 md:grid-cols-[minmax(0,1fr)_120px_120px]"
                >
                  <div className="min-w-0">
                    <div className="truncate font-mono text-primary">{chain.id}</div>
                    <div className="mt-1 truncate text-muted-foreground">{taskLabel(chain)}</div>
                  </div>
                  <div className={`font-medium ${chainStatusClass(chain.status)}`}>{chain.status}</div>
                  <div className="text-muted-foreground md:text-right">{formatDate(chain.updated_at)}</div>
                </Link>
              ))}
            </div>
          </section>

          <section className="min-w-0 border border-border">
            <SectionHeader title="Recent Conversations" link="/" linkLabel="Chat" />
            {conversationsLoading && <EmptyLine text="Loading conversations..." />}
            {!conversationsLoading && conversations.length === 0 && <EmptyLine text="No conversations recorded." />}
            <div className="divide-y divide-border/70">
              {conversations.map((conversation) => (
                <Link
                  key={conversation.id}
                  to={`/c/${conversation.id}`}
                  className="block px-3 py-3 text-xs hover:bg-muted/40"
                >
                  <div className="truncate text-foreground">{recentConversationTitle(conversation)}</div>
                  <div className="mt-1 text-muted-foreground">{formatDate(conversation.updated_at)}</div>
                </Link>
              ))}
            </div>
          </section>
        </div>

        <section className="border border-border">
          <SectionHeader title="Recent Project Memory Events" link="/chains" linkLabel="Chains" />
          {projectMemory.status !== "connected" && (
            <EmptyLine text={`Project memory ${projectMemory.status}.`} />
          )}
          {projectMemory.status === "connected" && projectMemory.recentEvents.length === 0 && (
            <EmptyLine text="No recent project memory events." />
          )}
          {projectMemory.status === "connected" && projectMemory.recentEvents.length > 0 && (
            <div className="divide-y divide-border/70">
              {projectMemory.recentEvents.slice(0, 6).map((event) => (
                <Link
                  key={event.id}
                  to={`/chains/${event.chainId}`}
                  className="grid gap-2 px-3 py-2 text-xs hover:bg-muted/40 md:grid-cols-[minmax(0,1fr)_auto]"
                >
                  <div className="min-w-0">
                    <span className="font-medium text-foreground">{event.eventType}</span>
                    <span className="ml-2 font-mono text-muted-foreground">{event.chainId}</span>
                  </div>
                  <div className="text-muted-foreground md:text-right">{formatDate(event.createdAt)}</div>
                </Link>
              ))}
            </div>
          )}
        </section>

        <section className="grid gap-3 md:grid-cols-2 xl:grid-cols-6">
          <ActionBlock to="/launch" icon={<Rocket size={16} aria-hidden="true" />} title="Launch" />
          <ActionBlock to="/project" icon={<FolderTree size={16} aria-hidden="true" />} title="Project" />
          <ActionBlock to="/" icon={<MessageSquare size={16} aria-hidden="true" />} title="Chat" />
          <ActionBlock to="/chains" icon={<Play size={16} aria-hidden="true" />} title="Monitor Chains" />
          <ActionBlock to="/metrics" icon={<BarChart3 size={16} aria-hidden="true" />} title="Metrics" />
          <ActionBlock to="/settings" icon={<Settings size={16} aria-hidden="true" />} title="Settings" />
        </section>
      </div>
    </div>
  );
}

function RuntimeMetric({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="min-w-0 border border-border px-3 py-3">
      <div className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">{label}</div>
      <div className="mt-2 truncate text-sm font-medium text-foreground">{value}</div>
    </div>
  );
}

function ReadinessFlag({ label, ok }: { label: string; ok: boolean }) {
  return (
    <div className="border border-border px-2 py-2">
      <div className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">{label}</div>
      <div className={`mt-1 font-medium ${ok ? "text-accent" : "text-destructive"}`}>{ok ? "ok" : "missing"}</div>
    </div>
  );
}

function SectionHeader({ title, link, linkLabel }: { title: string; link: string; linkLabel: string }) {
  return (
    <div className="flex items-center justify-between gap-3 border-b border-border bg-muted px-3 py-2">
      <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">{title}</h2>
      <Link to={link} className="text-[10px] font-medium uppercase tracking-widest text-primary hover:underline">
        {linkLabel}
      </Link>
    </div>
  );
}

function EmptyLine({ text }: { text: string }) {
  return <p className="px-3 py-3 text-xs text-muted-foreground">{text}</p>;
}

function StatusBanner({ text, tone }: { text: string; tone: "danger" | "warning" }) {
  const toneClass = tone === "danger" ? "border-destructive/50 text-destructive" : "border-warning/50 text-warning";
  return <div className={`border bg-background px-3 py-2 text-xs ${toneClass}`}>{text}</div>;
}

function ActionLink({ to, icon, label }: { to: string; icon: React.ReactNode; label: string }) {
  return (
    <Link
      to={to}
      className="inline-flex items-center gap-2 border border-border px-3 py-2 text-xs font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary"
    >
      {icon}
      {label}
    </Link>
  );
}

function ActionBlock({ to, icon, title }: { to: string; icon: React.ReactNode; title: string }) {
  return (
    <Link
      to={to}
      className="flex items-center gap-3 border border-border px-3 py-3 text-sm font-medium text-foreground hover:border-primary hover:text-primary"
    >
      {icon}
      {title}
    </Link>
  );
}
