import { expect, test } from "@playwright/test";
import {
  createVerifiedUser,
  json,
  withDatabase,
} from "./support";

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
           (user_id, timeframe, ranked_index, ranked_return_percentage, tracking_started_at)
         SELECT id, 'ALL', 100 + row_number() OVER (ORDER BY display_name),
                row_number() OVER (ORDER BY display_name), now() - interval '1 year'
         FROM seeded`,
        [marker],
      );
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
      await db.query("DELETE FROM users WHERE email LIKE $1", [`${marker}-%`]);
    });
    await viewer.client.dispose();
  }
});
