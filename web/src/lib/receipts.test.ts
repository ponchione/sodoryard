import { describe, expect, it } from "vitest";
import { parseReceiptDocument, receiptFollowUpSections, receiptRoute } from "./receipts";

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

  it("extracts follow-up sections from receipt body headings", () => {
    const parsed = parseReceiptDocument([
      "---",
      "role: coder",
      "---",
      "## Summary",
      "Done.",
      "",
      "## Concerns",
      "- Review the retry edge case.",
      "",
      "### Detail",
      "Nested notes stay with concerns.",
      "",
      "## Next Steps",
      "Code is ready for audit.",
    ].join("\n"));

    expect(receiptFollowUpSections(parsed)).toEqual([
      {
        title: "Concerns",
        content: "- Review the retry edge case.\n\n### Detail\nNested notes stay with concerns.",
      },
      {
        title: "Next Steps",
        content: "Code is ready for audit.",
      },
    ]);
  });
});
