import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { createBrowserRouter, RouterProvider } from "react-router-dom";
import "./index.css";
import { RootLayout } from "@/components/layout/root-layout";
import { ConversationListPage } from "@/pages/conversation-list";
import { ConversationPage } from "@/pages/conversation";

const router = createBrowserRouter([
  {
    path: "/",
    element: <RootLayout />,
    children: [
      { index: true, element: <ConversationListPage /> },
      { path: "c/:id", element: <ConversationPage /> },
    ],
  },
]);

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <RouterProvider router={router} />
  </StrictMode>,
);
