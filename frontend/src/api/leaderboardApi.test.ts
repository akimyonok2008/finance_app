import { afterEach, describe, expect, it, vi } from "vitest";

import {
  getGlobalLeaderboard,
  getLeaderboardStanding,
} from "@/api/leaderboardApi";
import { getLeaderboardMe } from "@/api/dashboardApi";

function respondWith(body: unknown): void {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    ),
  );
}

const validEntry = {
  rank: 1,
  display_name: "Alpha",
  ranked_index: "112.34",
  ranked_return_percentage: "12.34",
};

const validStanding = {
  timeframe: "ALL",
  eligible: true,
  rank: 1,
  previous_rank: 2,
  rank_delta: 1,
  best_rank: 1,
  participant_count: 8,
  total_participants: 8,
  percentile: 100,
  ranked_return_percentage: "12.34",
  ranked_index: "112.34",
  paused: false,
  next_milestone: null,
  reason: "",
};

describe("leaderboard API contracts", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("preserves valid ranked decimal strings", async () => {
    respondWith([validEntry]);

    const result = await getGlobalLeaderboard({ timeframe: "ALL" });

    expect(result.entries).toEqual([
      {
        ...validEntry,
        public_weights: [],
        badges: [],
      },
    ]);
  });

  it.each([
    ["legacy numeric ranked index", [{ ...validEntry, ranked_index: 112.34 }]],
    ["malformed ranked return", [{ ...validEntry, ranked_return_percentage: "not-a-decimal" }]],
    ["malformed rank", [{ ...validEntry, rank: "1" }]],
    ["missing critical field", [{ rank: 1, display_name: "Alpha", ranked_index: "112.34" }]],
    ["invalid response envelope", { entries: [validEntry] }],
  ])("rejects %s instead of fabricating a value", async (_name, body) => {
    respondWith(body);

    await expect(getGlobalLeaderboard({ timeframe: "ALL" })).rejects.toThrow(
      /Contract violation in GET \/leaderboard/,
    );
  });

  it("rejects malformed standing fields instead of coercing them to zero", async () => {
    respondWith({
      ...validStanding,
      rank: "1",
      ranked_return_percentage: null,
      participant_count: "8",
    });

    await expect(getLeaderboardStanding("ALL")).rejects.toThrow(
      /Contract violation in GET \/leaderboard\/me/,
    );
  });

  it("rejects a standing returned for the wrong timeframe", async () => {
    respondWith({ ...validStanding, timeframe: "1M" });

    await expect(getLeaderboardStanding("ALL")).rejects.toThrow(
      /timeframe: expected ALL, received 1M/,
    );
  });

  it("rejects ranked decimals outside the display number range", async () => {
    respondWith({
      ...validStanding,
      ranked_index: "9".repeat(400),
    });

    await expect(getLeaderboardMe()).rejects.toThrow(
      /ranked decimal is out of display range/,
    );
  });
});
