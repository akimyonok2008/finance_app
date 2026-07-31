import { afterEach, describe, expect, it, vi } from "vitest";

import { getAchievementReturns } from "@/api/achievements";

describe("achievement returns API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("requests the selected leaderboard timeframe", async () => {
    const fetchMock = vi.fn(async () =>
      new Response(
        JSON.stringify({
          timeframe: "6M",
          to: "2026-07-31T00:00:00Z",
          rows: [],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const result = await getAchievementReturns("6M");

    expect(result.timeframe).toBe("6M");
    expect(fetchMock).toHaveBeenCalledWith(
      "http://localhost:8080/achievements/returns?timeframe=6M",
      expect.objectContaining({ method: "GET", credentials: "include" }),
    );
  });
});
