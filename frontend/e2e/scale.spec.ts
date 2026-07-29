import { expect, test } from "@playwright/test";
import type { Client } from "pg";
import {
  createVerifiedUser,
  json,
  withDatabase,
} from "./support";

async function materializeActiveLeaderboardRanks(db: Client): Promise<void> {
  await db.query(
    `UPDATE leaderboard_rankings lr
     SET rnk = ranked.rnk
     FROM (
       SELECT lr2.timeframe, lr2.user_id,
              RANK() OVER (
                PARTITION BY lr2.timeframe
                ORDER BY lr2.ranked_return_percentage DESC,
                         u.display_name ASC,
                         lr2.user_id ASC
              )::integer AS rnk
       FROM leaderboard_rankings lr2
       JOIN users u ON u.id = lr2.user_id AND u.deleted_at IS NULL
       WHERE lr2.generation = (
         SELECT active_generation FROM leaderboard_ranking_state WHERE id
       )
     ) ranked
     WHERE lr.generation = (
       SELECT active_generation FROM leaderboard_ranking_state WHERE id
     )
       AND lr.timeframe = ranked.timeframe
       AND lr.user_id = ranked.user_id`,
  );
}

test("large activity histories remain readable", async () => {
  const user = await createVerifiedUser("History");
  try {
    await Promise.all(
      Array.from({ length: 120 }, (_, index) =>
        user.client.post("/portfolio/deposits", {
          headers: { "Idempotency-Key": `history-${crypto.randomUUID()}` },
          data: { currency: "USD", amount: `${index + 1}` },
        }),
      ),
    );
    const response = await user.client.get("/portfolio/activities");
    expect(response.status()).toBe(200);
    const activities = await json(response);
    expect(Array.isArray(activities)).toBe(true);
    expect((activities as unknown[]).length).toBeGreaterThanOrEqual(100);
  } finally {
    await user.client.dispose();
  }
});

test("leaderboard ranks and caps more than 100 participants", async () => {
  const viewer = await createVerifiedUser("ScaleViewer");
  const marker = `scale-${Date.now()}-${crypto.randomUUID().slice(0, 8)}`;
  try {
    await withDatabase(async (db) => {
      await db.query("BEGIN");
      try {
        await db.query(
          `WITH seeded AS (
             INSERT INTO users (id, email, password_hash, display_name, avatar_key)
             SELECT gen_random_uuid(),
                    $1 || '-' || n || '@example.test',
                    'not-a-login-hash',
                    'Scale ' || lpad(n::text, 3, '0'),
                    'default'
             FROM generate_series(1, 125) AS n
             RETURNING id, display_name
           )
           INSERT INTO leaderboard_rankings
             (user_id, timeframe, generation, ranked_index,
              ranked_return_percentage, tracking_started_at)
           SELECT id, 'ALL',
                  (SELECT active_generation FROM leaderboard_ranking_state WHERE id),
                  100 + row_number() OVER (ORDER BY display_name),
                  row_number() OVER (ORDER BY display_name),
                  now() - interval '1 year'
           FROM seeded`,
          [marker],
        );
        await materializeActiveLeaderboardRanks(db);
        // Background workers are intentionally disabled in the browser
        // harness. Mark this test-built generation fresh so the API exercises
        // the same materialized-rank read path production uses after
        // CompleteCycle.
        await db.query(
          "UPDATE leaderboard_ranking_state SET activated_at = now() WHERE id",
        );
        await db.query("COMMIT");
      } catch (error) {
        await db.query("ROLLBACK");
        throw error;
      }
    });

    const response = await viewer.client.get("/leaderboard?timeframe=ALL");
    expect(response.status()).toBe(200);
    const board = await json(response);
    expect(Array.isArray(board)).toBe(true);
    expect(board).toHaveLength(100);
    expect((board as Array<{ rank: number }>)[0].rank).toBe(1);
    expect((board as Array<{ rank: number }>)[99].rank).toBe(100);
  } finally {
    await withDatabase(async (db) => {
      await db.query("BEGIN");
      try {
        await db.query("DELETE FROM users WHERE email LIKE $1", [`${marker}-%`]);
        await materializeActiveLeaderboardRanks(db);
        await db.query(
          "UPDATE leaderboard_ranking_state SET activated_at = NULL WHERE id",
        );
        await db.query("COMMIT");
      } catch (error) {
        await db.query("ROLLBACK");
        throw error;
      }
    });
    await viewer.client.dispose();
  }
});
