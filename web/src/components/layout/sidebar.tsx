import { useCallback } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { Separator } from "@/components/ui/separator";
import { ScrollArea } from "@/components/ui/scroll-area";
import { useConversationList } from "@/hooks/use-conversation-list";
import type { ConversationSummary } from "@/types/api";

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
  const { conversations, loading, error, refresh, deleteConversation } =
    useConversationList();

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
    [deleteConversation, activeId, navigate],
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

      {/* Sidebar panel */}
      <aside
        className={`fixed inset-y-0 left-0 z-50 flex w-64 flex-col border-r border-border bg-sidebar text-sidebar-foreground transition-transform duration-200 md:static md:translate-x-0 ${
          open ? "translate-x-0" : "-translate-x-full"
        }`}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3">
          <span className="text-lg font-semibold tracking-tight">sirtopham</span>
          {/* Close button — mobile only */}
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-muted-foreground hover:bg-muted hover:text-foreground md:hidden"
            aria-label="Close sidebar"
          >
            <CloseIcon />
          </button>
        </div>

        <Separator />

        {/* New conversation */}
        <div className="p-2">
          <button
            type="button"
            onClick={handleNewConversation}
            className="inline-flex w-full items-center justify-start gap-2 rounded-lg border border-border bg-background px-2.5 py-1.5 text-sm font-medium hover:bg-muted"
          >
            <PlusIcon />
            New conversation
          </button>
        </div>

        <Separator />

        {/* Conversation list */}
        <ScrollArea className="flex-1 px-2 py-1">
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

          {!loading && !error && conversations.length === 0 && (
            <p className="px-2 py-8 text-center text-xs text-muted-foreground">
              No conversations yet
            </p>
          )}

          <div className="space-y-0.5">
            {conversations.map((conv) => (
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
          className="inline-flex w-full items-center justify-start gap-2 rounded-lg px-2.5 py-1.5 text-sm font-medium text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          <SettingsIcon />
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
      className={`group flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-sm transition-colors ${
        active
          ? "bg-muted font-medium text-foreground"
          : "text-muted-foreground hover:bg-muted/50 hover:text-foreground"
      }`}
    >
      <div className="min-w-0 flex-1">
        <div className="truncate">{title}</div>
        <div className="truncate text-[10px] text-muted-foreground/60">{time}</div>
      </div>
      <button
        type="button"
        onClick={(e) => onDelete(e, conversation.id)}
        className="shrink-0 rounded p-0.5 text-muted-foreground/40 opacity-0 transition-opacity hover:bg-destructive/10 hover:text-destructive group-hover:opacity-100"
        aria-label={`Delete ${title}`}
      >
        <TrashIcon />
      </button>
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

// ── Icons ────────────────────────────────────────────────────────────

function PlusIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M5 12h14" />
      <path d="M12 5v14" />
    </svg>
  );
}

function CloseIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="18"
      height="18"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M18 6 6 18" />
      <path d="m6 6 12 12" />
    </svg>
  );
}

function TrashIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M3 6h18" />
      <path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6" />
      <path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2" />
    </svg>
  );
}

function SettingsIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  );
}
