import { Outlet } from "react-router-dom";
import { Sidebar } from "@/components/layout/sidebar";

export function RootLayout() {
  return (
    <div className="flex h-screen overflow-hidden bg-background text-foreground">
      <Sidebar />
      <main className="flex flex-1 flex-col overflow-hidden">
        <Outlet />
      </main>
    </div>
  );
}
