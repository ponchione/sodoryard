import { useMemo, useState } from "react";
import { AlertTriangle, ExternalLink, Eye, FileText, Folder, FolderOpen, Paperclip, Rocket, Search, X } from "lucide-react";
import { Link } from "react-router-dom";
import { useApiResource } from "@/hooks/use-api-resource";
import { api } from "@/lib/api";
import { getYardPlatform } from "@/platform";
import type { ProjectInfo } from "@/types/metrics";

interface ProjectTreeNode {
  name: string;
  type: "dir" | "file";
  children?: ProjectTreeNode[];
}

interface ProjectFileOption {
  name: string;
  path: string;
}

interface ProjectFileResponse {
  path: string;
  content: string;
  language: string;
  line_count: number;
}

interface ValidatePathsResponse {
  accepted: string[];
  rejected: Array<{ path: string; reason: string }>;
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

function flattenProjectFiles(node: ProjectTreeNode | null, parent = ""): ProjectFileOption[] {
  if (!node) return [];
  const currentPath = node.name === "." ? parent : (parent ? `${parent}/${node.name}` : node.name);
  if (node.type === "file") return [{ name: node.name, path: currentPath }];
  return (node.children ?? []).flatMap((child) => flattenProjectFiles(child, currentPath));
}

function childPath(parentPath: string, node: ProjectTreeNode): string {
  if (node.name === ".") return parentPath;
  return parentPath ? `${parentPath}/${node.name}` : node.name;
}

function launchURL(paths: string[]): string {
  const params = new URLSearchParams();
  paths.forEach((path) => params.append("source_spec", path));
  return `/launch?${params.toString()}`;
}

function validationMessage(result: ValidatePathsResponse): string {
  return result.rejected.map((item) => `${item.path}: ${item.reason}`).join("; ");
}

function formatProjectTitle(project: ProjectInfo | null): string {
  return project ? `${project.name} / ${project.root_path}` : "Project loading";
}

export function ProjectPage() {
  const platform = useMemo(() => getYardPlatform(), []);
  const { data: project, loading: projectLoading, error: projectError } = (
    useApiResource<ProjectInfo | null>("/api/project", null)
  );
  const { data: tree, loading: treeLoading, error: treeError } = (
    useApiResource<ProjectTreeNode | null>("/api/project/tree?depth=6", null)
  );
  const files = useMemo(() => flattenProjectFiles(tree), [tree]);
  const [query, setQuery] = useState("");
  const [preview, setPreview] = useState<ProjectFileResponse | null>(null);
  const [previewLoadingPath, setPreviewLoadingPath] = useState<string | null>(null);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const [attachments, setAttachments] = useState<string[]>([]);
  const [attachmentLoadingPath, setAttachmentLoadingPath] = useState<string | null>(null);
  const [attachmentError, setAttachmentError] = useState<string | null>(null);
  const [fileActionPath, setFileActionPath] = useState<string | null>(null);
  const [fileActionError, setFileActionError] = useState<string | null>(null);

  const normalizedQuery = query.trim().toLowerCase();
  const filteredFiles = useMemo(() => {
    if (!normalizedQuery) return files;
    return files.filter((file) => file.path.toLowerCase().includes(normalizedQuery));
  }, [files, normalizedQuery]);
  const visibleFiles = filteredFiles.slice(0, 80);

  const validateAttachments = async (paths: string[]): Promise<string[]> => {
    const requested = unique(paths.map((path) => path.trim()).filter(Boolean));
    if (requested.length === 0) return [];
    const result = await api.post<ValidatePathsResponse>("/api/project/validate-paths", {
      purpose: "launch_attachment",
      paths: requested,
    });
    if (result.rejected.length > 0) {
      setAttachmentError(validationMessage(result));
    }
    if (result.accepted.length > 0) {
      setAttachments((current) => unique([...current, ...result.accepted]));
    }
    return result.accepted;
  };

  const openFile = async (path: string) => {
    setPreviewLoadingPath(path);
    setPreviewError(null);
    try {
      const file = await api.get<ProjectFileResponse>(`/api/project/file?path=${encodeURIComponent(path)}`);
      setPreview(file);
    } catch (error) {
      setPreviewError(error instanceof Error ? error.message : "Failed to load file");
    } finally {
      setPreviewLoadingPath(null);
    }
  };

  const addAttachment = async (path: string) => {
    setAttachmentLoadingPath(path);
    setAttachmentError(null);
    try {
      await validateAttachments([path]);
    } catch (error) {
      setAttachmentError(error instanceof Error ? error.message : "Failed to attach file");
    } finally {
      setAttachmentLoadingPath(null);
    }
  };

  const validateProjectFileAction = async (path: string, purpose: "open_editor" | "reveal"): Promise<string> => {
    const result = await api.post<ValidatePathsResponse>("/api/project/validate-paths", {
      purpose,
      paths: [path],
    });
    if (result.accepted.length > 0) return result.accepted[0];
    const rejection = result.rejected[0];
    throw new Error(rejection ? `${rejection.path}: ${rejection.reason}` : "path was not accepted");
  };

  const openProjectPath = async (path: string) => {
    if (!platform.openProjectPath) return;
    setFileActionPath(`open:${path}`);
    setFileActionError(null);
    try {
      const acceptedPath = await validateProjectFileAction(path, "open_editor");
      await platform.openProjectPath(acceptedPath);
    } catch (error) {
      setFileActionError(error instanceof Error ? error.message : "Failed to open file");
    } finally {
      setFileActionPath(null);
    }
  };

  const revealProjectPath = async (path: string) => {
    if (!platform.revealProjectPath) return;
    setFileActionPath(`reveal:${path}`);
    setFileActionError(null);
    try {
      const acceptedPath = await validateProjectFileAction(path, "reveal");
      await platform.revealProjectPath(acceptedPath);
    } catch (error) {
      setFileActionError(error instanceof Error ? error.message : "Failed to reveal file");
    } finally {
      setFileActionPath(null);
    }
  };

  const addNativeAttachments = async () => {
    if (!platform.chooseProjectFiles) return;
    setAttachmentLoadingPath("__native__");
    setAttachmentError(null);
    try {
      const paths = await platform.chooseProjectFiles();
      if (paths.length === 0) {
        setAttachmentError("No project files selected.");
        return;
      }
      await validateAttachments(paths);
    } catch (error) {
      setAttachmentError(error instanceof Error ? error.message : "Failed to choose project files");
    } finally {
      setAttachmentLoadingPath(null);
    }
  };

  const removeAttachment = (path: string) => {
    setAttachments((current) => current.filter((candidate) => candidate !== path));
  };

  return (
    <div className="flex-1 overflow-y-auto px-4 py-6">
      <div className="flex w-full flex-col gap-5">
        <header className="flex flex-col gap-3 border-b border-border pb-4 md:flex-row md:items-end md:justify-between">
          <div className="min-w-0">
            <h1 className="text-xl font-bold uppercase tracking-widest text-primary text-glow-cyan">
              Project Browser
            </h1>
            <p className="mt-1 truncate text-xs text-muted-foreground">{formatProjectTitle(project)}</p>
          </div>
          {attachments.length > 0 ? (
            <Link
              to={launchURL(attachments)}
              className="inline-flex items-center gap-2 border border-primary px-3 py-2 text-xs font-medium uppercase tracking-widest text-primary hover:bg-primary/10"
            >
              <Rocket size={15} aria-hidden="true" />
              Launch with attachments
            </Link>
          ) : (
            <button
              type="button"
              disabled
              className="inline-flex cursor-not-allowed items-center gap-2 border border-border px-3 py-2 text-xs font-medium uppercase tracking-widest text-muted-foreground opacity-60"
            >
              <Rocket size={15} aria-hidden="true" />
              Launch with attachments
            </button>
          )}
        </header>

        {projectError && <StatusBanner tone="danger" text={projectError} />}
        {treeError && <StatusBanner tone="danger" text={treeError} />}
        {previewError && <StatusBanner tone="danger" text={previewError} />}
        {attachmentError && <StatusBanner tone="warning" text={attachmentError} />}
        {fileActionError && <StatusBanner tone="warning" text={fileActionError} />}

        <div className="grid gap-5 xl:grid-cols-[minmax(300px,0.85fr)_minmax(0,1.15fr)]">
          <section className="min-w-0 border border-border">
            <SectionHeader title="Files" detail={projectLoading || treeLoading ? "Loading" : `${files.length} files`} />
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
                    value={query}
                    onChange={(event) => setQuery(event.target.value)}
                    placeholder="docs/specs"
                    className="w-full border border-border bg-background px-8 py-2 text-xs text-foreground outline-none placeholder:text-muted-foreground/60 focus:border-primary"
                  />
                </span>
              </label>
              {platform.chooseProjectFiles && (
                <button
                  type="button"
                  onClick={() => void addNativeAttachments()}
                  disabled={attachmentLoadingPath === "__native__"}
                  className="inline-flex items-center justify-center gap-2 border border-border px-3 py-2 text-xs font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
                >
                  <Paperclip size={14} aria-hidden="true" />
                  {attachmentLoadingPath === "__native__" ? "Choosing Files" : "Choose Files"}
                </button>
              )}

              {treeLoading && <p className="text-xs text-muted-foreground">Loading project tree...</p>}
              {!treeLoading && files.length === 0 && <p className="text-xs text-muted-foreground">No project files found.</p>}
              {!treeLoading && files.length > 0 && normalizedQuery && visibleFiles.length === 0 && (
                <p className="text-xs text-muted-foreground">No files match.</p>
              )}

              <div className="max-h-[34rem] overflow-auto border border-border">
                {normalizedQuery ? (
                  <div className="divide-y divide-border/70">
                    {visibleFiles.map((file) => (
                      <FileRow
                        key={file.path}
                        file={file}
                        selected={preview?.path === file.path}
                        attached={attachments.includes(file.path)}
                        loading={previewLoadingPath === file.path || attachmentLoadingPath === file.path}
                        actionLoading={fileActionPath}
                        canOpenExternal={Boolean(platform.openProjectPath)}
                        canReveal={Boolean(platform.revealProjectPath)}
                        onOpen={openFile}
                        onOpenExternal={openProjectPath}
                        onReveal={revealProjectPath}
                        onAttach={addAttachment}
                      />
                    ))}
                  </div>
                ) : (
                  <div className="p-2">
                    {tree && (
                      <ProjectTree
                        node={tree}
                        attachments={attachments}
                        selectedPath={preview?.path}
                        previewLoadingPath={previewLoadingPath}
                        attachmentLoadingPath={attachmentLoadingPath}
                        fileActionPath={fileActionPath}
                        canOpenExternal={Boolean(platform.openProjectPath)}
                        canReveal={Boolean(platform.revealProjectPath)}
                        onOpen={openFile}
                        onOpenExternal={openProjectPath}
                        onReveal={revealProjectPath}
                        onAttach={addAttachment}
                      />
                    )}
                  </div>
                )}
              </div>
            </div>
          </section>

          <div className="grid min-w-0 gap-5">
            <section className="border border-border">
              <SectionHeader title="Selected Attachments" detail={`${attachments.length}`} />
              <AttachmentSet paths={attachments} onRemove={removeAttachment} />
            </section>

            <section className="min-w-0 border border-border">
              <SectionHeader
                title="Preview"
                detail={preview ? `${preview.language} / ${preview.line_count} lines` : "No file selected"}
              />
              {!preview && !previewLoadingPath && (
                <p className="px-3 py-3 text-xs text-muted-foreground">Select a file to preview it.</p>
              )}
              {previewLoadingPath && (
                <p className="px-3 py-3 text-xs text-muted-foreground">Loading {previewLoadingPath}...</p>
              )}
              {preview && !previewLoadingPath && (
                <>
                  <div className="flex flex-col gap-2 border-b border-border px-3 py-2 text-xs sm:flex-row sm:items-center sm:justify-between">
                    <div className="min-w-0 truncate font-mono text-primary">{preview.path}</div>
                    <div className="flex flex-wrap gap-2 sm:justify-end">
                      <FileActionButtons
                        path={preview.path}
                        actionLoading={fileActionPath}
                        canOpenExternal={Boolean(platform.openProjectPath)}
                        canReveal={Boolean(platform.revealProjectPath)}
                        onOpenExternal={openProjectPath}
                        onReveal={revealProjectPath}
                      />
                      <button
                        type="button"
                        onClick={() => void addAttachment(preview.path)}
                        disabled={attachments.includes(preview.path) || attachmentLoadingPath === preview.path}
                        className="inline-flex items-center gap-2 border border-border px-2 py-1 text-[10px] font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
                      >
                        <Paperclip size={13} aria-hidden="true" />
                        {attachments.includes(preview.path) ? "Attached" : "Attach"}
                      </button>
                    </div>
                  </div>
                  <pre className="max-h-[36rem] overflow-auto whitespace-pre-wrap break-words px-3 py-3 font-mono text-xs leading-relaxed text-foreground">
                    {preview.content}
                  </pre>
                </>
              )}
            </section>
          </div>
        </div>
      </div>
    </div>
  );
}

function ProjectTree({
  node,
  attachments,
  selectedPath,
  previewLoadingPath,
  attachmentLoadingPath,
  fileActionPath,
  canOpenExternal,
  canReveal,
  onOpen,
  onOpenExternal,
  onReveal,
  onAttach,
  parentPath = "",
}: {
  node: ProjectTreeNode;
  attachments: string[];
  selectedPath?: string;
  previewLoadingPath: string | null;
  attachmentLoadingPath: string | null;
  fileActionPath: string | null;
  canOpenExternal: boolean;
  canReveal: boolean;
  onOpen: (path: string) => Promise<void>;
  onOpenExternal: (path: string) => Promise<void>;
  onReveal: (path: string) => Promise<void>;
  onAttach: (path: string) => Promise<void>;
  parentPath?: string;
}) {
  const path = childPath(parentPath, node);
  if (node.type === "file") {
    return (
      <FileRow
        file={{ name: node.name, path }}
        selected={selectedPath === path}
        attached={attachments.includes(path)}
        loading={previewLoadingPath === path || attachmentLoadingPath === path}
        actionLoading={fileActionPath}
        canOpenExternal={canOpenExternal}
        canReveal={canReveal}
        onOpen={onOpen}
        onOpenExternal={onOpenExternal}
        onReveal={onReveal}
        onAttach={onAttach}
      />
    );
  }
  const children = node.children ?? [];
  if (node.name === ".") {
    return (
      <div className="grid gap-1">
        {children.map((child) => (
          <ProjectTree
            key={`${child.type}:${childPath(path, child)}`}
            node={child}
            attachments={attachments}
            selectedPath={selectedPath}
            previewLoadingPath={previewLoadingPath}
            attachmentLoadingPath={attachmentLoadingPath}
            fileActionPath={fileActionPath}
            canOpenExternal={canOpenExternal}
            canReveal={canReveal}
            onOpen={onOpen}
            onOpenExternal={onOpenExternal}
            onReveal={onReveal}
            onAttach={onAttach}
            parentPath={path}
          />
        ))}
      </div>
    );
  }
  return (
    <details open className="group">
      <summary className="flex cursor-pointer list-none items-center gap-2 px-2 py-1.5 text-xs text-muted-foreground hover:bg-muted/40 hover:text-foreground">
        <Folder size={14} aria-hidden="true" className="shrink-0" />
        <span className="min-w-0 truncate font-mono">{node.name}</span>
      </summary>
      <div className="ml-3 grid gap-1 border-l border-border/70 pl-2">
        {children.map((child) => (
          <ProjectTree
            key={`${child.type}:${childPath(path, child)}`}
            node={child}
            attachments={attachments}
            selectedPath={selectedPath}
            previewLoadingPath={previewLoadingPath}
            attachmentLoadingPath={attachmentLoadingPath}
            fileActionPath={fileActionPath}
            canOpenExternal={canOpenExternal}
            canReveal={canReveal}
            onOpen={onOpen}
            onOpenExternal={onOpenExternal}
            onReveal={onReveal}
            onAttach={onAttach}
            parentPath={path}
          />
        ))}
      </div>
    </details>
  );
}

function FileRow({
  file,
  selected,
  attached,
  loading,
  actionLoading,
  canOpenExternal,
  canReveal,
  onOpen,
  onOpenExternal,
  onReveal,
  onAttach,
}: {
  file: ProjectFileOption;
  selected: boolean;
  attached: boolean;
  loading: boolean;
  actionLoading: string | null;
  canOpenExternal: boolean;
  canReveal: boolean;
  onOpen: (path: string) => Promise<void>;
  onOpenExternal: (path: string) => Promise<void>;
  onReveal: (path: string) => Promise<void>;
  onAttach: (path: string) => Promise<void>;
}) {
  return (
    <div className={`grid gap-2 px-2 py-1.5 text-xs sm:grid-cols-[minmax(0,1fr)_auto] ${selected ? "bg-muted/60" : ""}`}>
      <button
        type="button"
        onClick={() => void onOpen(file.path)}
        className="flex min-w-0 items-center gap-2 text-left text-muted-foreground hover:text-foreground"
      >
        <FileText size={14} className="shrink-0" aria-hidden="true" />
        <span className="truncate font-mono">{file.path}</span>
      </button>
      <div className="flex flex-wrap gap-2 sm:justify-end">
        <button
          type="button"
          onClick={() => void onOpen(file.path)}
          className="inline-flex items-center gap-1 border border-border px-2 py-1 text-[10px] font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary"
        >
          <Eye size={12} aria-hidden="true" />
          Preview
        </button>
        <FileActionButtons
          path={file.path}
          actionLoading={actionLoading}
          canOpenExternal={canOpenExternal}
          canReveal={canReveal}
          onOpenExternal={onOpenExternal}
          onReveal={onReveal}
        />
        <button
          type="button"
          onClick={() => void onAttach(file.path)}
          disabled={attached || loading}
          aria-label={`Attach ${file.path} to launch`}
          className="inline-flex items-center gap-1 border border-border px-2 py-1 text-[10px] font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
        >
          <Paperclip size={12} aria-hidden="true" />
          {attached ? "Attached" : loading ? "Adding" : "Attach"}
        </button>
      </div>
    </div>
  );
}

function FileActionButtons({
  path,
  actionLoading,
  canOpenExternal,
  canReveal,
  onOpenExternal,
  onReveal,
}: {
  path: string;
  actionLoading: string | null;
  canOpenExternal: boolean;
  canReveal: boolean;
  onOpenExternal: (path: string) => Promise<void>;
  onReveal: (path: string) => Promise<void>;
}) {
  if (!canOpenExternal && !canReveal) return null;
  return (
    <>
      {canOpenExternal && (
        <button
          type="button"
          onClick={() => void onOpenExternal(path)}
          disabled={actionLoading !== null}
          aria-label={`Open ${path}`}
          className="inline-flex items-center gap-1 border border-border px-2 py-1 text-[10px] font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
        >
          <ExternalLink size={12} aria-hidden="true" />
          {actionLoading === `open:${path}` ? "Opening" : "Open"}
        </button>
      )}
      {canReveal && (
        <button
          type="button"
          onClick={() => void onReveal(path)}
          disabled={actionLoading !== null}
          aria-label={`Reveal ${path}`}
          className="inline-flex items-center gap-1 border border-border px-2 py-1 text-[10px] font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
        >
          <FolderOpen size={12} aria-hidden="true" />
          {actionLoading === `reveal:${path}` ? "Revealing" : "Reveal"}
        </button>
      )}
    </>
  );
}

function AttachmentSet({ paths, onRemove }: { paths: string[]; onRemove: (path: string) => void }) {
  if (paths.length === 0) {
    return <p className="px-3 py-3 text-xs text-muted-foreground">No project attachments selected.</p>;
  }
  return (
    <div className="flex flex-wrap gap-2 p-3">
      {paths.map((path) => (
        <span
          key={path}
          className="inline-flex max-w-full items-center gap-2 border border-border px-2 py-1 font-mono text-xs text-foreground"
        >
          <span className="truncate">{path}</span>
          <button
            type="button"
            onClick={() => onRemove(path)}
            aria-label={`Remove ${path} from project attachments`}
            className="shrink-0 text-muted-foreground hover:text-destructive"
          >
            <X size={13} aria-hidden="true" />
          </button>
        </span>
      ))}
    </div>
  );
}

function SectionHeader({ title, detail }: { title: string; detail?: string }) {
  return (
    <div className="flex items-center justify-between gap-3 border-b border-border bg-muted px-3 py-2">
      <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">{title}</h2>
      {detail && <span className="text-[10px] font-medium uppercase tracking-widest text-muted-foreground">{detail}</span>}
    </div>
  );
}

function StatusBanner({ text, tone }: { text: string; tone: "danger" | "warning" }) {
  const toneClass = tone === "danger" ? "border-destructive/50 text-destructive" : "border-warning/50 text-warning";
  return (
    <div className={`flex items-start gap-2 border bg-background px-3 py-2 text-xs ${toneClass}`}>
      <AlertTriangle size={14} aria-hidden="true" className="mt-0.5 shrink-0" />
      <span>{text}</span>
    </div>
  );
}
