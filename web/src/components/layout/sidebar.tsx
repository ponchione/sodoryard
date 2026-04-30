import { useCallback, useEffect, useRef } from "react";
import { Plus, Search, Settings, Trash2, X } from "lucide-react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { Separator } from "@/components/ui/separator";
import { ScrollArea } from "@/components/ui/scroll-area";
import { useConversationList } from "@/hooks/use-conversation-list";
import type { ConversationSummary, SearchResult } from "@/types/api";

interface SidebarProps {
  open: boolean;
  onClose: () => void;
}

/**
 * Extract the conversation ID from the current path.
 * Paths look like /c/:id — returns the id or undefined.
 */
function useActiveConversationId(): string | undefined {
  const location = useLocation();
  const match = /^\/c\/(.+)$/.exec(location.pathname);
  return match ? match[1] : undefined;
}

export function Sidebar({ open, onClose }: SidebarProps) {
  const activeId = useActiveConversationId();
  const navigate = useNavigate();
  const {
    conversations,
    searchQuery,
    setSearchQuery,
    searchResults,
    searching,
    searchError,
    showingSearchResults,
    loading,
    error,
    refresh,
    deleteConversation,
  } = useConversationList();

  // B2 fix: when the user navigates to a conversation that isn't in the
  // current list (e.g. a brand-new one created from the landing page), refresh
  // the list once. Guarded so we don't loop on a truly missing id.
  const refreshedIdsRef = useRef<Set<string>>(new Set());
  useEffect(() => {
    if (!activeId || loading) return;
    if (conversations.some((c) => c.id === activeId)) return;
    if (refreshedIdsRef.current.has(activeId)) return;
    refreshedIdsRef.current.add(activeId);
    refresh();
  }, [activeId, conversations, loading, refresh]);

  const handleNewConversation = useCallback(() => {
    navigate("/");
    onClose();
  }, [navigate, onClose]);

  const handleSelectConversation = useCallback(
    (id: string) => {
      navigate(`/c/${id}`);
      onClose();
    },
    [navigate, onClose],
  );

  const handleDelete = useCallback(
    async (e: React.MouseEvent, id: string) => {
      e.stopPropagation();
      e.preventDefault();
      const conversation = conversations.find((c) => c.id === id);
      const title = conversation?.title?.trim() || "Untitled";
      if (!window.confirm(`Delete conversation "${title}"?`)) {
        return;
      }
      try {
        await deleteConversation(id);
        // If we deleted the active conversation, navigate home.
        if (activeId === id) {
          navigate("/");
        }
      } catch (err) {
        console.error("Failed to delete conversation:", err);
      }
    },
    [conversations, deleteConversation, activeId, navigate],
  );

  return (
    <>
      {/* Mobile overlay backdrop */}
      {open && (
        <div
          className="fixed inset-0 z-40 bg-black/50 md:hidden"
          onClick={onClose}
        />
      )}

      {/* Sidebar panel — augmented frame */}
      <aside
        data-augmented-ui="tr-clip border"
        className={`fixed inset-y-0 left-0 z-50 flex min-h-0 w-64 flex-col bg-sidebar text-sidebar-foreground transition-transform duration-200 md:static md:translate-x-0 ${
          open ? "translate-x-0" : "-translate-x-full"
        }`}
        style={{
          "--aug-tr": "20px",
          "--aug-border-right": "2px",
          "--aug-border-bg":
            "linear-gradient(180deg, #00e5ff, #00e5ff40, transparent)",
        } as React.CSSProperties}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3">
          <span className="text-lg font-semibold uppercase tracking-widest text-primary text-glow-cyan">
            Sodoryard
          </span>
          {/* Close button — mobile only */}
          <button
            type="button"
            onClick={onClose}
            className="p-1 text-muted-foreground hover:bg-muted hover:text-foreground md:hidden"
            aria-label="Close sidebar"
          >
            <X size={18} aria-hidden="true" />
          </button>
        </div>

        <Separator />

        {/* New conversation — augmented button */}
        <div className="p-2">
          <button
            type="button"
            onClick={handleNewConversation}
            data-augmented-ui="tl-clip br-clip border"
            className="inline-flex w-full items-center justify-start gap-2 border-0 bg-background px-2.5 py-1.5 text-sm font-medium text-primary hover:bg-muted"
            style={{
              "--aug-tl": "8px",
              "--aug-br": "8px",
              "--aug-border-all": "1px",
              "--aug-border-bg": "#00e5ff",
            } as React.CSSProperties}
          >
            <Plus size={16} aria-hidden="true" />
            New conversation
          </button>
        </div>

        <Separator />

        <div className="p-2">
          <label htmlFor="conversation-search" className="sr-only">
            Search conversations
          </label>
          <div className="relative">
            <Search
              size={14}
              aria-hidden="true"
              className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-muted-foreground/70"
            />
            <input
              id="conversation-search"
              type="search"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search conversations"
              className="w-full border border-border bg-background px-8 py-2 text-sm text-foreground outline-none transition-colors placeholder:text-muted-foreground/60 focus:border-primary"
            />
            {showingSearchResults && (
              <button
                type="button"
                onClick={() => setSearchQuery("")}
                className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-muted-foreground hover:text-foreground"
                aria-label="Clear search"
              >
                <X size={14} aria-hidden="true" />
              </button>
            )}
          </div>
        </div>

        {/* Conversation list */}
        <ScrollArea className="min-h-0 flex-1 px-2 py-1">
          {loading && conversations.length === 0 && (
            <p className="px-2 py-8 text-center text-xs text-muted-foreground">
              Loading…
            </p>
          )}

          {error && (
            <div className="px-2 py-4 text-center text-xs text-destructive">
              {error}
              <button
                type="button"
                onClick={refresh}
                className="ml-1 underline hover:no-underline"
              >
                Retry
              </button>
            </div>
          )}

          {showingSearchResults && searching && (
            <p className="px-2 py-3 text-xs text-muted-foreground">Searching…</p>
          )}

          {showingSearchResults && searchError && (
            <div className="px-2 py-4 text-center text-xs text-destructive">
              {searchError}
            </div>
          )}

          {!showingSearchResults && !loading && !error && conversations.length === 0 && (
            <p className="px-2 py-8 text-center text-xs text-muted-foreground">
              No conversations yet
            </p>
          )}

          {showingSearchResults && !searching && !searchError && searchResults.length === 0 && (
            <p className="px-2 py-8 text-center text-xs text-muted-foreground">
              No matches for “{searchQuery.trim()}”
            </p>
          )}

          <div className="space-y-0.5">
            {showingSearchResults
              ? searchResults.map((result) => (
                  <SearchResultItem
                    key={result.id}
                    result={result}
                    active={activeId === result.id}
                    onSelect={handleSelectConversation}
                  />
                ))
              : conversations.map((conv) => (
                  <ConversationItem
                    key={conv.id}
                    conversation={conv}
                    active={activeId === conv.id}
                    onSelect={handleSelectConversation}
                    onDelete={handleDelete}
                  />
                ))}
          </div>
        </ScrollArea>

        <Separator />

        {/* Footer / settings */}
        <div className="p-2">
          <Link
            to="/settings"
            onClick={onClose}
            className="inline-flex w-full items-center justify-start gap-2 px-2.5 py-1.5 text-sm font-medium text-muted-foreground hover:bg-muted hover:text-foreground"
          >
            <Settings size={16} aria-hidden="true" />
            Settings
          </Link>
        </div>
      </aside>
    </>
  );
}

// ── Conversation list item ───────────────────────────────────────────

function ConversationItem({
  conversation,
  active,
  onSelect,
  onDelete,
}: {
  conversation: ConversationSummary;
  active: boolean;
  onSelect: (id: string) => void;
  onDelete: (e: React.MouseEvent, id: string) => void;
}) {
  const title = conversation.title || "Untitled";
  const time = formatRelativeTime(conversation.updated_at);

  return (
    <button
      type="button"
      onClick={() => onSelect(conversation.id)}
      className={`group flex w-full items-center gap-2 px-2.5 py-2 text-left text-sm transition-colors ${
        active
          ? "border-l-2 border-l-primary bg-muted font-medium text-foreground"
          : "border-l-2 border-l-transparent text-muted-foreground hover:bg-muted/50 hover:text-foreground"
      }`}
      style={
        active
          ? ({ boxShadow: "inset 4px 0 8px -4px #00e5ff40" } as React.CSSProperties)
          : undefined
      }
    >
      <div className="min-w-0 flex-1">
        <div className="truncate">{title}</div>
        <div className="truncate text-[10px] text-muted-foreground/60">{time}</div>
      </div>
      <button
        type="button"
        onClick={(e) => onDelete(e, conversation.id)}
        className="shrink-0 p-0.5 text-muted-foreground/40 opacity-0 transition-opacity hover:bg-destructive/10 hover:text-destructive group-hover:opacity-100"
        aria-label={`Delete ${title}`}
      >
        <Trash2 size={14} aria-hidden="true" />
      </button>
    </button>
  );
}

function SearchResultItem({
  result,
  active,
  onSelect,
}: {
  result: SearchResult;
  active: boolean;
  onSelect: (id: string) => void;
}) {
  const title = result.title || "Untitled";
  const time = formatRelativeTime(result.updated_at);

  return (
    <button
      type="button"
      onClick={() => onSelect(result.id)}
      className={`group flex w-full flex-col gap-1 px-2.5 py-2 text-left text-sm transition-colors ${
        active
          ? "border-l-2 border-l-primary bg-muted font-medium text-foreground"
          : "border-l-2 border-l-transparent text-muted-foreground hover:bg-muted/50 hover:text-foreground"
      }`}
      style={
        active
          ? ({ boxShadow: "inset 4px 0 8px -4px #00e5ff40" } as React.CSSProperties)
          : undefined
      }
    >
      <div className="flex min-w-0 items-center gap-2">
        <div className="min-w-0 flex-1 truncate">{title}</div>
        <div className="shrink-0 text-[10px] text-muted-foreground/60">{time}</div>
      </div>
      <div className="line-clamp-2 text-xs text-muted-foreground/80">{result.snippet}</div>
    </button>
  );
}

// ── Time formatting ──────────────────────────────────────────────────

function formatRelativeTime(isoString: string): string {
  const date = new Date(isoString);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMin = Math.floor(diffMs / 60_000);
  const diffHr = Math.floor(diffMs / 3_600_000);
  const diffDay = Math.floor(diffMs / 86_400_000);

  if (diffMin < 1) return "just now";
  if (diffMin < 60) return `${diffMin}m ago`;
  if (diffHr < 24) return `${diffHr}h ago`;
  if (diffDay < 7) return `${diffDay}d ago`;

  return date.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}
