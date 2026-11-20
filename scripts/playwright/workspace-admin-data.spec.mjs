import { expect, test } from "@playwright/test";

const email = process.env.PLAYWRIGHT_EMAIL;
const password = process.env.PLAYWRIGHT_PASSWORD;

async function openAdminWorkspace(page, baseURL = process.env.PLAYWRIGHT_BASE_URL) {
  if (!email || !password) {
    throw new Error("PLAYWRIGHT_EMAIL and PLAYWRIGHT_PASSWORD are required");
  }
  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(new URL("/", baseURL).href);
  const workspaceLink = page
    .locator('.cards a[href^="/workspaces/"]')
    .filter({ hasText: "UI Baselines" })
    .first();
  await workspaceLink.click();
  await expect(page.locator("[data-workspace-home]")).toBeVisible();
  return new URL(page.url()).pathname;
}

test.describe("unconfigured webhook encryption", () => {
  test.use({ baseURL: process.env.PLAYWRIGHT_MEMBERS_BASE_URL });

  test("token secret appears once and webhook controls explain missing encryption setup", async ({ page }) => {
    const baseURL = process.env.PLAYWRIGHT_MEMBERS_BASE_URL;
    if (!baseURL) throw new Error("PLAYWRIGHT_MEMBERS_BASE_URL is required");
    const workspacePath = await openAdminWorkspace(page, baseURL);
    await page.goto(`${workspacePath}/settings/tokens`);
    const tokenName = `browser token ${Date.now()}`;
    await page.locator("#token-name").fill(tokenName);
    const createTokenButton = page.locator(
      'form[action$="/settings/tokens"] button[name="action"][value="create"]',
    );
    await createTokenButton.click();
    const secret = await page.locator(".success code").textContent();
    expect(secret).toBeTruthy();
    await page.goto(`${workspacePath}/settings/tokens`);
    await expect(page.locator(".settings-record").filter({ hasText: tokenName }).first()).toBeVisible();
    await expect(page.locator(".success code")).toHaveCount(0);
    await expect(page.locator("body")).not.toContainText(secret);

    await page.goto(`${workspacePath}/settings/webhooks`);
    await expect(page.locator("#webhook-secret-key-warning")).toContainText("ENCRYPTION_KEY");
    const createWebhookButton = page.locator(
      'form[data-persist="webhook-create"] button[value="create"]',
    );
    await expect(createWebhookButton).toBeDisabled();
  });
});

test("audit history renders its read-only route", async ({ page }) => {
  const workspacePath = await openAdminWorkspace(page);
  await page.goto(`${workspacePath}/settings/audit?all=1`);
  await expect(page.getByRole("heading", { level: 1, name: /Audit history|Historial de auditoría/i })).toBeVisible();
  await expect(page.locator("#audit-table")).toBeVisible();
  await expect(page.locator("#audit-table form")).toHaveCount(0);
});
