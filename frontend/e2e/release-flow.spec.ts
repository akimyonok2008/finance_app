import { expect, test, type Page } from "@playwright/test";
import pg from "pg";

const apiURL = process.env.E2E_API_URL ?? "http://127.0.0.1:18090";
const mailpitURL =
  process.env.E2E_MAILPIT_URL ?? "http://127.0.0.1:18092";
const databaseURL =
  process.env.E2E_DATABASE_URL ??
  "postgres://postgres:postgres@127.0.0.1:15433/finance_app?sslmode=disable";

type JSONObject = Record<string, unknown>;
type ApiResult = { status: number; body: unknown };

async function apiRequest(
  page: Page,
  path: string,
  options: {
    token?: string;
    method?: string;
    body?: JSONObject;
    idempotencyKey?: string;
  } = {},
): Promise<ApiResult> {
  return page.evaluate(
    async ({ base, requestPath, token, method, body, idempotencyKey }) => {
      const response = await fetch(`${base}${requestPath}`, {
        method: method ?? "GET",
        headers: {
          ...(body ? { "Content-Type": "application/json" } : {}),
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
          ...(idempotencyKey
            ? { "Idempotency-Key": idempotencyKey }
            : {}),
        },
        body: body ? JSON.stringify(body) : undefined,
      });
      const text = await response.text();
      let parsed: unknown = null;
      if (text) {
        try {
          parsed = JSON.parse(text);
        } catch {
          parsed = text;
        }
      }
      return { status: response.status, body: parsed };
    },
    {
      base: apiURL,
      requestPath: path,
      token: options.token,
      method: options.method,
      body: options.body,
      idempotencyKey: options.idempotencyKey,
    },
  );
}

function object(value: unknown): JSONObject {
  expect(value).not.toBeNull();
  expect(typeof value).toBe("object");
  expect(Array.isArray(value)).toBe(false);
  return value as JSONObject;
}

async function expectApi(
  page: Page,
  path: string,
  expectedStatus: number,
  options: Parameters<typeof apiRequest>[2] = {},
): Promise<JSONObject> {
  const result = await apiRequest(page, path, options);
  expect(result.status, `${options.method ?? "GET"} ${path}`).toBe(
    expectedStatus,
  );
  return result.body === null ? {} : object(result.body);
}

async function verificationToken(email: string): Promise<string> {
  const deadline = Date.now() + 20_000;
  while (Date.now() < deadline) {
    const response = await fetch(`${mailpitURL}/api/v1/messages?limit=100`);
    if (response.ok) {
      const data = object(await response.json());
      const messages = Array.isArray(data.messages) ? data.messages : [];
      for (const candidate of messages) {
        const message = object(candidate);
        if (!JSON.stringify(message).includes(email)) continue;
        const detailResponse = await fetch(
          `${mailpitURL}/api/v1/message/${encodeURIComponent(String(message.ID))}`,
        );
        if (!detailResponse.ok) continue;
        const detail = await detailResponse.json();
        const match = JSON.stringify(detail).match(
          /verify-email\?token=([A-Za-z0-9._~-]+)/,
        );
        if (match) return match[1];
      }
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`verification email for ${email} did not arrive`);
}

async function createVerifiedUser(
  page: Page,
  email: string,
  displayName: string,
  password: string,
): Promise<{ token: string; user: JSONObject }> {
  await expectApi(page, "/auth/register", 201, {
    method: "POST",
    body: { email, display_name: displayName, password },
  });
  const token = await verificationToken(email);
  const session = await expectApi(page, "/auth/verify-email", 200, {
    method: "POST",
    body: { token },
  });
  return {
    token: String(session.token),
    user: object(session.user),
  };
}

test("release-critical account, investing, social, moderation, and deletion journey", async ({
  page,
}) => {
  const suffix = `${Date.now()}`;
  const password = "E2ePassword123!";
  const aliceEmail = `alice-${suffix}@example.test`;
  const bobEmail = `bob-${suffix}@example.test`;
  const moderatorEmail = `moderator-${suffix}@example.test`;
  const aliceName = `Alice ${suffix.slice(-6)}`;

  await test.step("register and verify email through the browser", async () => {
    await page.goto("/register");
    await page.getByLabel("Display name").fill(aliceName);
    await page.getByLabel("Email address").fill(aliceEmail);
    await page.getByRole("textbox", { name: "Password" }).fill(password);
    await page.getByRole("button", { name: "Create Account" }).click();
    await expect(page).toHaveURL(/\/verification-pending/);

    const token = await verificationToken(aliceEmail);
    await page.goto(`/verify-email?token=${encodeURIComponent(token)}`);
    await expect(page.getByText("Email verified")).toBeVisible();
  });

  await test.step("log in through the browser", async () => {
    await page.evaluate(() => localStorage.clear());
    await page.goto("/login");
    await page.getByLabel("Email address").fill(aliceEmail);
    await page.getByRole("textbox", { name: "Password" }).fill(password);
    await page.getByRole("button", { name: "Sign In" }).click();
    await expect(page).toHaveURL(/\/dashboard/);
  });

  const aliceToken = await page.evaluate(() =>
    String(localStorage.getItem("finance_app_token") ?? ""),
  );
  expect(aliceToken).not.toBe("");

  const bob = await createVerifiedUser(
    page,
    bobEmail,
    `Bob ${suffix.slice(-6)}`,
    password,
  );
  const moderator = await createVerifiedUser(
    page,
    moderatorEmail,
    `Moderator ${suffix.slice(-6)}`,
    password,
  );

  await test.step("deposit, buy, sell, and read the portfolio summary", async () => {
    await expectApi(page, "/portfolio/deposits", 201, {
      token: aliceToken,
      method: "POST",
      idempotencyKey: crypto.randomUUID(),
      body: { currency: "USD", amount: "5000" },
    });
    const buy = await expectApi(page, "/portfolio/buys", 201, {
      token: aliceToken,
      method: "POST",
      idempotencyKey: crypto.randomUUID(),
      body: { symbol: "AAPL", asset_type: "stock", quantity: "10" },
    });
    const position = object(buy.position);
    await expectApi(page, "/portfolio/sells", 201, {
      token: aliceToken,
      method: "POST",
      idempotencyKey: crypto.randomUUID(),
      body: { position_id: String(position.id), quantity: "2" },
    });
    const summary = await expectApi(page, "/portfolio/summary", 200, {
      token: aliceToken,
    });
    expect(summary).toHaveProperty("portfolio_index");

    await page.goto("/portfolio");
    await expect(page.getByText("AAPL").first()).toBeVisible();
  });

  await test.step("load the canonical leaderboard and join an active sprint", async () => {
    const leaderboard = await apiRequest(
      page,
      "/leaderboard?timeframe=ALL",
      { token: aliceToken },
    );
    expect(leaderboard.status).toBe(200);
    expect(Array.isArray(leaderboard.body)).toBe(true);
    expect(JSON.stringify(leaderboard.body)).toContain(aliceName);

    const competitions = await apiRequest(page, "/competitions", {
      token: aliceToken,
    });
    expect(competitions.status).toBe(200);
    expect(Array.isArray(competitions.body)).toBe(true);
    const sprint = (competitions.body as JSONObject[]).find(
      (item) => item.type === "weekly_sprint" && item.status === "active",
    );
    expect(sprint).toBeDefined();
    const joined = await expectApi(
      page,
      `/competitions/${String(sprint?.id)}/join`,
      200,
      { token: aliceToken, method: "POST" },
    );
    expect(joined.joined).toBe(true);

    await page.goto("/leaderboard");
    await expect(page.getByText(aliceName).first()).toBeVisible();
  });

  let reportID = "";
  await test.step("follow, message, block, and report", async () => {
    const aliceProfile = await expectApi(page, "/profiles/me", 200, {
      token: aliceToken,
    });
    const bobProfile = await expectApi(page, "/profiles/me", 200, {
      token: bob.token,
    });
    const aliceHandle = String(aliceProfile.handle);
    const bobHandle = String(bobProfile.handle);
    await expectApi(page, "/profiles/me", 200, {
      token: aliceToken,
      method: "PATCH",
      body: { is_public: true },
    });
    await expectApi(page, "/profiles/me", 200, {
      token: bob.token,
      method: "PATCH",
      body: { is_public: true },
    });

    await expectApi(
      page,
      `/social/follows/${encodeURIComponent(bobHandle)}`,
      200,
      { token: aliceToken, method: "POST" },
    );
    await expectApi(
      page,
      `/social/follows/${encodeURIComponent(aliceHandle)}`,
      200,
      { token: bob.token, method: "POST" },
    );
    const conversation = await expectApi(page, "/dm/conversations", 200, {
      token: aliceToken,
      method: "POST",
      body: { handle: bobHandle },
    });
    const conversationID = String(object(conversation.conversation).id);
    const sent = await expectApi(
      page,
      `/dm/conversations/${conversationID}/messages`,
      200,
      {
        token: bob.token,
        method: "POST",
        body: { body: `moderation evidence ${suffix}` },
      },
    );
    const messageID = String(object(sent.message).id);

    await expectApi(
      page,
      `/social/users/${encodeURIComponent(bobHandle)}/block`,
      200,
      { token: aliceToken, method: "POST" },
    );
    const report = await expectApi(
      page,
      `/reports/messages/${messageID}`,
      201,
      {
        token: aliceToken,
        method: "POST",
        body: {
          category: "harassment",
          explanation: `release-gate report ${suffix}`,
        },
      },
    );
    reportID = String(report.report_id);
    expect(reportID).not.toBe("");
  });

  await test.step("resolve the report in the moderator browser UI", async () => {
    const client = new pg.Client({ connectionString: databaseURL });
    await client.connect();
    try {
      await client.query("UPDATE users SET role = 'moderator' WHERE id = $1", [
        String(moderator.user.id),
      ]);
    } finally {
      await client.end();
    }

    await page.evaluate(() => localStorage.clear());
    await page.goto("/login");
    await page.getByLabel("Email address").fill(moderatorEmail);
    await page.getByRole("textbox", { name: "Password" }).fill(password);
    await page.getByRole("button", { name: "Sign In" }).click();
    await expect(page).toHaveURL(/\/dashboard/);
    await page.goto("/moderation/reports");
    await expect(
      page.getByRole("heading", { name: "Moderation queue" }),
    ).toBeVisible();
    await page.getByRole("button", { name: /harassment/ }).click();
    await expect(
      page.getByText(`moderation evidence ${suffix}`),
    ).toBeVisible();
    await page
      .getByLabel("Moderator notes (internal)")
      .fill(`browser E2E resolution ${suffix}`);
    await page.getByRole("button", { name: "Dismiss (no action)" }).click();
    await expect(page.getByText("Decision: no_action")).toBeVisible();

    const resolved = await expectApi(
      page,
      `/moderation/reports/${reportID}`,
      200,
      { token: moderator.token },
    );
    expect(resolved.status).toBe("resolved_no_action");
  });

  await test.step("delete the account and reject its old JWT", async () => {
    const reauthenticated = await expectApi(
      page,
      "/auth/reauthenticate",
      200,
      {
        token: aliceToken,
        method: "POST",
        body: { password },
      },
    );
    const deleted = await apiRequest(page, "/auth/delete-account", {
      token: aliceToken,
      method: "POST",
      body: {
        reauthentication_token: String(
          reauthenticated.reauthentication_token,
        ),
      },
    });
    expect(deleted.status).toBe(204);
    const staleSession = await apiRequest(page, "/me", { token: aliceToken });
    expect(staleSession.status).toBe(401);
  });
});
