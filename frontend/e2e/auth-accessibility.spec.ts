import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import {
  loginInBrowser,
  verificationToken,
} from "./support";

test("registration, token scrubbing, login, and dashboard accessibility @browser-contract @mobile", async ({
  page,
}) => {
  const suffix = `${Date.now()}-${crypto.randomUUID().slice(0, 6)}`;
  const email = `browser-${suffix}@example.test`;
  const password = "E2ePassword123!";

  await page.goto("/register");
  await page.getByLabel("Display name").fill(`Browser ${suffix}`);
  await page.getByLabel("Email address").fill(email);
  await page.getByRole("textbox", { name: "Password" }).fill(password);
  await page.getByRole("button", { name: "Create Account" }).click();
  await expect(page).toHaveURL(/\/verification-pending/);

  const token = await verificationToken(email);
  await page.goto(`/verify-email?token=${encodeURIComponent(token)}`);
  await expect(page).toHaveURL(/\/verify-email$/);
  await expect(page.getByText("Email verified")).toBeVisible();

  // The verify-email response carries the session only via HttpOnly
  // cookie, never a token in the body — acceptSession must still treat
  // this as authenticated. Follow the page's own "Open dashboard" link
  // rather than navigating directly, so this exercises the same
  // isAuthenticated check ProtectedRoute makes, not just the cookie.
  await page.getByRole("link", { name: "Open dashboard" }).click();
  await expect(page).toHaveURL(/\/dashboard/);
  await expect(page.getByRole("main")).toBeVisible();

  await page.context().clearCookies();
  await loginInBrowser(page, email, password);
  await expect(page.getByRole("main")).toBeVisible();

  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth,
  );
  expect(overflow, "page must not horizontally overflow its viewport").toBe(false);

  const results = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
    // Contrast is tracked as a visual-design audit; keep this release gate
    // deterministic across browser color engines while enforcing structural,
    // naming, keyboard, landmark, and ARIA rules.
    .disableRules(["color-contrast"])
    .analyze();
  expect(
    results.violations.filter((violation) =>
      ["serious", "critical"].includes(violation.impact ?? ""),
    ),
  ).toEqual([]);
});
