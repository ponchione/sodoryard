import { Link } from "react-router-dom";
import { Separator } from "@/components/ui/separator";
import { ScrollArea } from "@/components/ui/scroll-area";

export function Sidebar() {
  return (
    <aside className="flex w-64 flex-col border-r border-border bg-sidebar text-sidebar-foreground">
      {/* Header */}
      <div className="flex items-center gap-2 px-4 py-3">
        <span className="text-lg font-semibold tracking-tight">sirtopham</span>
      </div>

      <Separator />

      {/* New conversation */}
      <div className="p-2">
        <Link
          to="/"
          className="inline-flex w-full items-center justify-start gap-2 rounded-lg border border-border bg-background px-2.5 py-1.5 text-sm font-medium hover:bg-muted"
        >
          <PlusIcon />
          New conversation
        </Link>
      </div>

      <Separator />

      {/* Conversation list placeholder */}
      <ScrollArea className="flex-1 px-2 py-1">
        <p className="px-2 py-8 text-center text-sm text-muted-foreground">
          Conversations will appear here
        </p>
      </ScrollArea>

      <Separator />

      {/* Footer / settings placeholder */}
      <div className="p-2">
        <Link
          to="/"
          className="inline-flex w-full items-center justify-start gap-2 rounded-lg px-2.5 py-1.5 text-sm font-medium text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          <SettingsIcon />
          Settings
        </Link>
      </div>
    </aside>
  );
}

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
