import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { apiGet } = vi.hoisted(() => ({
  apiGet: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  api: {
    get: apiGet,
  },
}));

import { useApiResource } from "./use-api-resource";

describe("useApiResource", () => {
  beforeEach(() => {
    apiGet.mockReset();
  });

  it("does not refetch when callers pass inline fallback values", async () => {
    apiGet.mockResolvedValue([{ id: "chain-1" }]);

    const { result, rerender } = renderHook(() => (
      useApiResource<Array<{ id: string }>>("/api/chains", [])
    ));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(apiGet).toHaveBeenCalledTimes(1);
    expect(result.current.data).toEqual([{ id: "chain-1" }]);

    rerender();

    expect(apiGet).toHaveBeenCalledTimes(1);
  });
});
