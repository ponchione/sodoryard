import { describe, expect, it } from "vitest";
import { parseReceiptDocument, receiptRoute } from "./receipts";

describe("receipt helpers", () => {
  it("builds receipt detail routes for orchestrator and step receipts", () => {
    expect(receiptRoute("chain one")).toBe("/receipts/chain%20one");
    expect(receiptRoute("chain one", "2")).toBe("/receipts/chain%20one/2");
  });

  it("extracts simple frontmatter and markdown body from a receipt", () => {
    const parsed = parseReceiptDocument([
      "---",
      "role: coder",
      "verdict: accepted",
      "changed_files:",
      "  - internal/a.go",
      "  - web/src/app.tsx",
      "validation: [rtk make test, rtk make build]",
      "---",
      "# Receipt",
      "",
      "Body",
    ].join("\n"));

    expect(parsed.frontmatter).toEqual({
      role: "coder",
      verdict: "accepted",
      changed_files: "internal/a.go, web/src/app.tsx",
      validation: "rtk make test, rtk make build",
    });
    expect(parsed.body).toBe("# Receipt\n\nBody");
  });
});
