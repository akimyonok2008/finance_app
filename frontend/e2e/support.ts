import {
  expect,
  request,
  type APIRequestContext,
  type APIResponse,
  type Page,
} from "@playwright/test";
import pg from "pg";

export const apiURL =
  process.env.E2E_API_URL ?? "http://127.0.0.1:18090";
const mailpitURL =
  process.env.E2E_MAILPIT_URL ?? "http://127.0.0.1:18092";
export const databaseURL =
  process.env.E2E_DATABASE_URL ??
  "postgres://postgres:postgres@127.0.0.1:15433/finance_app?sslmode=disable";

export type JSONObject = Record<string, unknown>;

export function object(value: unknown): JSONObject {
  expect(value).not.toBeNull();
  expect(typeof value).toBe("object");
  expect(Array.isArray(value)).toBe(false);
  return value as JSONObject;
}

export async function json(response: APIResponse): Promise<unknown> {
  const text = await response.text();
  return text ? JSON.parse(text) : null;
}

export async function expectStatus(
  response: APIResponse,
  status: number,
): Promise<JSONObject> {
  expect(response.status(), response.url()).toBe(status);
  const body = await json(response);
  return body === null ? {} : object(body);
}

export async function verificationToken(email: string): Promise<string> {
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
        const match = JSON.stringify(await detailResponse.json()).match(
          /verify-email\?token=([A-Za-z0-9._~-]+)/,
        );
        if (match) return match[1];
      }
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`verification email for ${email} did not arrive`);
}

export async function createVerifiedUser(label: string): Promise<{
  client: APIRequestContext;
  email: string;
  password: string;
  user: JSONObject;
}> {
  const suffix = `${label}-${Date.now()}-${crypto.randomUUID().slice(0, 8)}`;
  const email = `${suffix}@example.test`.toLowerCase();
  const password = "E2ePassword123!";
  const octet = (parseInt(crypto.randomUUID().slice(0, 4), 16) % 250) + 1;
  const client = await request.newContext({
    baseURL: apiURL,
    extraHTTPHeaders: { "X-Forwarded-For": `198.51.100.${octet}` },
  });
  await expectStatus(
    await client.post("/auth/register", {
      data: { email, display_name: label, password },
    }),
    201,
  );
  const token = await verificationToken(email);
  const session = await expectStatus(
    await client.post("/auth/verify-email", { data: { token } }),
    200,
  );
  return { client, email, password, user: object(session.user) };
}

export async function loginInBrowser(
  page: Page,
  email: string,
  password: string,
): Promise<void> {
  await page.goto("/login");
  await expect(page.getByRole("heading", { name: "Welcome back" })).toBeVisible();
  await page.waitForLoadState("networkidle");
  const emailInput = page.getByLabel("Email address");
  const passwordInput = page.getByRole("textbox", { name: "Password" });
  // AuthProvider may complete its initial /me probe just after the route
  // renders. Retry the controlled inputs if that one-time rerender clears them
  // (observed in Firefox/WebKit under CI load).
  for (let attempt = 0; attempt < 3; attempt += 1) {
    await emailInput.fill(email);
    await passwordInput.fill(password);
    if (
      (await emailInput.inputValue()) === email &&
      (await passwordInput.inputValue()) === password
    ) {
      break;
    }
  }
  await expect(emailInput).toHaveValue(email);
  await expect(passwordInput).toHaveValue(password);
  await page.getByRole("button", { name: "Sign In" }).click();
  await expect(page).toHaveURL(/\/dashboard/);
}

export async function withDatabase<T>(
  fn: (client: pg.Client) => Promise<T>,
): Promise<T> {
  const client = new pg.Client({ connectionString: databaseURL });
  await client.connect();
  try {
    return await fn(client);
  } finally {
    await client.end();
  }
}
