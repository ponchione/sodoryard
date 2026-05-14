import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { Sidebar } from "./sidebar";

vi.mock("@/hooks/use-conversation-list", () => ({
  useConversationList: () => ({
    conversations: Array.from({ length: 40 }, (_, i) => ({
      id: `conv-${i}`,
      title: `Conversation ${i}`,
      updated_at: "2026-04-18T00:00:00Z",
    })),
    searchQuery: "",
    setSearchQuery: vi.fn(),
    searchResults: [],
    searching: false,
    searchError: "",
    showingSearchResults: false,
    loading: false,
    error: "",
    refresh: vi.fn(),
    deleteConversation: vi.fn(),
  }),
}));

describe("Sidebar layout", () => {
  it("constrains the sidebar list inside the viewport instead of growing the root layout", () => {
    const { container } = render(
      <MemoryRouter initialEntries={["/"]}>
        <Sidebar open={false} onClose={() => {}} />
      </MemoryRouter>,
    );

    const aside = container.querySelector("aside");
    expect(aside).toBeTruthy();
    expect(aside).toHaveClass("min-h-0");

    const scrollArea = container.querySelector('[data-slot="scroll-area"]');
    expect(scrollArea).toBeTruthy();
    expect(scrollArea).toHaveClass("min-h-0");
  });

  it("links to the dashboard workspace", () => {
    render(
      <MemoryRouter initialEntries={["/dashboard"]}>
        <Sidebar open={false} onClose={() => {}} />
      </MemoryRouter>,
    );

    expect(screen.getAllByRole("link", { name: "Dashboard" })[0]).toHaveAttribute("href", "/dashboard");
    expect(screen.getAllByRole("link", { name: "Launch" })[0]).toHaveAttribute("href", "/launch");
    expect(screen.getAllByRole("link", { name: "Project" })[0]).toHaveAttribute("href", "/project");
  });
});
