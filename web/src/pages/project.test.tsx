import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { useApiResourceMock, apiGetMock, apiPostMock, chooseProjectFilesMock, openProjectPathMock, revealProjectPathMock } = vi.hoisted(() => ({
  useApiResourceMock: vi.fn(),
  apiGetMock: vi.fn(),
  apiPostMock: vi.fn(),
  chooseProjectFilesMock: vi.fn(),
  openProjectPathMock: vi.fn(),
  revealProjectPathMock: vi.fn(),
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

vi.mock("@/platform", () => ({
  getYardPlatform: () => ({
    kind: "desktop",
    backendBaseUrl: "",
    openExternal: vi.fn(),
    chooseProjectFiles: chooseProjectFilesMock,
    openProjectPath: openProjectPathMock,
    revealProjectPath: revealProjectPathMock,
  }),
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
    chooseProjectFilesMock.mockReset().mockResolvedValue([]);
    openProjectPathMock.mockReset().mockResolvedValue(undefined);
    revealProjectPathMock.mockReset().mockResolvedValue(undefined);
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

  it("validates project files before opening or revealing them through desktop actions", async () => {
    render(
      <MemoryRouter>
        <ProjectPage />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Open docs/specs/24-electron-desktop-app.md" }));

    await waitFor(() => {
      expect(apiPostMock).toHaveBeenCalledWith("/api/project/validate-paths", {
        purpose: "open_editor",
        paths: ["docs/specs/24-electron-desktop-app.md"],
      });
      expect(openProjectPathMock).toHaveBeenCalledWith("docs/specs/24-electron-desktop-app.md");
    });

    fireEvent.click(screen.getByRole("button", { name: "Reveal docs/specs/24-electron-desktop-app.md" }));

    await waitFor(() => {
      expect(apiPostMock).toHaveBeenCalledWith("/api/project/validate-paths", {
        purpose: "reveal",
        paths: ["docs/specs/24-electron-desktop-app.md"],
      });
      expect(revealProjectPathMock).toHaveBeenCalledWith("docs/specs/24-electron-desktop-app.md");
    });
  });

  it("validates native dialog selections before launch handoff", async () => {
    chooseProjectFilesMock.mockResolvedValue(["README.md", "docs/specs/24-electron-desktop-app.md"]);
    apiPostMock.mockResolvedValue({
      accepted: ["README.md", "docs/specs/24-electron-desktop-app.md"],
      rejected: [],
    });

    render(
      <MemoryRouter>
        <ProjectPage />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Choose Files" }));

    await waitFor(() => {
      expect(chooseProjectFilesMock).toHaveBeenCalled();
      expect(apiPostMock).toHaveBeenCalledWith("/api/project/validate-paths", {
        purpose: "launch_attachment",
        paths: ["README.md", "docs/specs/24-electron-desktop-app.md"],
      });
    });
    expect(screen.getByRole("link", { name: "Launch with attachments" })).toHaveAttribute(
      "href",
      "/launch?source_spec=README.md&source_spec=docs%2Fspecs%2F24-electron-desktop-app.md",
    );
  });
});
