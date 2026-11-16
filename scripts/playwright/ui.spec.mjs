import { expect, test } from "@playwright/test";
import axe from "axe-core";

const email = process.env.PLAYWRIGHT_EMAIL;
const password = process.env.PLAYWRIGHT_PASSWORD;

async function openWorkspaceSelection(page) {
  if (!email || !password) {
    throw new Error("PLAYWRIGHT_EMAIL and PLAYWRIGHT_PASSWORD are required");
  }
  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(new URL("/", process.env.PLAYWRIGHT_BASE_URL).href);
}

async function openWorkspaceHome(page) {
  await openWorkspaceSelection(page);
  await page.locator(".cards a").first().click();
  await expect(page.locator("[data-workspace-home]")).toBeVisible();
}

const screenshots = [
  { name: "desktop-light", width: 1280, height: 900, colorScheme: "light" },
  { name: "desktop-dark", width: 1280, height: 900, colorScheme: "dark" },
  { name: "mobile-light", width: 390, height: 844, colorScheme: "light" },
  { name: "mobile-dark", width: 390, height: 844, colorScheme: "dark" },
];

test("configured OIDC choice and callback failure stay local and accessible", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.emulateMedia({ colorScheme: "dark", reducedMotion: "reduce" });
  await page.context().addCookies([{ name: "theme", value: "dark", url: process.env.PLAYWRIGHT_BASE_URL }]);
  await page.goto("/language?locale=es&return_to=%2Flogin");
  await page.route("**/login", (route) => route.fulfill({
    status: 200,
    contentType: "text/html; charset=utf-8",
    body: `<!doctype html><html lang="es" data-theme="dark"><head><title>Iniciar sesión</title></head><body><a class="skip-link" href="#main">Saltar al contenido</a><main id="main"><h1>Iniciar sesión</h1><section aria-labelledby="auth-provider-heading"><h2 id="auth-provider-heading">Otros métodos de inicio de sesión</h2><a class="btn" href="/auth/oidc/company">Iniciar sesión con Company SSO</a></section></main></body></html>`,
  }));
  await page.goto("/login");
  await expect(page.locator("html")).toHaveAttribute("lang", "es");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  const provider = page.getByRole("link", { name: "Iniciar sesión con Company SSO" });
  await expect(provider).toHaveAttribute("href", "/auth/oidc/company");
  expect(new URL(await provider.getAttribute("href"), process.env.PLAYWRIGHT_BASE_URL).origin)
    .toBe(new URL(process.env.PLAYWRIGHT_BASE_URL).origin);
  await page.keyboard.press("Tab");
  await expect(page.locator(".skip-link")).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(provider).toBeFocused();
  const results = await page.evaluate(async (source) => {
    eval(source);
    return axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa"] } });
  }, axe.source);
  expect(results.violations.filter((violation) => ["critical", "serious"].includes(violation.impact))).toEqual([]);

  await page.unrouteAll();
  await page.goto("/auth/oidc/company/callback?error=access_denied");
  await expect(page.locator("html")).toHaveAttribute("lang", "es");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await expect(page.getByRole("alert")).toContainText(/no se pudo completar/i);
  await expect(page.getByRole("alert")).toBeFocused();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
});

for (const viewport of ["desktop", "mobile"]) {
  for (const theme of ["light", "dark"]) {
    for (const locale of ["en", "es"]) {
      test(`login page accessibility: ${viewport}-${theme}-${locale}`, async ({ page }) => {
        const width = viewport === "desktop" ? 1280 : 390;
        await page.setViewportSize({ width, height: viewport === "desktop" ? 900 : 844 });
        await page.emulateMedia({ colorScheme: theme, reducedMotion: "reduce" });
        await page.context().addCookies([{ name: "theme", value: theme, url: process.env.PLAYWRIGHT_BASE_URL }]);
        await page.goto(`/language?locale=${locale}&return_to=%2Flogin`);
        await page.goto("/login");
        await expect(page.locator("html")).toHaveAttribute("lang", locale);
        await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
        await expect(page.locator('form[action="/login"]')).toBeVisible();
        await page.keyboard.press("Tab");
        await expect(page.locator(".skip-link")).toBeFocused();
        const results = await page.evaluate(async (source) => {
          eval(source);
          return axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa"] } });
        }, axe.source);
        expect(results.violations.filter((violation) => ["critical", "serious"].includes(violation.impact)).map((item) => item.id)).toEqual([]);
        expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
      });
    }
  }
}

for (const screenshot of screenshots) {
  test(`workspace selection screenshot: ${screenshot.name}`, async ({ page }) => {
    await page.setViewportSize({ width: screenshot.width, height: screenshot.height });
    await page.emulateMedia({ colorScheme: screenshot.colorScheme, reducedMotion: "reduce" });
    await openWorkspaceSelection(page);
    await expect(page.getByRole("heading", { name: "Workspaces" })).toBeVisible();
    await expect(page).toHaveScreenshot(`workspace-selection-${screenshot.name}.png`, {
      animations: "disabled",
      fullPage: true,
    });
  });

  test(`workspace home screenshot: ${screenshot.name}`, async ({ page }) => {
    await page.setViewportSize({ width: screenshot.width, height: screenshot.height });
    await page.emulateMedia({ colorScheme: screenshot.colorScheme, reducedMotion: "reduce" });
    await openWorkspaceHome(page);
    if (screenshot.width > 608) {
      const sidebar = await page.locator(".sidebar").boundingBox();
      expect(sidebar.y).toBe(0);
      expect(sidebar.height).toBeGreaterThanOrEqual(screenshot.height);
    }
    const canvas = await page.locator("html").evaluate((element) => getComputedStyle(element).getPropertyValue("--canvas").trim());
    expect(canvas).toBe(screenshot.colorScheme === "dark" ? "#111826" : "#f3f6fa");
    await expect(page).toHaveScreenshot(`workspace-home-${screenshot.name}.png`, {
      animations: "disabled",
      fullPage: true,
    });
  });
}

test("workspace home opens Items and creates the first task", async ({ page }) => {
  await openWorkspaceHome(page);
  await page.locator(".workspace-cards a[href$='/items']").first().click();
  await expect(page.locator("#item-list")).toBeVisible();
  const title = "Playwright journey item";
  await page.getByLabel("New item title").fill(title);
  await page.getByRole("button", { name: "Add item" }).click();
  await expect(page.locator(`#item-list .item-list-view input[value="${title}"]`)).toBeVisible();
});

test.describe("workspace selection without JavaScript", () => {
  test.use({ javaScriptEnabled: false });

  test("shows validation and creates a workspace in Spanish", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await openWorkspaceSelection(page);
    await page.goto("/language?locale=es&return_to=%2F");
    await expect(page.locator("html")).toHaveAttribute("lang", "es");
    await page.locator("#workspace-name").fill("   ");
    await page.locator('form[action="/workspaces"] button').click();
    await expect(page.getByRole("alert")).toBeVisible();
    await page.locator("#workspace-name").fill("Espacio de prueba");
    await page.locator('form[action="/workspaces"] button').focus();
    await page.keyboard.press("Enter");
    await expect(page.locator(".cards a", { hasText: "Espacio de prueba" })).toBeVisible();
  });
});
