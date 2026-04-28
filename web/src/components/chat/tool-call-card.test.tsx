import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ToolCallCard } from "./tool-call-card";

describe("ToolCallCard details", () => {
  it("renders shell details above the raw result", () => {
    render(
      <ToolCallCard
        block={{
          kind: "tool_call",
          toolCallId: "tc-shell",
          toolName: "shell",
          output: "",
          result: "Exit code: 2\n\nSTDERR:\nboom",
          details: {
            version: 1,
            kind: "shell",
            exit_code: 2,
            timed_out: false,
            cancelled: false,
            output_bytes: 32,
            stdout_bytes: 0,
            stderr_bytes: 4,
          },
          done: true,
          success: true,
        }}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /shell/ }));

    expect(screen.getByText("Details")).toBeInTheDocument();
    expect(screen.getByText("exit")).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument();
    expect(screen.getByText("stdout 0 B / stderr 4 B")).toBeInTheDocument();
    expect(screen.getByText(/Exit code: 2/)).toBeInTheDocument();
  });

  it("renders file mutation details above the raw result", () => {
    render(
      <ToolCallCard
        block={{
          kind: "tool_call",
          toolCallId: "tc-edit",
          toolName: "file_edit",
          output: "",
          result: "--- a/main.go\n+++ b/main.go",
          details: {
            version: 1,
            kind: "file_mutation",
            operation: "edit",
            path: "internal/tool/file_edit.go",
            created: false,
            changed: true,
            bytes_before: 100,
            bytes_after: 125,
            diff_line_count: 6,
            diff_truncated: false,
          },
          done: true,
          success: true,
        }}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /file_edit/ }));

    expect(screen.getByText("internal/tool/file_edit.go")).toBeInTheDocument();
    expect(screen.getByText("edit")).toBeInTheDocument();
    expect(screen.getByText("changed")).toBeInTheDocument();
    expect(screen.getByText("100 B -> 125 B (+25 B)")).toBeInTheDocument();
    expect(screen.getByText("6 lines")).toBeInTheDocument();
    expect(screen.getByText(/--- a\/main.go/)).toBeInTheDocument();
  });
});
