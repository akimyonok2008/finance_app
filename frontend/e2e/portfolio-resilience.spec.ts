import { expect, test } from "@playwright/test";
import {
  apiURL,
  createVerifiedUser,
  expectStatus,
  json,
  loginInBrowser,
  object,
} from "./support";

test("portfolio mutations and reads are isolated", async ({ page }) => {
  const user = await createVerifiedUser("Portfolio");
  try {
    await expectStatus(
      await user.client.post("/portfolio/deposits", {
        headers: { "Idempotency-Key": crypto.randomUUID() },
        data: { currency: "USD", amount: "5000" },
      }),
      201,
    );
    const buy = await expectStatus(
      await user.client.post("/portfolio/buys", {
        headers: { "Idempotency-Key": crypto.randomUUID() },
        data: { symbol: "AAPL", asset_type: "stock", quantity: "10" },
      }),
      201,
    );
    const position = object(buy.position);
    await expectStatus(
      await user.client.post("/portfolio/sells", {
        headers: { "Idempotency-Key": crypto.randomUUID() },
        data: { position_id: String(position.id), quantity: "2" },
      }),
      201,
    );
    const summary = await expectStatus(
      await user.client.get("/portfolio/summary"),
      200,
    );
    expect(summary).toHaveProperty("portfolio_index");

    await loginInBrowser(page, user.email, user.password);
    await page.goto("/portfolio");
    await expect(page.getByText("AAPL").first()).toBeVisible();
  } finally {
    await user.client.dispose();
  }
});

test("idempotent retries apply once and conflicting payloads are rejected", async () => {
  const user = await createVerifiedUser("Idempotency");
  try {
    const key = crypto.randomUUID();
    const body = { currency: "USD", amount: "25" };
    expect(
      (
        await Promise.all([
          user.client.post("/portfolio/deposits", {
            headers: { "Idempotency-Key": key },
            data: body,
          }),
          user.client.post("/portfolio/deposits", {
            headers: { "Idempotency-Key": key },
            data: body,
          }),
        ])
      ).every((response) => [200, 201].includes(response.status())),
    ).toBe(true);

    const conflict = await user.client.post("/portfolio/deposits", {
      headers: { "Idempotency-Key": key },
      data: { currency: "USD", amount: "26" },
    });
    expect(conflict.status()).toBe(409);
  } finally {
    await user.client.dispose();
  }
});

test("concurrent sells cannot oversell a position", async () => {
  const user = await createVerifiedUser("Concurrency");
  try {
    const buy = await expectStatus(
      await user.client.post("/portfolio/buys", {
        headers: { "Idempotency-Key": crypto.randomUUID() },
        data: { symbol: "MSFT", asset_type: "stock", quantity: "10" },
      }),
      201,
    );
    const positionID = String(object(buy.position).id);
    const responses = await Promise.all([
      user.client.post("/portfolio/sells", {
        headers: { "Idempotency-Key": crypto.randomUUID() },
        data: { position_id: positionID, quantity: "7" },
      }),
      user.client.post("/portfolio/sells", {
        headers: { "Idempotency-Key": crypto.randomUUID() },
        data: { position_id: positionID, quantity: "7" },
      }),
    ]);
    expect(responses.filter((response) => response.status() === 201)).toHaveLength(1);
    expect(
      responses.filter((response) => [400, 409, 422].includes(response.status())),
    ).toHaveLength(1);
  } finally {
    await user.client.dispose();
  }
});

test("portfolio UI recovers after an interrupted summary request", async ({
  page,
}) => {
  const user = await createVerifiedUser("Network");
  try {
    await loginInBrowser(page, user.email, user.password);
    let interrupted = false;
    await page.route(`${apiURL}/portfolio/summary`, async (route) => {
      if (!interrupted) {
        interrupted = true;
        await route.abort("connectionreset");
        return;
      }
      await route.continue();
    });
    await page.goto("/portfolio");
    await expect.poll(() => interrupted).toBe(true);
    await page.reload();
    await expect(page.getByRole("main")).toBeVisible();
    const response = await page.request.get(`${apiURL}/portfolio/summary`);
    expect(response.status()).toBe(200);
    expect(object(await json(response))).toHaveProperty("portfolio_index");
  } finally {
    await user.client.dispose();
  }
});
