import { describe, expect, it } from "vitest";
import { chainStatusClass, chainStatusGroup } from "./chain-status";

describe("chain status helpers", () => {
  it("treats dry_run as a successful terminal status", () => {
    expect(chainStatusGroup("dry_run")).toBe("success");
    expect(chainStatusClass("dry_run")).toBe("text-accent");
  });
});
