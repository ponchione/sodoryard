import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { useApiResourceMock, apiGetMock, apiPostMock } = vi.hoisted(() => ({
  useApiResourceMock: vi.fn(),
  apiGetMock: vi.fn(),
  apiPostMock: vi.fn(),
}));

vi.mock("@/hooks/use-api-resource", () => ({
  useApiResource: useApiResourceMock,
}));

vi.mock("@/lib/api", () => ({
  api: {
    get: apiGetMock,
    post: apiPostMock,
  },
}));

import { ProjectPage } from "./project";

describe("ProjectPage", () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    apiGetMock.mockReset().mockResolvedValue({
      path: "docs/specs/24-electron-desktop-app.md",
      content: "# Electron Desktop App\n\nSpec body.",
      language: "markdown",
      line_count: 3,
    });
    apiPostMock.mockReset().mockResolvedValue({
      accepted: ["docs/specs/24-electron-desktop-app.md"],
      rejected: [],
    });
    useApiResourceMock.mockReset().mockImplementation((path: string, fallback: unknown) => {
      if (path === "/api/project") {
        return {
          data: { name: "sodoryard", root_path: "/home/project", id: "/home/project" },
          loading: false,
          error: null,
          refresh: vi.fn(),
        };
      }
      if (path === "/api/project/tree?depth=6") {
        return {
          data: {
            name: ".",
            type: "dir",
            children: [
              {
                name: "docs",
                type: "dir",
                children: [
                  {
                    name: "specs",
                    type: "dir",
                    children: [{ name: "24-electron-desktop-app.md", type: "file" }],
                  },
                ],
              },
            ],
          },
          loading: false,
          error: null,
          refresh: vi.fn(),
        };
      }
      return { data: fallback, loading: false, error: null, refresh: vi.fn() };
    });
  });

  it("previews files and hands validated attachments to launch", async () => {
    render(
      <MemoryRouter>
        <ProjectPage />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getAllByRole("button", { name: /docs\/specs\/24-electron-desktop-app\.md/ })[0]);

    await waitFor(() => {
      expect(apiGetMock).toHaveBeenCalledWith(
        "/api/project/file?path=docs%2Fspecs%2F24-electron-desktop-app.md",
      );
    });
    expect(await screen.findByText("# Electron Desktop App", { exact: false })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", {
      name: "Attach docs/specs/24-electron-desktop-app.md to launch",
    }));

    await waitFor(() => {
      expect(apiPostMock).toHaveBeenCalledWith("/api/project/validate-paths", {
        purpose: "launch_attachment",
        paths: ["docs/specs/24-electron-desktop-app.md"],
      });
    });
    expect(screen.getByRole("link", { name: "Launch with attachments" })).toHaveAttribute(
      "href",
      "/launch?source_spec=docs%2Fspecs%2F24-electron-desktop-app.md",
    );
  });
});
