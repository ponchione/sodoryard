import { useEffect, useMemo, useRef, useState } from "react";
import type { DragEvent, FormEvent, ReactNode } from "react";
import {
  AlertTriangle,
  ArrowLeft,
  ArrowRight,
  Copy,
  Eye,
  FileText,
  Plus,
  RefreshCw,
  Rocket,
  Save,
  Search,
  Trash2,
  X,
} from "lucide-react";
import { useNavigate, useSearchParams } from "react-router-dom";
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
  LaunchRosterStep,
  LaunchStartResponse,
  LaunchTemplate,
  RuntimeStatus,
} from "@/types/chains";

interface ProjectTreeNode {
  name: string;
  type: "dir" | "file";
  children?: ProjectTreeNode[];
}

interface ProjectFileOption {
  name: string;
  path: string;
}

interface ValidatePathsResponse {
  accepted: string[];
  rejected: Array<{ path: string; reason: string }>;
}

interface ComposerNode {
  id: string;
  role: string;
  note: string;
  sources: string[];
}

interface ComposerState {
  sourceTask: string;
  sourceSpecs: string[];
  nodes: ComposerNode[];
  allowedRoles: string[];
  dispatchMode: "none" | "free" | "constrained";
}

const emptyComposer: ComposerState = {
  sourceTask: "",
  sourceSpecs: [],
  nodes: [],
  allowedRoles: [],
  dispatchMode: "none",
};

function blankComposer(): ComposerState {
  return {
    sourceTask: "",
    sourceSpecs: [],
    nodes: [],
    allowedRoles: [],
    dispatchMode: "none",
  };
}

let nextNodeID = 0;

function newNodeID(): string {
  nextNodeID += 1;
  return `node-${nextNodeID}`;
}

function compactList(values: string[]): string[] {
  const out: string[] = [];
  for (const value of values) {
    const trimmed = value.trim();
    if (trimmed) out.push(trimmed);
  }
  return out;
}

function splitList(value: string): string[] {
  return compactList(value.split(/[\n,]/));
}

function unique(values: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const value of values) {
    const trimmed = value.trim();
    if (!trimmed || seen.has(trimmed)) continue;
    seen.add(trimmed);
    out.push(trimmed);
  }
  return out;
}

function roleNames(roles: AgentRoleSummary[]): string[] {
  return roles.map((role) => role.name).filter(Boolean);
}

function flattenProjectFiles(node: ProjectTreeNode | null, parent = ""): ProjectFileOption[] {
  if (!node) return [];
  const currentPath = node.name === "." ? parent : (parent ? `${parent}/${node.name}` : node.name);
  if (node.type === "file") return [{ name: node.name, path: currentPath }];
  return (node.children ?? []).flatMap((child) => flattenProjectFiles(child, currentPath));
}

function attachmentErrorMessage(result: ValidatePathsResponse): string {
  return result.rejected.map((item) => `${item.path}: ${item.reason}`).join("; ");
}

function templateForMode(templates: LaunchTemplate[], mode: LaunchMode): LaunchTemplate | undefined {
  return templates.find((template) => template.mode === mode);
}

function stepsFromRequest(request: Partial<LaunchRequest> | undefined): LaunchRosterStep[] {
  if (!request) return [];
  if (request.steps && request.steps.length > 0) return request.steps;
  if (request.roster && request.roster.length > 0) return request.roster.map((role) => ({ role }));
  if (request.mode === "one_step_chain" && request.role) return [{ role: request.role }];
  return [];
}

function composerFromRequest(request: Partial<LaunchRequest> | undefined): ComposerState {
  if (!request) return blankComposer();
  const dispatchMode = request.mode === "sir_topham_decides"
    ? "free"
    : request.mode === "constrained_orchestration"
      ? "constrained"
      : "none";
  return {
    sourceTask: request.source_task ?? "",
    sourceSpecs: unique(request.source_specs ?? []),
    nodes: stepsFromRequest(request).map((step) => ({
      id: newNodeID(),
      role: step.role,
      note: step.note ?? "",
      sources: unique(step.sources ?? []),
    })),
    allowedRoles: unique(request.allowed_roles ?? []),
    dispatchMode,
  };
}

function stepsFromNodes(nodes: ComposerNode[]): LaunchRosterStep[] {
  return nodes.map((node) => {
    const step: LaunchRosterStep = { role: node.role };
    const note = node.note.trim();
    const sources = unique(node.sources);
    if (note) step.note = note;
    if (sources.length > 0) step.sources = sources;
    return step;
  });
}

function buildLaunchRequest(state: ComposerState, templates: LaunchTemplate[]): LaunchRequest {
  const sourceTask = state.sourceTask.trim();
  const sourceSpecs = unique(state.sourceSpecs);
  const base = {
    source_task: sourceTask || undefined,
    source_specs: sourceSpecs.length > 0 ? sourceSpecs : undefined,
  };

  if (state.dispatchMode === "free") {
    return {
      ...base,
      template_id: templateForMode(templates, "sir_topham_decides")?.id,
      mode: "sir_topham_decides",
      role: "orchestrator",
    };
  }

  if (state.dispatchMode === "constrained") {
    return {
      ...base,
      template_id: templateForMode(templates, "constrained_orchestration")?.id,
      mode: "constrained_orchestration",
      role: "orchestrator",
      allowed_roles: state.allowedRoles.length > 0 ? state.allowedRoles : undefined,
    };
  }

  const steps = stepsFromNodes(state.nodes);
  if (steps.length === 1) {
    return {
      ...base,
      template_id: templateForMode(templates, "one_step_chain")?.id,
      mode: "one_step_chain",
      role: steps[0].role,
      steps,
    };
  }

  return {
    ...base,
    template_id: templateForMode(templates, "manual_roster")?.id,
    mode: "manual_roster",
    roster: steps.length > 0 ? steps.map((step) => step.role) : undefined,
    steps: steps.length > 0 ? steps : undefined,
  };
}

function errorMessage(error: unknown): string {
  if (error instanceof ApiError && error.body.trim()) return error.body.trim();
  return error instanceof Error ? error.message : "Request failed";
}

function validationReasons(state: ComposerState): string[] {
  const reasons: string[] = [];
  const hasWorkPacket = state.sourceTask.trim() !== "" || state.sourceSpecs.length > 0;
  if (!hasWorkPacket) {
    reasons.push("Work packet needs a task or global source.");
  }
  if (state.dispatchMode === "none" && state.nodes.length === 0) {
    reasons.push("Add at least one agent node.");
  }
  if (state.dispatchMode === "constrained" && state.allowedRoles.length === 0) {
    reasons.push("Select at least one allowed role.");
  }
  return reasons;
}

function sourceCountLabel(count: number): string {
  if (count === 0) return "0 sources";
  return count === 1 ? "1 source" : `${count} sources`;
}

export function LaunchPage() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { data: runtime, error: runtimeError } = useApiResource<RuntimeStatus | null>("/api/runtime/status", null);
  const { data: roles, loading: rolesLoading, error: rolesError } = useApiResource<AgentRoleSummary[]>("/api/roles", []);
  const { data: templates, loading: templatesLoading, error: templatesError } = (
    useApiResource<LaunchTemplate[]>("/api/chains/templates", [])
  );
  const { data: draftRead, loading: draftLoading, error: draftError } = (
    useApiResource<LaunchDraftRead>("/api/launch/draft", { found: false })
  );
  const { data: presets, error: presetsError, refresh: refreshPresets } = (
    useApiResource<LaunchPreset[]>("/api/launch/presets", [])
  );
  const { data: projectTree, loading: projectTreeLoading, error: projectTreeError } = (
    useApiResource<ProjectTreeNode | null>("/api/project/tree?depth=4", null)
  );

  const rolesList = useMemo(() => roleNames(roles), [roles]);
  const workerRoles = useMemo(() => rolesList.filter((role) => role !== "orchestrator"), [rolesList]);
  const projectFiles = useMemo(() => flattenProjectFiles(projectTree), [projectTree]);
  const sourceSpecParams = useMemo(() => unique(searchParams.getAll("source_spec").flatMap(splitList)), [searchParams]);
  const [composer, setComposer] = useState<ComposerState>(emptyComposer);
  const [selectedID, setSelectedID] = useState("work-packet");
  const [preview, setPreview] = useState<LaunchPreview | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const [startLoading, setStartLoading] = useState(false);
  const [startError, setStartError] = useState<string | null>(null);
  const [attachmentQuery, setAttachmentQuery] = useState("");
  const [attachmentLoading, setAttachmentLoading] = useState<string | null>(null);
  const [attachmentError, setAttachmentError] = useState<string | null>(null);
  const [templateName, setTemplateName] = useState("");
  const [templateStatus, setTemplateStatus] = useState<string | null>(null);
  const initialized = useRef(false);

  useEffect(() => {
    if (initialized.current || rolesLoading || templatesLoading || draftLoading) return;
    const nextComposer = composerFromRequest(draftRead.found ? draftRead.draft?.request : undefined);
    if (sourceSpecParams.length > 0) {
      nextComposer.sourceSpecs = unique([...nextComposer.sourceSpecs, ...sourceSpecParams]);
    }
    setComposer(nextComposer);
    setSelectedID(nextComposer.nodes[0]?.id ?? "work-packet");
    initialized.current = true;
  }, [draftLoading, draftRead, rolesLoading, sourceSpecParams, templatesLoading]);

  const currentRequest = useMemo(() => buildLaunchRequest(composer, templates), [composer, templates]);
  const selectedNode = composer.nodes.find((node) => node.id === selectedID) ?? null;
  const reasons = validationReasons(composer);
  const canSubmit = reasons.length === 0 && !previewLoading && !startLoading;
  const normalizedAttachmentQuery = attachmentQuery.trim().toLowerCase();
  const visibleProjectFiles = useMemo(() => {
    const filtered = normalizedAttachmentQuery
      ? projectFiles.filter((file) => file.path.toLowerCase().includes(normalizedAttachmentQuery))
      : projectFiles;
    return filtered.slice(0, 24);
  }, [normalizedAttachmentQuery, projectFiles]);

  const clearResults = () => {
    setPreview(null);
    setPreviewError(null);
    setStartError(null);
    setTemplateStatus(null);
  };

  const updateComposer = (updater: (current: ComposerState) => ComposerState) => {
    setComposer((current) => updater(current));
    clearResults();
  };

  const addNode = (role: string, afterIndex?: number) => {
    if (!role) return;
    const node: ComposerNode = { id: newNodeID(), role, note: "", sources: [] };
    updateComposer((current) => {
      const nodes = [...current.nodes];
      if (afterIndex === undefined) nodes.push(node);
      else nodes.splice(afterIndex + 1, 0, node);
      return { ...current, nodes, dispatchMode: "none" };
    });
    setSelectedID(node.id);
  };

  const duplicateNode = (node: ComposerNode) => {
    const index = composer.nodes.findIndex((candidate) => candidate.id === node.id);
    const copy: ComposerNode = {
      id: newNodeID(),
      role: node.role,
      note: node.note,
      sources: [...node.sources],
    };
    updateComposer((current) => {
      const nodes = [...current.nodes];
      nodes.splice(index + 1, 0, copy);
      return { ...current, nodes, dispatchMode: "none" };
    });
    setSelectedID(copy.id);
  };

  const removeNode = (nodeID: string) => {
    updateComposer((current) => {
      const index = current.nodes.findIndex((node) => node.id === nodeID);
      const nodes = current.nodes.filter((node) => node.id !== nodeID);
      if (nodeID === selectedID) setSelectedID(nodes[index]?.id ?? nodes[index - 1]?.id ?? "work-packet");
      return { ...current, nodes };
    });
  };

  const moveNode = (nodeID: string, delta: -1 | 1) => {
    updateComposer((current) => {
      const index = current.nodes.findIndex((node) => node.id === nodeID);
      const target = index + delta;
      if (index < 0 || target < 0 || target >= current.nodes.length) return current;
      const nodes = [...current.nodes];
      const [node] = nodes.splice(index, 1);
      nodes.splice(target, 0, node);
      return { ...current, nodes };
    });
  };

  const updateNode = (nodeID: string, patch: Partial<ComposerNode>) => {
    updateComposer((current) => ({
      ...current,
      nodes: current.nodes.map((node) => (node.id === nodeID ? { ...node, ...patch } : node)),
      dispatchMode: "none",
    }));
  };

  const applyPreset = (preset: LaunchPreset) => {
    const templateComposer = composerFromRequest(preset.request);
    setComposer((current) => ({
      ...templateComposer,
      sourceTask: current.sourceTask,
      sourceSpecs: current.sourceSpecs,
    }));
    setSelectedID(templateComposer.nodes[0]?.id ?? "work-packet");
    clearResults();
  };

  const addProjectAttachment = async (path: string, target: "work-packet" | string) => {
    setAttachmentLoading(`${target}:${path}`);
    setAttachmentError(null);
    try {
      const result = await api.post<ValidatePathsResponse>("/api/project/validate-paths", {
        purpose: "launch_attachment",
        paths: [path],
      });
      if (result.rejected.length > 0) {
        setAttachmentError(attachmentErrorMessage(result));
        return;
      }
      updateComposer((current) => {
        if (target === "work-packet") {
          return { ...current, sourceSpecs: unique([...current.sourceSpecs, ...result.accepted]) };
        }
        return {
          ...current,
          nodes: current.nodes.map((node) => (
            node.id === target ? { ...node, sources: unique([...node.sources, ...result.accepted]) } : node
          )),
          dispatchMode: "none",
        };
      });
    } catch (error) {
      setAttachmentError(errorMessage(error));
    } finally {
      setAttachmentLoading(null);
    }
  };

  const removeSource = (path: string, target: "work-packet" | string) => {
    updateComposer((current) => {
      if (target === "work-packet") {
        return { ...current, sourceSpecs: current.sourceSpecs.filter((candidate) => candidate !== path) };
      }
      return {
        ...current,
        nodes: current.nodes.map((node) => (
          node.id === target ? { ...node, sources: node.sources.filter((candidate) => candidate !== path) } : node
        )),
      };
    });
  };

  const handleDropSource = (event: DragEvent<HTMLElement>, target: "work-packet" | string) => {
    event.preventDefault();
    const path = event.dataTransfer.getData("text/plain");
    if (path) void addProjectAttachment(path, target);
  };

  const saveTemplate = async () => {
    const name = templateName.trim();
    if (!name) {
      setTemplateStatus("Template name is required.");
      return;
    }
    setTemplateStatus(null);
    try {
      await api.post<LaunchPreset>("/api/launch/presets", { name, request: currentRequest });
      setTemplateName("");
      setTemplateStatus("Saved.");
      await refreshPresets();
    } catch (error) {
      setTemplateStatus(errorMessage(error));
    }
  };

  const resetComposer = () => {
    setComposer(blankComposer());
    setSelectedID("work-packet");
    clearResults();
  };

  const handlePreview = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (reasons.length > 0) return;
    setPreviewLoading(true);
    setPreviewError(null);
    setStartError(null);
    try {
      setPreview(await api.post<LaunchPreview>("/api/launch/preview", currentRequest));
    } catch (error) {
      setPreviewError(errorMessage(error));
    } finally {
      setPreviewLoading(false);
    }
  };

  const handleStart = async () => {
    if (reasons.length > 0) return;
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

  const pageErrors = [runtimeError, rolesError, templatesError, draftError, presetsError, projectTreeError].filter(Boolean);

  return (
    <div className="flex-1 overflow-y-auto px-4 py-6">
      <div className="flex w-full flex-col gap-5">
        <header className="flex flex-col gap-3 border-b border-border pb-4 md:flex-row md:items-end md:justify-between">
          <div className="min-w-0">
            <h1 className="text-xl font-semibold uppercase tracking-widest text-primary text-glow-cyan">
              Launch Composer
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
            <span className="border border-border px-2 py-1">{composer.nodes.length} nodes</span>
            <span className="border border-border px-2 py-1">{sourceCountLabel(composer.sourceSpecs.length)}</span>
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

        <form
          onSubmit={handlePreview}
          className="grid gap-5 xl:grid-cols-[260px_minmax(0,1fr)_360px]"
        >
          <aside className="space-y-5">
            <section className="border border-border">
              <SectionHeader title="Role Palette" />
              <div className="grid gap-2 p-3">
                {workerRoles.map((role) => (
                  <button
                    key={role}
                    type="button"
                    onClick={() => addNode(role)}
                    className="inline-flex items-center justify-between gap-2 border border-border px-3 py-2 text-left text-xs font-medium text-foreground hover:border-primary hover:text-primary"
                  >
                    <span className="min-w-0 truncate font-mono">{role}</span>
                    <Plus size={14} aria-hidden="true" />
                  </button>
                ))}
                {workerRoles.length === 0 && <EmptyLine text="No worker roles loaded." />}
              </div>
            </section>

            <section className="border border-border">
              <SectionHeader title="Saved Templates" />
              <div className="divide-y divide-border/70">
                {presets.map((preset) => (
                  <button
                    key={preset.id}
                    type="button"
                    onClick={() => applyPreset(preset)}
                    className="block w-full p-3 text-left text-xs hover:bg-muted/40"
                  >
                    <span className="block font-medium text-foreground">{preset.name}</span>
                    <span className="mt-1 block font-mono text-[11px] text-muted-foreground">
                      {preset.request.steps?.map((step) => step.role).join(" -> ") || preset.request.mode}
                    </span>
                  </button>
                ))}
                {presets.length === 0 && <EmptyLine text="No saved templates." />}
              </div>
              <div className="grid gap-2 border-t border-border p-3">
                <label className="grid gap-1 text-xs">
                  <span className="font-medium uppercase tracking-widest text-muted-foreground">Template Name</span>
                  <input
                    value={templateName}
                    onChange={(event) => setTemplateName(event.target.value)}
                    className="border border-border bg-background px-3 py-2 text-sm text-foreground outline-none focus:border-primary"
                  />
                </label>
                {templateStatus && <p className="text-xs text-muted-foreground">{templateStatus}</p>}
                <button
                  type="button"
                  onClick={() => void saveTemplate()}
                  className="inline-flex items-center justify-center gap-2 border border-border px-3 py-2 text-xs font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary"
                >
                  <Save size={14} aria-hidden="true" />
                  Save
                </button>
              </div>
            </section>
          </aside>

          <main className="space-y-5">
            <section
              className="border border-border"
              onDragOver={(event) => event.preventDefault()}
              onDrop={(event) => handleDropSource(event, "work-packet")}
            >
              <SectionHeader title="Work Packet" />
              <div className="grid gap-3 p-3">
                <label className="grid gap-1 text-xs">
                  <span className="font-medium uppercase tracking-widest text-muted-foreground">Task</span>
                  <textarea
                    value={composer.sourceTask}
                    onChange={(event) => updateComposer((current) => ({ ...current, sourceTask: event.target.value }))}
                    className="min-h-28 resize-y border border-border bg-background px-3 py-2 text-sm text-foreground outline-none focus:border-primary"
                  />
                </label>
                <label className="grid gap-1 text-xs">
                  <span className="font-medium uppercase tracking-widest text-muted-foreground">Project Sources</span>
                  <textarea
                    value={composer.sourceSpecs.join("\n")}
                    onChange={(event) => (
                      updateComposer((current) => ({ ...current, sourceSpecs: unique(splitList(event.target.value)) }))
                    )}
                    className="min-h-20 resize-y border border-border bg-background px-3 py-2 font-mono text-xs text-foreground outline-none focus:border-primary"
                  />
                </label>
                <AttachmentPills paths={composer.sourceSpecs} target="work-packet" onRemove={removeSource} />
              </div>
            </section>

            <section className="border border-border">
              <SectionHeader title="Composer Canvas" />
              <div className="grid gap-3 p-3">
                <div className="flex flex-wrap items-center gap-2">
                  <button
                    type="button"
                    aria-pressed={selectedID === "work-packet"}
                    onClick={() => setSelectedID("work-packet")}
                    className={`border px-3 py-2 text-left text-xs font-medium uppercase tracking-widest ${
                      selectedID === "work-packet" ? "border-primary text-primary" : "border-border text-foreground"
                    }`}
                  >
                    Work Packet
                  </button>
                  {composer.nodes.map((node, index) => (
                    <div key={node.id} className="flex items-center gap-2">
                      <ArrowRight size={14} className="text-muted-foreground" aria-hidden="true" />
                      <button
                        type="button"
                        aria-pressed={selectedID === node.id}
                        onClick={() => setSelectedID(node.id)}
                        onDragOver={(event) => event.preventDefault()}
                        onDrop={(event) => handleDropSource(event, node.id)}
                        className={`min-w-40 border px-3 py-2 text-left text-xs ${
                          selectedID === node.id
                            ? "border-primary text-primary"
                            : "border-border text-foreground hover:border-primary"
                        }`}
                      >
                        <span className="block truncate font-mono">{node.role}</span>
                        <span className="mt-1 block text-[10px] uppercase tracking-widest text-muted-foreground">
                          {sourceCountLabel(node.sources.length)}
                          {node.note.trim() ? " / note" : ""}
                        </span>
                      </button>
                      {index < composer.nodes.length - 1 && null}
                    </div>
                  ))}
                </div>
                {composer.nodes.length === 0 && <p className="text-xs text-muted-foreground">No agent nodes.</p>}
                <div className="flex flex-wrap gap-2 border-t border-border pt-3">
                  <button
                    type="button"
                    aria-pressed={composer.dispatchMode === "free"}
                    onClick={() => updateComposer((current) => ({ ...current, dispatchMode: current.dispatchMode === "free" ? "none" : "free" }))}
                    className="border border-border px-3 py-2 text-xs font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary"
                  >
                    Dispatcher
                  </button>
                  <button
                    type="button"
                    aria-pressed={composer.dispatchMode === "constrained"}
                    onClick={() => updateComposer((current) => ({ ...current, dispatchMode: current.dispatchMode === "constrained" ? "none" : "constrained" }))}
                    className="border border-border px-3 py-2 text-xs font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary"
                  >
                    Constrained
                  </button>
                </div>
              </div>
            </section>

            {composer.dispatchMode === "constrained" && (
              <section className="border border-border">
                <SectionHeader title="Allowed Roles" />
                <div className="grid gap-2 p-3 md:grid-cols-2">
                  {workerRoles.map((role) => (
                    <label
                      key={role}
                      className="flex items-center gap-2 border border-border px-3 py-2 text-xs text-foreground"
                    >
                      <input
                        type="checkbox"
                        checked={composer.allowedRoles.includes(role)}
                        onChange={() => updateComposer((current) => {
                          const selected = current.allowedRoles.includes(role);
                          return {
                            ...current,
                            allowedRoles: selected
                              ? current.allowedRoles.filter((item) => item !== role)
                              : unique([...current.allowedRoles, role]),
                          };
                        })}
                        className="size-4 accent-primary"
                      />
                      <span className="min-w-0 truncate font-mono">{role}</span>
                    </label>
                  ))}
                </div>
              </section>
            )}

            <section className="border border-border">
              <SectionHeader title="Project Attachments" />
              <div className="grid gap-3 p-3">
                <label className="grid gap-1 text-xs">
                  <span className="font-medium uppercase tracking-widest text-muted-foreground">Find Files</span>
                  <span className="relative">
                    <Search
                      size={14}
                      aria-hidden="true"
                      className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-muted-foreground/70"
                    />
                    <input
                      type="search"
                      value={attachmentQuery}
                      onChange={(event) => setAttachmentQuery(event.target.value)}
                      className="w-full border border-border bg-background px-8 py-2 text-xs text-foreground outline-none focus:border-primary"
                    />
                  </span>
                </label>
                {attachmentError && <p className="text-xs text-destructive">{attachmentError}</p>}
                {projectTreeLoading && <p className="text-xs text-muted-foreground">Loading project files...</p>}
                {!projectTreeLoading && visibleProjectFiles.length === 0 && (
                  <p className="text-xs text-muted-foreground">No project files match.</p>
                )}
                <div className="max-h-72 divide-y divide-border/70 overflow-auto border border-border">
                  {visibleProjectFiles.map((file) => {
                    const workAttached = composer.sourceSpecs.includes(file.path);
                    const nodeAttached = selectedNode?.sources.includes(file.path) ?? false;
                    return (
                      <div
                        key={file.path}
                        draggable
                        onDragStart={(event) => event.dataTransfer.setData("text/plain", file.path)}
                        className="grid gap-2 px-3 py-2 text-xs lg:grid-cols-[minmax(0,1fr)_auto_auto]"
                      >
                        <div className="flex min-w-0 items-center gap-2">
                          <FileText size={14} className="shrink-0 text-muted-foreground" aria-hidden="true" />
                          <span className="truncate font-mono text-muted-foreground">{file.path}</span>
                        </div>
                        <button
                          type="button"
                          onClick={() => void addProjectAttachment(file.path, "work-packet")}
                          disabled={workAttached || attachmentLoading === `work-packet:${file.path}`}
                          className="border border-border px-2 py-1 text-[10px] font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
                        >
                          {workAttached ? "Global" : "Work"}
                        </button>
                        <button
                          type="button"
                          onClick={() => selectedNode && void addProjectAttachment(file.path, selectedNode.id)}
                          disabled={!selectedNode || nodeAttached || attachmentLoading === `${selectedNode?.id}:${file.path}`}
                          className="border border-border px-2 py-1 text-[10px] font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
                        >
                          {nodeAttached ? "Dossier" : "Node"}
                        </button>
                      </div>
                    );
                  })}
                </div>
              </div>
            </section>
          </main>

          <aside className="space-y-5">
            <section className="border border-border">
              <SectionHeader title="Inspector" />
              {selectedNode ? (
                <NodeInspector
                  node={selectedNode}
                  index={composer.nodes.findIndex((node) => node.id === selectedNode.id)}
                  count={composer.nodes.length}
                  globalSources={composer.sourceSpecs}
                  onUpdate={updateNode}
                  onDuplicate={duplicateNode}
                  onRemove={removeNode}
                  onMove={moveNode}
                  onInsertAfter={(index) => addNode(selectedNode.role, index)}
                  onRemoveSource={removeSource}
                />
              ) : (
                <div className="grid gap-3 p-3 text-xs">
                  <div>
                    <div className="font-medium uppercase tracking-widest text-muted-foreground">Work Packet</div>
                    <div className="mt-1 text-foreground">{composer.sourceTask.trim() || "No task recorded"}</div>
                  </div>
                  <AttachmentPills paths={composer.sourceSpecs} target="work-packet" onRemove={removeSource} />
                </div>
              )}
            </section>

            <section className="border border-border">
              <SectionHeader title="Run Sheet" />
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
                <div className="grid gap-3 p-3 text-xs">
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
                    {preview.run_sheet_markdown || preview.work_packet_markdown || preview.compiled_task}
                  </pre>
                </div>
              )}
            </section>

            <details className="border border-border">
              <summary className="cursor-pointer border-b border-border bg-muted px-3 py-2 text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
                Request JSON
              </summary>
              <pre className="max-h-56 overflow-auto whitespace-pre-wrap break-words p-3 font-mono text-[11px] text-muted-foreground">
                {JSON.stringify(currentRequest, null, 2)}
              </pre>
            </details>

            <div className="grid gap-2">
              {reasons.length > 0 && (
                <div className="border border-warning/50 bg-warning/5 px-3 py-2 text-xs text-warning">
                  {reasons.map((reason) => <p key={reason}>{reason}</p>)}
                </div>
              )}
              <div className="flex flex-wrap gap-2">
                <button
                  type="submit"
                  disabled={!canSubmit}
                  className="inline-flex items-center gap-2 border border-primary px-3 py-2 text-xs font-medium uppercase tracking-widest text-primary hover:bg-primary/10 disabled:cursor-not-allowed disabled:opacity-60"
                >
                  <Eye size={15} aria-hidden="true" />
                  {previewLoading ? "Previewing" : "Preview"}
                </button>
                <button
                  type="button"
                  onClick={resetComposer}
                  className="inline-flex items-center gap-2 border border-border px-3 py-2 text-xs font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary"
                >
                  <RefreshCw size={15} aria-hidden="true" />
                  Reset
                </button>
                <button
                  type="button"
                  onClick={handleStart}
                  disabled={!canSubmit}
                  className="inline-flex items-center gap-2 border border-border px-3 py-2 text-xs font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
                >
                  <Rocket size={15} aria-hidden="true" />
                  {startLoading ? "Starting" : "Start"}
                </button>
              </div>
            </div>
          </aside>
        </form>
      </div>
    </div>
  );
}

function NodeInspector({
  node,
  index,
  count,
  globalSources,
  onUpdate,
  onDuplicate,
  onRemove,
  onMove,
  onInsertAfter,
  onRemoveSource,
}: {
  node: ComposerNode;
  index: number;
  count: number;
  globalSources: string[];
  onUpdate: (nodeID: string, patch: Partial<ComposerNode>) => void;
  onDuplicate: (node: ComposerNode) => void;
  onRemove: (nodeID: string) => void;
  onMove: (nodeID: string, delta: -1 | 1) => void;
  onInsertAfter: (index: number) => void;
  onRemoveSource: (path: string, target: "work-packet" | string) => void;
}) {
  const effectiveSources = unique([...globalSources, ...node.sources]);
  return (
    <div className="grid gap-3 p-3 text-xs">
      <div>
        <div className="font-medium uppercase tracking-widest text-muted-foreground">Role</div>
        <div className="mt-1 font-mono text-foreground">{node.role}</div>
      </div>
      <div className="flex flex-wrap gap-2">
        <IconButton label="Move left" disabled={index <= 0} onClick={() => onMove(node.id, -1)}>
          <ArrowLeft size={14} aria-hidden="true" />
        </IconButton>
        <IconButton label="Move right" disabled={index >= count - 1} onClick={() => onMove(node.id, 1)}>
          <ArrowRight size={14} aria-hidden="true" />
        </IconButton>
        <IconButton label="Duplicate" onClick={() => onDuplicate(node)}>
          <Copy size={14} aria-hidden="true" />
        </IconButton>
        <IconButton label="Insert after" onClick={() => onInsertAfter(index)}>
          <Plus size={14} aria-hidden="true" />
        </IconButton>
        <IconButton label="Remove" onClick={() => onRemove(node.id)}>
          <Trash2 size={14} aria-hidden="true" />
        </IconButton>
      </div>
      <label className="grid gap-1 text-xs">
        <span className="font-medium uppercase tracking-widest text-muted-foreground">Dossier Note</span>
        <textarea
          value={node.note}
          onChange={(event) => onUpdate(node.id, { note: event.target.value })}
          className="min-h-28 resize-y border border-border bg-background px-3 py-2 text-sm text-foreground outline-none focus:border-primary"
        />
      </label>
      <div>
        <div className="mb-2 font-medium uppercase tracking-widest text-muted-foreground">Dossier Sources</div>
        <AttachmentPills paths={node.sources} target={node.id} onRemove={onRemoveSource} />
      </div>
      <div className="border border-border p-2">
        <div className="font-medium uppercase tracking-widest text-muted-foreground">Effective Inputs</div>
        <div className="mt-2 grid gap-1 font-mono text-[11px] text-foreground">
          {effectiveSources.length === 0 && <p>None.</p>}
          {effectiveSources.map((source) => <p key={source}>{source}</p>)}
        </div>
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
  return <p className="p-3 text-xs text-muted-foreground">{text}</p>;
}

function StatusBanner({ children, tone }: { children: ReactNode; tone: "danger" | "warning" }) {
  const toneClass = tone === "danger" ? "border-destructive/50 text-destructive" : "border-warning/50 text-warning";
  return <div className={`border bg-background px-3 py-2 text-xs ${toneClass}`}>{children}</div>;
}

function IconButton({
  label,
  disabled,
  children,
  onClick,
}: {
  label: string;
  disabled?: boolean;
  children: ReactNode;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      disabled={disabled}
      onClick={onClick}
      className="inline-flex size-8 items-center justify-center border border-border text-muted-foreground hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-40"
    >
      {children}
    </button>
  );
}

function AttachmentPills({
  paths,
  target,
  onRemove,
}: {
  paths: string[];
  target: "work-packet" | string;
  onRemove: (path: string, target: "work-packet" | string) => void;
}) {
  if (paths.length === 0) {
    return <p className="text-xs text-muted-foreground">No project sources selected.</p>;
  }
  return (
    <div className="flex flex-wrap gap-2">
      {paths.map((path) => (
        <span
          key={path}
          className="inline-flex max-w-full items-center gap-2 border border-border px-2 py-1 font-mono text-xs text-foreground"
        >
          <span className="truncate">{path}</span>
          <button
            type="button"
            onClick={() => onRemove(path, target)}
            className="shrink-0 text-muted-foreground hover:text-destructive"
            aria-label={`Remove ${path}`}
          >
            <X size={13} aria-hidden="true" />
          </button>
        </span>
      ))}
    </div>
  );
}
