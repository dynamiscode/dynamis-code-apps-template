import { expect, test } from "@playwright/test";
import { execFileSync } from "node:child_process";

const email = process.env.PLAYWRIGHT_EMAIL;
const password = process.env.PLAYWRIGHT_PASSWORD;
const layouts = [
  { name: "desktop-light", width: 1280, height: 900, theme: "light" },
  { name: "desktop-dark", width: 1280, height: 900, theme: "dark" },
  { name: "mobile-light", width: 390, height: 844, theme: "light" },
  { name: "mobile-dark", width: 390, height: 844, theme: "dark" },
];

async function signIn(page) {
  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(new URL("/", process.env.PLAYWRIGHT_BASE_URL).href);
}

for (const layout of layouts) {
  test(`account and security pages render: ${layout.name}`, async ({ page }) => {
    test.setTimeout(60_000);
    await page.setViewportSize({ width: layout.width, height: layout.height });
    await page.emulateMedia({ colorScheme: layout.theme, reducedMotion: "reduce" });
    await page.context().addCookies([{ name: "theme", value: layout.theme, url: process.env.PLAYWRIGHT_BASE_URL }]);
    await signIn(page);
    await page.goto("/account");
    const profile = page.locator('form[action="/account/profile"]');
    const original = {
      csrf: await profile.locator('[name="csrf"]').inputValue(),
      timezone: await profile.locator('[name="timezone"]').inputValue(),
      locale: await profile.locator('[name="locale"]').inputValue(),
      display_name: await profile.locator('[name="display_name"]').inputValue(),
      theme: await profile.locator('[name="theme"]').inputValue(),
    };
    await profile.locator('[name="theme"]').selectOption(layout.theme);
    await profile.locator('[name="locale"]').selectOption("");
    await profile.getByRole("button", { name: /save/i }).click();
    await expect(page).toHaveURL(/saved=1/);
    await expect(page).toHaveScreenshot(`account-${layout.name}.png`, {
      animations: "disabled",
      fullPage: true,
      mask: [profile.locator('[name="display_name"]')],
    });

    try {
      for (const [path, heading] of [
      ["/account", "Account"],
      ["/settings/language", "Language"],
      ["/security", "Security"],
      ["/sessions", "Sessions"],
      ]) {
      const locale = path === "/settings/language" ? "es" : "en";
      await page.goto(`/language?locale=${locale}&return_to=${encodeURIComponent(path)}`);
      const response = await page.goto(path);
      expect(response?.ok()).toBeTruthy();
      await expect(page.locator("html")).toHaveAttribute("lang", locale);
      await expect(page.locator("html")).toHaveAttribute("data-theme", layout.theme);
      await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
      if (path === "/settings/language") {
        await expect(page.getByRole("heading", { level: 1 })).toContainText("Configuración de idioma");
      } else {
        await expect(page.getByRole("heading", { level: 1 })).toContainText(heading);
      }
      if (layout.width < 608) {
        const dimensions = await page.locator("html").evaluate((html) => [html.scrollWidth, html.clientWidth]);
        expect(dimensions[0]).toBeLessThanOrEqual(dimensions[1]);
      }
    }
    } finally {
      await page.request.post("/account/profile", { maxRedirects: 0, form: original });
    }
  });
}

test("profile preference persists through ordinary form submission", async ({ page }) => {
  await signIn(page);
  await page.goto("/account");
  const profile = page.locator('form[action="/account/profile"]');
  const locale = await profile.locator('[name="locale"]').inputValue();
  const timezone = await profile.locator('[name="timezone"]').inputValue();
  const displayName = await profile.locator('[name="display_name"]').inputValue();
  const theme = await profile.locator('[name="theme"]').inputValue();
  const csrf = await profile.locator('[name="csrf"]').inputValue();
  await profile.locator('[name="display_name"]').fill("Playwright profile");
  await profile.getByRole("button", { name: /save/i }).click();
  await expect(page).toHaveURL(/saved=1/);
  await expect(page.locator('form[action="/account/profile"] [name="display_name"]')).toHaveValue("Playwright profile");

  const response = await page.request.post("/account/profile", {
    maxRedirects: 0,
    form: { csrf, timezone, locale, display_name: displayName, theme },
  });
  expect(response.status()).toBe(303);
});

test.describe("account forms without JavaScript", () => {
  test.use({ javaScriptEnabled: false });

  test("profile saves and account deletion requires explicit confirmation", async ({ page }) => {
    await signIn(page);
    await page.goto("/account");
    const profile = page.locator('form[action="/account/profile"]');
    await profile.locator('[name="display_name"]').fill("No JavaScript profile");
    await profile.getByRole("button", { name: /save/i }).click();
    await expect(page).toHaveURL(/saved=1/);

    const deletion = page.locator('form[action="/account/delete"]');
    await expect(deletion.locator('[name="confirm_delete"]')).toHaveAttribute("required", "");
    await expect(deletion.locator('[name="password"]')).toHaveAttribute("required", "");
    const csrf = await deletion.locator('[name="csrf"]').inputValue();
    const response = await page.request.post("/account/delete", {
      maxRedirects: 0,
      form: { csrf, password },
    });
    expect(response.status()).toBe(422);
    expect(await response.text()).toContain("Confirm permanent account deletion to continue.");
  });
});

test("session can be revoked from its ordinary form", async ({ browser, page }) => {
  test.setTimeout(60_000);
  await signIn(page);
  const secondContext = await browser.newContext({ baseURL: process.env.PLAYWRIGHT_BASE_URL });
  try {
    const secondPage = await secondContext.newPage();
    await signIn(secondPage);
    await page.goto("/sessions");
    const revoke = page.locator('.session-card form[action^="/sessions/"]').filter({ has: page.getByRole("button", { name: "Revoke" }) }).first();
    await revoke.getByRole("button").click();
    await expect(page).toHaveURL(/revoked=1/);
    await expect(page.getByRole("status")).toContainText("Session revoked");
  } finally {
    await secondContext.close();
  }
});

test("expired session is rejected and shown in the session list", async ({ browser, page }) => {
  test.setTimeout(60_000);
  await signIn(page);
  const secondContext = await browser.newContext({ baseURL: process.env.PLAYWRIGHT_BASE_URL });
  try {
    const secondPage = await secondContext.newPage();
    await signIn(secondPage);
    const sql = `UPDATE sessions SET expires_at = strftime('%Y-%m-%dT%H:%M:%fZ','now','-1 minute')
      WHERE id = (
        SELECT s.id FROM sessions s JOIN users u ON u.id = s.user_id
        WHERE u.email = 'playwright@example.com' AND s.revoked_at IS NULL
        ORDER BY s.created_at DESC LIMIT 1
      );`;
    execFileSync("sqlite3", ["-batch", "-bail", process.env.PLAYWRIGHT_SQLITE_PATH, sql], {
      env: { PATH: process.env.PATH },
      stdio: "pipe",
    });

    await secondPage.goto("/account");
    await expect(secondPage).toHaveURL(/\/login(?:\?|$)/);
    await page.goto("/sessions");
    const expired = page.locator(".session-card").filter({ hasText: "Expired" });
    await expect(expired).toBeVisible();
    await expect(expired.locator('form[action^="/sessions/"]')).toHaveCount(0);
  } finally {
    await secondContext.close();
  }
});
