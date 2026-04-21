import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { MarkdownContent } from "./markdown-content";

describe("MarkdownContent", () => {
  it("renders plain markdown, inline code, and GFM tables", () => {
    render(
      <MarkdownContent
        content={[
          "A paragraph with `inline code`.",
          "",
          "| Name | Value |",
          "| --- | --- |",
          "| chunk | smaller |",
        ].join("\n")}
      />,
    );

    expect(screen.getByText("A paragraph with", { exact: false })).toBeInTheDocument();
    expect(screen.getByText("inline code")).toBeInTheDocument();
    expect(screen.getByRole("table")).toBeInTheDocument();
    expect(screen.getByText("smaller")).toBeInTheDocument();
  });

  it("lazy-loads highlighting for registered fenced code block languages", async () => {
    const { container } = render(
      <MarkdownContent
        content={["```typescript", "const answer = 42;", "```"].join("\n")}
      />,
    );

    expect(container.querySelector('[data-code-block-renderer="plain"]')).toBeTruthy();

    await waitFor(() => {
      expect(container.querySelector('[data-code-block-renderer="highlighted"]')).toBeTruthy();
    });
  });

  it("falls back to an unhighlighted block for unknown fenced code block languages", async () => {
    const { container } = render(
      <MarkdownContent
        content={["```brainfuck", "++++[>++++<-]>+.", "```"].join("\n")}
      />,
    );

    await waitFor(() => {
      const plainBlock = container.querySelector('[data-code-block-renderer="plain"]');
      expect(plainBlock).toBeTruthy();
      expect(container.querySelector('[data-code-block-renderer="highlighted"]')).toBeFalsy();
    });
  });
});
