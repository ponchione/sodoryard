import { describe, expect, it } from "vitest";

import {
  assertProjectMemoryContractCompatible,
  projectMemoryContract,
  projectMemorySubscribeURL,
} from "./client";

describe("project memory Shunter client", () => {
  it("builds the subscribe URL under the dedicated project memory route", () => {
    expect(projectMemorySubscribeURL("http://localhost:5173")).toBe(
      "ws://localhost:5173/api/project-memory/subscribe",
    );
    expect(projectMemorySubscribeURL("https://yard.example")).toBe(
      "wss://yard.example/api/project-memory/subscribe",
    );
  });

  it("asserts the checked-in generated contract metadata", () => {
    expect(projectMemoryContract.moduleName).toBe("yard_project_memory");
    expect(projectMemoryContract.moduleVersion).toBe("0.12.0");
    expect(() => assertProjectMemoryContractCompatible()).not.toThrow();
  });
});
