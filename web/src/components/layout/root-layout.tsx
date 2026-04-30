import { useState, useCallback } from "react";
import { Outlet } from "react-router-dom";
import { Sidebar } from "@/components/layout/sidebar";

export function RootLayout() {
  const [sidebarOpen, setSidebarOpen] = useState(false);

  const handleClose = useCallback(() => setSidebarOpen(false), []);

  return (
    <div className="scanlines flex h-screen overflow-hidden bg-background text-foreground">
      <Sidebar open={sidebarOpen} onClose={handleClose} />

      <main className="flex flex-1 flex-col overflow-hidden">
        {/* Mobile top bar with hamburger */}
        <div className="flex items-center border-b border-border px-3 py-2 md:hidden">
          <button
            type="button"
            onClick={() => setSidebarOpen(true)}
            className="p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground"
            aria-label="Open sidebar"
          >
            <MenuIcon />
          </button>
          <span className="ml-2 text-sm font-semibold uppercase tracking-widest text-glow-cyan text-primary">
            Sodoryard
          </span>
        </div>

        <Outlet />
      </main>
    </div>
  );
}

function MenuIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <line x1="4" x2="20" y1="12" y2="12" />
      <line x1="4" x2="20" y1="6" y2="6" />
      <line x1="4" x2="20" y1="18" y2="18" />
    </svg>
  );
}
