import { describe, expect, it } from "vitest";
import { chainStatusClass, chainStatusGroup } from "./chain-status";

describe("chain status helpers", () => {
  it("treats waiting approval as an active chain state", () => {
    expect(chainStatusGroup("waiting_approval")).toBe("active");
    expect(chainStatusClass("waiting_approval")).toBe("text-warning");
  });

  it("treats dry_run as a successful terminal status", () => {
    expect(chainStatusGroup("dry_run")).toBe("success");
    expect(chainStatusClass("dry_run")).toBe("text-accent");
  });
});
