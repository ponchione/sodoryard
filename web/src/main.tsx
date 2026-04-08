import { StrictMode, Suspense, lazy } from "react";
import { createRoot } from "react-dom/client";
import { createBrowserRouter, RouterProvider } from "react-router-dom";
import "./index.css";

const RootLayout = lazy(() => import("@/components/layout/root-layout").then((m) => ({ default: m.RootLayout })));
const ConversationListPage = lazy(() => import("@/pages/conversation-list").then((m) => ({ default: m.ConversationListPage })));
const ConversationPage = lazy(() => import("@/pages/conversation").then((m) => ({ default: m.ConversationPage })));
const SettingsPage = lazy(() => import("@/pages/settings").then((m) => ({ default: m.SettingsPage })));

function RouteFallback() {
  return <div className="min-h-screen bg-background" />;
}

const router = createBrowserRouter([
  {
    path: "/",
    element: (
      <Suspense fallback={<RouteFallback />}>
        <RootLayout />
      </Suspense>
    ),
    children: [
      {
        index: true,
        element: (
          <Suspense fallback={<RouteFallback />}>
            <ConversationListPage />
          </Suspense>
        ),
      },
      {
        path: "c/:id",
        element: (
          <Suspense fallback={<RouteFallback />}>
            <ConversationPage />
          </Suspense>
        ),
      },
      {
        path: "settings",
        element: (
          <Suspense fallback={<RouteFallback />}>
            <SettingsPage />
          </Suspense>
        ),
      },
    ],
  },
]);

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <RouterProvider router={router} />
  </StrictMode>,
);
