import { expect, test } from "@playwright/test";
import {
  createVerifiedUser,
  expectStatus,
  loginInBrowser,
  object,
  withDatabase,
} from "./support";

test("social messaging, blocking, reporting, and moderation are isolated", async ({
  page,
}) => {
  const alice = await createVerifiedUser("SocialAlice");
  const bob = await createVerifiedUser("SocialBob");
  const moderator = await createVerifiedUser("Moderator");
  try {
    const aliceProfile = await expectStatus(
      await alice.client.get("/profiles/me"),
      200,
    );
    const bobProfile = await expectStatus(
      await bob.client.get("/profiles/me"),
      200,
    );
    const aliceHandle = String(aliceProfile.handle);
    const bobHandle = String(bobProfile.handle);
    for (const client of [alice.client, bob.client]) {
      await expectStatus(
        await client.patch("/profiles/me", { data: { is_public: true } }),
        200,
      );
    }
    await expectStatus(
      await alice.client.post(
        `/social/follows/${encodeURIComponent(bobHandle)}`,
      ),
      200,
    );
    await expectStatus(
      await bob.client.post(
        `/social/follows/${encodeURIComponent(aliceHandle)}`,
      ),
      200,
    );
    const conversation = await expectStatus(
      await alice.client.post("/dm/conversations", {
        data: { handle: bobHandle },
      }),
      200,
    );
    const conversationID = String(object(conversation.conversation).id);
    const evidence = `moderation evidence ${crypto.randomUUID()}`;
    const sent = await expectStatus(
      await bob.client.post(
        `/dm/conversations/${conversationID}/messages`,
        { data: { body: evidence } },
      ),
      200,
    );
    const messageID = String(object(sent.message).id);
    await expectStatus(
      await alice.client.post(
        `/social/users/${encodeURIComponent(bobHandle)}/block`,
      ),
      200,
    );
    const report = await expectStatus(
      await alice.client.post(`/reports/messages/${messageID}`, {
        data: { category: "harassment", explanation: "release E2E report" },
      }),
      201,
    );
    const reportID = String(report.report_id);

    await withDatabase(async (db) => {
      await db.query("UPDATE users SET role = 'moderator' WHERE id = $1", [
        String(moderator.user.id),
      ]);
    });
    await loginInBrowser(page, moderator.email, moderator.password);
    await page.goto("/moderation/reports");
    await expect(
      page.getByRole("heading", { name: "Moderation queue" }),
    ).toBeVisible();
    await page.getByRole("button", { name: /harassment/ }).click();
    await expect(page.getByText(evidence)).toBeVisible();
    await page
      .getByLabel("Moderator notes (internal)")
      .fill("isolated browser E2E resolution");
    await page.getByRole("button", { name: "Dismiss (no action)" }).click();
    await expect(page.getByText("Decision: no_action")).toBeVisible();

    await expectStatus(
      await moderator.client.post("/auth/login", {
        data: { email: moderator.email, password: moderator.password },
      }),
      200,
    );
    const resolved = await expectStatus(
      await moderator.client.get(`/moderation/reports/${reportID}`),
      200,
    );
    expect(resolved.status).toBe("resolved_no_action");
  } finally {
    await Promise.all([
      alice.client.dispose(),
      bob.client.dispose(),
      moderator.client.dispose(),
    ]);
  }
});

test("account deletion revokes the browser session", async () => {
  const user = await createVerifiedUser("Deletion");
  try {
    const reauthenticated = await expectStatus(
      await user.client.post("/auth/reauthenticate", {
        data: { password: user.password },
      }),
      200,
    );
    const deleted = await user.client.post("/auth/delete-account", {
      data: {
        reauthentication_token: String(
          reauthenticated.reauthentication_token,
        ),
      },
    });
    expect(deleted.status()).toBe(204);
    expect((await user.client.get("/me")).status()).toBe(401);
  } finally {
    await user.client.dispose();
  }
});
