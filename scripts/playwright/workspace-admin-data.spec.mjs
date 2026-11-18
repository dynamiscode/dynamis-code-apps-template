import { expect, test } from "@playwright/test";

const email = process.env.PLAYWRIGHT_EMAIL;
const password = process.env.PLAYWRIGHT_PASSWORD;

async function openAdminWorkspace(page) {
  if (!email || !password) {
    throw new Error("PLAYWRIGHT_EMAIL and PLAYWRIGHT_PASSWORD are required");
  }
  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(new URL("/", process.env.PLAYWRIGHT_BASE_URL).href);
  const workspaceLink = page
    .locator('.cards a[href^="/workspaces/"]')
    .filter({ hasText: "UI Baselines" })
    .first();
  await workspaceLink.click();
  await expect(page.locator("[data-workspace-home]")).toBeVisible();
  return new URL(page.url()).pathname;
}

async function downloadWorkspaceExport(page, workspacePath) {
  const response = await page.request.get(`${workspacePath}/settings/export/download`);
  expect(response.status()).toBe(200);
  expect(response.headers()["cache-control"]).toContain("no-store");
  expect(response.headers()["content-type"]).toContain("application/json");
  return response.json();
}

test.describe("workspace administration and data", () => {
  test.use({ javaScriptEnabled: false });

  test("export, preview, reject invalid input, and confirm import with ordinary forms", async ({ page }) => {
    const workspacePath = await openAdminWorkspace(page);
    const before = await downloadWorkspaceExport(page, workspacePath);
    expect(before.formatVersion).toBe("dynamis-code.workspace/v1");

    await page.goto(`${workspacePath}/settings/import`);
    const privateTitle = `Do not echo ${Date.now()}`;
    await page.locator("#import-file").setInputFiles({
      name: "invalid.csv",
      mimeType: "text/csv",
      buffer: Buffer.from(`title,status\n${privateTitle},unknown\n`),
    });
    await page.locator('form.import-preview-form button[type="submit"]').click();
    await expect(page.locator(".import-preview-errors")).toContainText(/Record 1|Registro 1/i);
    await expect(page.locator(".import-preview-errors")).not.toContainText(privateTitle);

    const title = `Browser import ${Date.now()}`;
    const csv = `title,status\n${title},active\n`;
    await page.locator("#import-file").setInputFiles({
      name: "items.csv",
      mimeType: "text/csv",
      buffer: Buffer.from(csv),
    });
    await page.locator('form.import-preview-form button[type="submit"]').click();
    await expect(page.locator("#import-preview-heading")).toBeVisible();
    await expect(page.locator(".import-preview")).toContainText(title);
    await expect(page.locator(".import-confirm-form")).toBeVisible();
    const previewExport = await downloadWorkspaceExport(page, workspacePath);
    expect(previewExport.items.some((item) => item.title === title)).toBe(false);

    await page.locator("#import-confirm-file").setInputFiles({
      name: "items.csv",
      mimeType: "text/csv",
      buffer: Buffer.from(csv),
    });
    await page.locator("#import-confirm").check();
    await page.locator('.import-confirm-form button[type="submit"]').click();
    await expect(page).toHaveURL(/\/settings\/import\?imported=1$/);
    await expect(page.getByRole("status")).toContainText("1");
    const importedExport = await downloadWorkspaceExport(page, workspacePath);
    expect(importedExport.items.some((item) => item.title === title)).toBe(true);
  });
});

test("token secret appears once and webhook controls explain missing encryption setup", async ({ page }) => {
  const workspacePath = await openAdminWorkspace(page);
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

test("audit history renders its read-only route", async ({ page }) => {
  const workspacePath = await openAdminWorkspace(page);
  await page.goto(`${workspacePath}/settings/audit?all=1`);
  await expect(page.getByRole("heading", { level: 1, name: /Audit history|Historial de auditoría/i })).toBeVisible();
  await expect(page.locator("#audit-table")).toBeVisible();
  await expect(page.locator("#audit-table form")).toHaveCount(0);
});
