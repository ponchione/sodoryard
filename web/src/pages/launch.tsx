import { useEffect, useMemo, useRef, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { AlertTriangle, Check, Eye, Plus, RefreshCw, Rocket, X } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { useApiResource } from "@/hooks/use-api-resource";
import { ApiError, api } from "@/lib/api";
import { formatTokenLimit } from "@/lib/model-capabilities";
import type {
  AgentRoleSummary,
  LaunchDraftRead,
  LaunchMode,
  LaunchPreset,
  LaunchPreview,
  LaunchRequest,
  LaunchStartResponse,
  LaunchTemplate,
  RuntimeStatus,
} from "@/types/chains";

interface LaunchFormState {
  mode: LaunchMode;
  templateId: string;
  sourceTask: string;
  sourceSpecsText: string;
  role: string;
  roster: string[];
  rosterCandidate: string;
  allowedRoles: string[];
  maxSteps: string;
  maxResolverLoops: string;
  maxDuration: string;
  tokenBudget: string;
  stepMaxTurns: string;
  stepMaxTokens: string;
  allowApprovalWait: boolean;
}

const modeOptions: Array<{ mode: LaunchMode; label: string }> = [
  { mode: "one_step_chain", label: "One Step" },
  { mode: "manual_roster", label: "Manual Roster" },
  { mode: "sir_topham_decides", label: "Orchestrated" },
  { mode: "constrained_orchestration", label: "Constrained" },
];

const emptyForm: LaunchFormState = {
  mode: "one_step_chain",
  templateId: "one_step",
  sourceTask: "",
  sourceSpecsText: "",
  role: "",
  roster: [],
  rosterCandidate: "",
  allowedRoles: [],
  maxSteps: "",
  maxResolverLoops: "",
  maxDuration: "",
  tokenBudget: "",
  stepMaxTurns: "",
  stepMaxTokens: "",
  allowApprovalWait: false,
};

function compactList(values: string[]): string[] {
  return values.map((value) => value.trim()).filter(Boolean);
}

function splitList(value: string): string[] {
  return compactList(value.split(/[\n,]/));
}

function unique(values: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  values.forEach((value) => {
    if (seen.has(value)) return;
    seen.add(value);
    out.push(value);
  });
  return out;
}

function optionalInt(value: string): number | undefined {
  const text = value.trim();
  if (!text) return undefined;
  const parsed = Number.parseInt(text, 10);
  return Number.isNaN(parsed) ? undefined : parsed;
}

function roleNames(roles: AgentRoleSummary[]): string[] {
  return roles.map((role) => role.name).filter(Boolean);
}

function preferredRole(roles: string[], current?: string): string {
  if (current && roles.includes(current)) return current;
  if (roles.includes("coder")) return "coder";
  return roles[0] ?? current ?? "";
}

function defaultRoster(roles: string[]): string[] {
  const preferred = ["planner", "coder"].filter((role) => roles.includes(role));
  if (preferred.length > 0) return preferred;
  return roles[0] ? [roles[0]] : [];
}

function defaultAllowedRoles(roles: string[]): string[] {
  const preferred = ["planner", "coder", "correctness-auditor"].filter((role) => roles.includes(role));
  if (preferred.length > 0) return preferred;
  return roles.slice(0, 2);
}

function templateForMode(templates: LaunchTemplate[], mode: LaunchMode): LaunchTemplate | undefined {
  return templates.find((template) => template.mode === mode);
}

function formFromRequest(
  request: Partial<LaunchRequest> | undefined,
  roles: string[],
  templates: LaunchTemplate[],
): LaunchFormState {
  const mode = request?.mode ?? "one_step_chain";
  const templateId = request?.template_id ?? templateForMode(templates, mode)?.id ?? "";
  const roster = request?.roster && request.roster.length > 0 ? request.roster : defaultRoster(roles);
  const allowedRoles = request?.allowed_roles && request.allowed_roles.length > 0
    ? request.allowed_roles
    : defaultAllowedRoles(roles);
  const role = preferredRole(roles, request?.role);
  return {
    ...emptyForm,
    mode,
    templateId,
    sourceTask: request?.source_task ?? "",
    sourceSpecsText: request?.source_specs?.join("\n") ?? "",
    role,
    roster,
    rosterCandidate: preferredRole(roles, roster[0]),
    allowedRoles,
    maxSteps: request?.max_steps ? String(request.max_steps) : "",
    maxResolverLoops: request?.max_resolver_loops ? String(request.max_resolver_loops) : "",
    maxDuration: request?.max_duration ?? "",
    tokenBudget: request?.token_budget ? String(request.token_budget) : "",
    stepMaxTurns: request?.step_max_turns ? String(request.step_max_turns) : "",
    stepMaxTokens: request?.step_max_tokens ? String(request.step_max_tokens) : "",
    allowApprovalWait: request?.allow_approval_wait ?? false,
  };
}

function buildLaunchRequest(form: LaunchFormState, templates: LaunchTemplate[]): LaunchRequest {
  const specs = splitList(form.sourceSpecsText);
  const request: LaunchRequest = {
    template_id: form.templateId || templateForMode(templates, form.mode)?.id,
    mode: form.mode,
    source_task: form.sourceTask.trim() || undefined,
    source_specs: specs.length > 0 ? specs : undefined,
    max_steps: optionalInt(form.maxSteps),
    max_resolver_loops: optionalInt(form.maxResolverLoops),
    max_duration: form.maxDuration.trim() || undefined,
    token_budget: optionalInt(form.tokenBudget),
    step_max_turns: optionalInt(form.stepMaxTurns),
    step_max_tokens: optionalInt(form.stepMaxTokens),
    allow_approval_wait: form.allowApprovalWait || undefined,
  };

  if (form.mode === "one_step_chain") request.role = form.role || undefined;
  if (form.mode === "manual_roster") request.roster = form.roster.length > 0 ? form.roster : undefined;
  if (form.mode === "constrained_orchestration") {
    request.allowed_roles = form.allowedRoles.length > 0 ? form.allowedRoles : undefined;
  }
  return request;
}

function errorMessage(error: unknown): string {
  if (error instanceof ApiError && error.body.trim()) return error.body.trim();
  return error instanceof Error ? error.message : "Request failed";
}

function modeLabel(mode: LaunchMode): string {
  return modeOptions.find((option) => option.mode === mode)?.label ?? mode;
}

export function LaunchPage() {
  const navigate = useNavigate();
  const { data: runtime, error: runtimeError } = useApiResource<RuntimeStatus | null>("/api/runtime/status", null);
  const { data: roles, loading: rolesLoading, error: rolesError } = useApiResource<AgentRoleSummary[]>("/api/roles", []);
  const { data: templates, loading: templatesLoading, error: templatesError } = (
    useApiResource<LaunchTemplate[]>("/api/chains/templates", [])
  );
  const { data: draftRead, loading: draftLoading, error: draftError } = (
    useApiResource<LaunchDraftRead>("/api/launch/draft", { found: false })
  );
  const { data: presets, error: presetsError } = useApiResource<LaunchPreset[]>("/api/launch/presets", []);
  const rolesList = useMemo(() => roleNames(roles), [roles]);
  const [form, setForm] = useState<LaunchFormState>(emptyForm);
  const [preview, setPreview] = useState<LaunchPreview | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const [startLoading, setStartLoading] = useState(false);
  const [startError, setStartError] = useState<string | null>(null);
  const initialized = useRef(false);

  useEffect(() => {
    if (initialized.current || rolesLoading || templatesLoading || draftLoading) return;
    const nextForm = formFromRequest(draftRead.found ? draftRead.draft?.request : undefined, rolesList, templates);
    setForm(nextForm);
    initialized.current = true;
  }, [draftLoading, draftRead, rolesList, rolesLoading, templates, templatesLoading]);

  const selectedTemplate = templateForMode(templates, form.mode);
  const currentRequest = useMemo(() => buildLaunchRequest(form, templates), [form, templates]);

  const updateMode = (mode: LaunchMode) => {
    const template = templateForMode(templates, mode);
    setForm((current) => ({
      ...current,
      mode,
      templateId: template?.id ?? "",
      role: current.role || preferredRole(rolesList),
      roster: current.roster.length > 0 ? current.roster : defaultRoster(rolesList),
      rosterCandidate: current.rosterCandidate || preferredRole(rolesList),
      allowedRoles: current.allowedRoles.length > 0 ? current.allowedRoles : defaultAllowedRoles(rolesList),
    }));
    setPreview(null);
    setPreviewError(null);
    setStartError(null);
  };

  const applyTemplate = (template: LaunchTemplate) => {
    updateMode(template.mode);
    setForm((current) => ({ ...current, templateId: template.id }));
  };

  const applyPreset = (preset: LaunchPreset) => {
    setForm(formFromRequest(preset.request, rolesList, templates));
    setPreview(null);
    setPreviewError(null);
    setStartError(null);
  };

  const addRosterRole = () => {
    if (!form.rosterCandidate) return;
    setForm((current) => ({
      ...current,
      roster: unique([...current.roster, current.rosterCandidate]),
    }));
  };

  const removeRosterRole = (role: string) => {
    setForm((current) => ({ ...current, roster: current.roster.filter((item) => item !== role) }));
  };

  const toggleAllowedRole = (role: string) => {
    setForm((current) => {
      const selected = current.allowedRoles.includes(role);
      return {
        ...current,
        allowedRoles: selected
          ? current.allowedRoles.filter((item) => item !== role)
          : unique([...current.allowedRoles, role]),
      };
    });
  };

  const resetForm = () => {
    setForm(formFromRequest(undefined, rolesList, templates));
    setPreview(null);
    setPreviewError(null);
    setStartError(null);
  };

  const handlePreview = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setPreviewLoading(true);
    setPreviewError(null);
    setStartError(null);
    try {
      const result = await api.post<LaunchPreview>("/api/launch/preview", currentRequest);
      setPreview(result);
    } catch (error) {
      setPreviewError(errorMessage(error));
    } finally {
      setPreviewLoading(false);
    }
  };

  const handleStart = async () => {
    setStartLoading(true);
    setPreviewError(null);
    setStartError(null);
    try {
      const result = await api.post<LaunchStartResponse>("/api/launch/start", currentRequest);
      setPreview(result.preview);
      navigate(`/chains/${result.chain_id}`);
    } catch (error) {
      setStartError(errorMessage(error));
    } finally {
      setStartLoading(false);
    }
  };

  const pageErrors = [runtimeError, rolesError, templatesError, draftError, presetsError].filter(Boolean);

  return (
    <div className="flex-1 overflow-y-auto px-4 py-6">
      <div className="mx-auto flex max-w-6xl flex-col gap-5">
        <header className="flex flex-col gap-3 border-b border-border pb-4 md:flex-row md:items-end md:justify-between">
          <div className="min-w-0">
            <h1 className="text-xl font-bold uppercase tracking-widest text-primary text-glow-cyan">
              Launch Workbench
            </h1>
            <p className="mt-1 truncate text-xs text-muted-foreground">
              {runtime ? `${runtime.project_name} / ${runtime.provider}:${runtime.model}` : "Runtime status loading"}
            </p>
            {runtime && (
              <p className="mt-1 text-xs text-muted-foreground">
                {formatTokenLimit(runtime.context_window)} context / auth {runtime.auth_status}
              </p>
            )}
          </div>
          <div className="flex items-center gap-2 text-[10px] font-medium uppercase tracking-widest text-muted-foreground">
            <span className="border border-border px-2 py-1">{modeLabel(form.mode)}</span>
            {selectedTemplate?.receipt_schema && (
              <span className="border border-border px-2 py-1">{selectedTemplate.receipt_schema}</span>
            )}
          </div>
        </header>

        {pageErrors.map((error) => (
          <StatusBanner key={error} tone="danger">
            {error}
          </StatusBanner>
        ))}

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

        <form onSubmit={handlePreview} className="grid gap-5 xl:grid-cols-[minmax(0,1.2fr)_minmax(320px,0.8fr)]">
          <div className="space-y-5">
            <section className="border border-border">
              <SectionHeader title="Mode" />
              <div className="grid gap-2 p-3 md:grid-cols-4">
                {modeOptions.map((option) => (
                  <button
                    key={option.mode}
                    type="button"
                    aria-pressed={form.mode === option.mode}
                    onClick={() => updateMode(option.mode)}
                    className={`border px-3 py-2 text-left text-xs font-medium uppercase tracking-widest ${
                      form.mode === option.mode
                        ? "border-primary text-primary"
                        : "border-border text-muted-foreground hover:border-primary hover:text-primary"
                    }`}
                  >
                    {option.label}
                  </button>
                ))}
              </div>
            </section>

            <section className="border border-border">
              <SectionHeader title="Work Packet" />
              <div className="grid gap-3 p-3">
                <label className="grid gap-1 text-xs">
                  <span className="font-medium uppercase tracking-widest text-muted-foreground">Task</span>
                  <textarea
                    value={form.sourceTask}
                    onChange={(event) => setForm((current) => ({ ...current, sourceTask: event.target.value }))}
                    placeholder="Implement the next Spec 24 slice"
                    className="min-h-28 resize-y border border-border bg-background px-3 py-2 text-sm text-foreground outline-none focus:border-primary"
                  />
                </label>
                <label className="grid gap-1 text-xs">
                  <span className="font-medium uppercase tracking-widest text-muted-foreground">Source Specs</span>
                  <textarea
                    value={form.sourceSpecsText}
                    onChange={(event) => setForm((current) => ({ ...current, sourceSpecsText: event.target.value }))}
                    placeholder="docs/specs/24-electron-desktop-app.md"
                    className="min-h-20 resize-y border border-border bg-background px-3 py-2 font-mono text-xs text-foreground outline-none focus:border-primary"
                  />
                </label>
              </div>
            </section>

            <section className="border border-border">
              <SectionHeader title="Roles" />
              <div className="grid gap-3 p-3">
                {form.mode === "one_step_chain" && (
                  <label className="grid gap-1 text-xs">
                    <span className="font-medium uppercase tracking-widest text-muted-foreground">Selected Role</span>
                    <select
                      value={form.role}
                      onChange={(event) => setForm((current) => ({ ...current, role: event.target.value }))}
                      className="border border-border bg-background px-3 py-2 text-sm text-foreground outline-none focus:border-primary"
                    >
                      {rolesList.map((role) => (
                        <option key={role} value={role}>
                          {role}
                        </option>
                      ))}
                    </select>
                  </label>
                )}

                {form.mode === "manual_roster" && (
                  <>
                    <div className="grid gap-2 md:grid-cols-[minmax(0,1fr)_auto]">
                      <label className="grid gap-1 text-xs">
                        <span className="font-medium uppercase tracking-widest text-muted-foreground">Roster Role</span>
                        <select
                          value={form.rosterCandidate}
                          onChange={(event) => (
                            setForm((current) => ({ ...current, rosterCandidate: event.target.value }))
                          )}
                          className="border border-border bg-background px-3 py-2 text-sm text-foreground outline-none focus:border-primary"
                        >
                          {rolesList.map((role) => (
                            <option key={role} value={role}>
                              {role}
                            </option>
                          ))}
                        </select>
                      </label>
                      <button
                        type="button"
                        onClick={addRosterRole}
                        className="inline-flex items-center justify-center gap-2 border border-border px-3 py-2 text-xs font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary md:self-end"
                      >
                        <Plus size={14} aria-hidden="true" />
                        Add
                      </button>
                    </div>
                    <RolePills roles={form.roster} onRemove={removeRosterRole} emptyText="No roster roles selected." />
                  </>
                )}

                {form.mode === "constrained_orchestration" && (
                  <div className="grid gap-2 md:grid-cols-2">
                    {rolesList.map((role) => (
                      <label
                        key={role}
                        className="flex items-center gap-2 border border-border px-3 py-2 text-xs text-foreground"
                      >
                        <input
                          type="checkbox"
                          checked={form.allowedRoles.includes(role)}
                          onChange={() => toggleAllowedRole(role)}
                          className="h-4 w-4 accent-primary"
                        />
                        <span className="min-w-0 truncate font-mono">{role}</span>
                      </label>
                    ))}
                  </div>
                )}

                {form.mode === "sir_topham_decides" && (
                  <p className="text-xs text-muted-foreground">orchestrator</p>
                )}
              </div>
            </section>

            <section className="border border-border">
              <SectionHeader title="Limits" />
              <div className="grid gap-3 p-3 md:grid-cols-3">
                <TextInput label="Max Steps" value={form.maxSteps} onChange={(value) => (
                  setForm((current) => ({ ...current, maxSteps: value }))
                )} />
                <TextInput label="Resolver Loops" value={form.maxResolverLoops} onChange={(value) => (
                  setForm((current) => ({ ...current, maxResolverLoops: value }))
                )} />
                <TextInput label="Max Duration" value={form.maxDuration} onChange={(value) => (
                  setForm((current) => ({ ...current, maxDuration: value }))
                )} placeholder="10m" />
                <TextInput label="Token Budget" value={form.tokenBudget} onChange={(value) => (
                  setForm((current) => ({ ...current, tokenBudget: value }))
                )} />
                <TextInput label="Step Turns" value={form.stepMaxTurns} onChange={(value) => (
                  setForm((current) => ({ ...current, stepMaxTurns: value }))
                )} />
                <TextInput label="Step Tokens" value={form.stepMaxTokens} onChange={(value) => (
                  setForm((current) => ({ ...current, stepMaxTokens: value }))
                )} />
                <label className="flex items-center gap-2 border border-border px-3 py-2 text-xs text-foreground md:col-span-3">
                  <input
                    type="checkbox"
                    checked={form.allowApprovalWait}
                    onChange={(event) => (
                      setForm((current) => ({ ...current, allowApprovalWait: event.target.checked }))
                    )}
                    className="h-4 w-4 accent-primary"
                  />
                  <span className="font-medium uppercase tracking-widest text-muted-foreground">
                    Allow Approval Wait
                  </span>
                </label>
              </div>
            </section>

            <div className="flex flex-wrap gap-2">
              <button
                type="submit"
                disabled={previewLoading}
                className="inline-flex items-center gap-2 border border-primary px-3 py-2 text-xs font-medium uppercase tracking-widest text-primary hover:bg-primary/10 disabled:cursor-not-allowed disabled:opacity-60"
              >
                <Eye size={15} aria-hidden="true" />
                {previewLoading ? "Previewing" : "Preview"}
              </button>
              <button
                type="button"
                onClick={resetForm}
                className="inline-flex items-center gap-2 border border-border px-3 py-2 text-xs font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary"
              >
                <RefreshCw size={15} aria-hidden="true" />
                Reset
              </button>
              <button
                type="button"
                onClick={handleStart}
                disabled={startLoading || previewLoading}
                className="inline-flex items-center gap-2 border border-border px-3 py-2 text-xs font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
              >
                <Rocket size={15} aria-hidden="true" />
                {startLoading ? "Starting" : "Start"}
              </button>
            </div>
          </div>

          <aside className="space-y-5">
            <section className="border border-border">
              <SectionHeader title="Templates" />
              <div className="divide-y divide-border/70">
                {templates.map((template) => (
                  <button
                    key={template.id}
                    type="button"
                    onClick={() => applyTemplate(template)}
                    className="flex w-full items-center justify-between gap-3 px-3 py-3 text-left text-xs hover:bg-muted/40"
                  >
                    <span>
                      <span className="block font-medium text-foreground">{template.label}</span>
                      <span className="mt-1 block font-mono text-[11px] text-muted-foreground">{template.id}</span>
                    </span>
                    {form.templateId === template.id && <Check size={15} className="text-primary" aria-hidden="true" />}
                  </button>
                ))}
                {templates.length === 0 && <EmptyLine text="No templates loaded." />}
              </div>
            </section>

            <section className="border border-border">
              <SectionHeader title="Custom Presets" />
              <div className="divide-y divide-border/70">
                {presets.map((preset) => (
                  <button
                    key={preset.id}
                    type="button"
                    onClick={() => applyPreset(preset)}
                    className="block w-full px-3 py-3 text-left text-xs hover:bg-muted/40"
                  >
                    <span className="block font-medium text-foreground">{preset.name}</span>
                    <span className="mt-1 block font-mono text-[11px] text-muted-foreground">
                      {preset.request.mode}
                    </span>
                  </button>
                ))}
                {presets.length === 0 && <EmptyLine text="No custom presets." />}
              </div>
            </section>

            <section className="border border-border">
              <SectionHeader title="Request" />
              <pre className="max-h-56 overflow-auto whitespace-pre-wrap break-words px-3 py-3 font-mono text-[11px] text-muted-foreground">
                {JSON.stringify(currentRequest, null, 2)}
              </pre>
            </section>

            <section className="border border-border">
              <SectionHeader title="Preview" />
              {previewError && (
                <div className="border-b border-destructive/40 px-3 py-2 text-xs text-destructive">
                  {previewError}
                </div>
              )}
              {startError && (
                <div className="border-b border-destructive/40 px-3 py-2 text-xs text-destructive">
                  {startError}
                </div>
              )}
              {!preview && !previewError && !startError && <EmptyLine text="No preview generated." />}
              {preview && (
                <div className="grid gap-3 px-3 py-3 text-xs">
                  <div>
                    <div className="font-medium text-foreground">{preview.summary}</div>
                    <div className="mt-1 font-mono text-[11px] text-muted-foreground">
                      {preview.mode}
                      {preview.role ? ` / ${preview.role}` : ""}
                    </div>
                  </div>
                  {preview.warnings.length > 0 && (
                    <div className="border border-warning/50 bg-warning/5 p-2 text-warning">
                      {preview.warnings.map((warning) => (
                        <p key={warning.message}>{warning.message}</p>
                      ))}
                    </div>
                  )}
                  <pre className="max-h-[32rem] overflow-auto whitespace-pre-wrap break-words border border-border bg-background p-3 font-mono text-[11px] text-foreground">
                    {preview.work_packet_markdown || preview.compiled_task}
                  </pre>
                </div>
              )}
            </section>
          </aside>
        </form>
      </div>
    </div>
  );
}

function SectionHeader({ title }: { title: string }) {
  return (
    <div className="border-b border-border bg-muted px-3 py-2">
      <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">{title}</h2>
    </div>
  );
}

function EmptyLine({ text }: { text: string }) {
  return <p className="px-3 py-3 text-xs text-muted-foreground">{text}</p>;
}

function StatusBanner({ children, tone }: { children: ReactNode; tone: "danger" | "warning" }) {
  const toneClass = tone === "danger" ? "border-destructive/50 text-destructive" : "border-warning/50 text-warning";
  return <div className={`border bg-background px-3 py-2 text-xs ${toneClass}`}>{children}</div>;
}

function TextInput({
  label,
  value,
  onChange,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
}) {
  return (
    <label className="grid gap-1 text-xs">
      <span className="font-medium uppercase tracking-widest text-muted-foreground">{label}</span>
      <input
        type={label === "Max Duration" ? "text" : "number"}
        min={label === "Resolver Loops" ? 0 : 1}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        className="border border-border bg-background px-3 py-2 text-sm text-foreground outline-none focus:border-primary"
      />
    </label>
  );
}

function RolePills({
  roles,
  onRemove,
  emptyText,
}: {
  roles: string[];
  onRemove: (role: string) => void;
  emptyText: string;
}) {
  if (roles.length === 0) return <p className="text-xs text-muted-foreground">{emptyText}</p>;
  return (
    <div className="flex flex-wrap gap-2">
      {roles.map((role) => (
        <span
          key={role}
          className="inline-flex items-center gap-2 border border-border px-2 py-1 font-mono text-xs text-foreground"
        >
          {role}
          <button
            type="button"
            onClick={() => onRemove(role)}
            className="text-muted-foreground hover:text-destructive"
            aria-label={`Remove ${role} from roster`}
          >
            <X size={13} aria-hidden="true" />
          </button>
        </span>
      ))}
    </div>
  );
}
