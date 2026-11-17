import axe from "axe-core";
import { expect, test } from "@playwright/test";

const email = process.env.PLAYWRIGHT_EMAIL;
const password = process.env.PLAYWRIGHT_PASSWORD;
const screenshots = [
  { name: "desktop-light", width: 1280, height: 900, colorScheme: "light" },
  { name: "desktop-dark", width: 1280, height: 900, colorScheme: "dark" },
  { name: "mobile-light", width: 390, height: 844, colorScheme: "light" },
  { name: "mobile-dark", width: 390, height: 844, colorScheme: "dark" },
];

async function openWorkspace(page) {
  if (!email || !password) {
    throw new Error("PLAYWRIGHT_EMAIL and PLAYWRIGHT_PASSWORD are required");
  }
  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(new URL("/", process.env.PLAYWRIGHT_BASE_URL).href);
  await page.locator(".cards a").first().click();
  await expect(page.locator("[data-workspace-home]")).toBeVisible();
  return new URL(page.url()).pathname;
}

async function openSettings(page, section) {
  const workspacePath = await openWorkspace(page);
  const response = await page.goto(`${workspacePath}/settings/${section}`);
  expect(response?.ok()).toBeTruthy();
  return workspacePath;
}

async function setThemePreference(page, workspacePath, theme) {
  await page.goto("/account");
  const profile = page.locator('form[action="/account/profile"]');
  const csrf = await profile.locator('input[name="csrf"]').inputValue();
  const timezone = await profile.locator('[name="timezone"]').inputValue();
  const locale = await profile.locator('[name="locale"]').inputValue();
  const displayName = await profile.locator('[name="display_name"]').inputValue();
  const previousTheme = await profile.locator('[name="theme"]').inputValue();
  const response = await page.request.post("/account/profile", {
    maxRedirects: 0,
    form: { csrf, timezone, locale, display_name: displayName, theme },
  });
  expect(response.status()).toBe(303);
  return {
    theme,
    restore: async () => {
      const restore = await page.request.post("/account/profile", {
        maxRedirects: 0,
        form: { csrf, timezone, locale, display_name: displayName, theme: previousTheme },
      });
      expect(restore.status()).toBe(303);
    },
  };
}

for (const screenshot of screenshots) {
  for (const section of ["general", "email", "tokens", "provisioning"]) {
    test(`${section} settings screenshot: ${screenshot.name}`, async ({ page }) => {
      await page.setViewportSize({ width: screenshot.width, height: screenshot.height });
      await page.emulateMedia({ colorScheme: screenshot.colorScheme, reducedMotion: "reduce" });
      const workspacePath = await openWorkspace(page);
      const { theme, restore } = await setThemePreference(page, workspacePath, screenshot.colorScheme);
      try {
        const response = await page.goto(`${workspacePath}/settings/${section}`);
        expect(response?.headers()["cache-control"]).toBe("no-store");
        await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
        await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
        if (section === "tokens") await expect(page.getByText("Copy this token now", { exact: false })).toHaveCount(0);
        if (section === "provisioning") await expect(page.getByText("Copy this credential now", { exact: false })).toHaveCount(0);
        const canvas = await page.locator("html").evaluate((element) => getComputedStyle(element).getPropertyValue("--canvas").trim());
        expect(canvas).toBe(screenshot.colorScheme === "dark" ? "#111826" : "#f3f6fa");
        const platformSuffix = ["email", "provisioning"].includes(section) && screenshot.width < 608 && process.platform === "linux" ? "-linux" : "";
        await expect(page).toHaveScreenshot(`${section}-settings-${screenshot.name}${platformSuffix}.png`, {
          animations: "disabled",
          fullPage: true,
        });
      } finally {
        await restore();
      }
    });
  }
}

test("workspace settings are keyboard reachable and credential pages pass axe", async ({ page }) => {
  const workspacePath = await openWorkspace(page);
  await page.goto(`${workspacePath}/settings/general`);
  await page.keyboard.press("Tab");
  await expect(page.locator(".skip-link")).toBeFocused();
  await page.locator("#workspace-locale").focus();
  await expect(page.locator("#workspace-locale")).toBeFocused();

  await page.goto(`${workspacePath}/settings/email`);
  await page.keyboard.press("Tab");
  await expect(page.locator(".skip-link")).toBeFocused();
  await page.locator("#email-provider").focus();
  await page.keyboard.press("Tab");
  await expect(page.locator("#email-sender-name")).toBeFocused();

  await page.goto(`${workspacePath}/settings/tokens`);
  await page.keyboard.press("Tab");
  await expect(page.locator(".skip-link")).toBeFocused();
  await page.locator("#token-name").focus();
  await expect(page.locator("#token-name")).toBeFocused();

  await page.goto(`${workspacePath}/settings/provisioning`);
  await page.keyboard.press("Tab");
  await expect(page.locator(".skip-link")).toBeFocused();
  await page.getByRole("button", { name: "Create or rotate credential" }).focus();
  await expect(page.getByRole("button", { name: "Create or rotate credential" })).toBeFocused();

  await page.route("**/__test/axe.js", (route) => route.fulfill({
    contentType: "application/javascript",
    body: axe.source,
  }));
  for (const section of ["general", "email", "tokens", "provisioning"]) {
    await page.goto(`${workspacePath}/settings/${section}`);
    await page.addScriptTag({ url: new URL("/__test/axe.js", process.env.PLAYWRIGHT_BASE_URL).href });
    const violations = await page.evaluate(async () => {
      const results = await window.axe.run(document, {
        runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"] },
      });
      return results.violations.map(({ id, impact, nodes }) => ({ id, impact, targets: nodes.map(({ target }) => target) }));
    });
    expect(violations, `${section} settings axe violations`).toEqual([]);
  }
});

test("settings pages render in Spanish", async ({ page }) => {
  test.setTimeout(30_000);
  const workspacePath = await openWorkspace(page);

  await page.goto(`/language?locale=es&return_to=${encodeURIComponent(workspacePath)}`);
  await page.goto(`${workspacePath}/settings/general`);
  await expect(page.locator("html")).toHaveAttribute("lang", "es");
  await expect(page.getByRole("heading", { level: 1, name: "Configuración general del espacio de trabajo" })).toBeVisible();
  await expect(page.locator('form[action$="/settings/general"][method="post"] input[name="csrf"]')).not.toHaveValue("");

  await page.goto(`${workspacePath}/settings/email`);
  await expect(page.locator("html")).toHaveAttribute("lang", "es");
  await expect(page.getByRole("heading", { level: 1, name: "Entrega de correo" })).toBeVisible();
  await expect(page.locator('form[action$="/settings/email"][method="post"] input[name="csrf"]')).not.toHaveValue("");
  await expect(page.locator('input[type="password"]')).toHaveCount(4);

  await page.goto(`${workspacePath}/settings/tokens`);
  await expect(page.locator("html")).toHaveAttribute("lang", "es");
  await expect(page.getByRole("heading", { level: 1, name: "Tokens de API" })).toBeVisible();
  await expect(page.locator('form[action$="/settings/tokens"][method="post"] input[name="csrf"]')).not.toHaveValue("");
  await expect(page.getByText("Copia este token ahora", { exact: false })).toHaveCount(0);

  await page.goto(`${workspacePath}/settings/provisioning`);
  await expect(page.locator("html")).toHaveAttribute("lang", "es");
  await expect(page.getByRole("heading", { level: 1, name: "Aprovisionamiento SCIM" })).toBeVisible();
  await expect(page.locator('form[action$="/settings/provisioning"][method="post"] input[name="csrf"]')).toHaveCount(2);
  await expect(page.getByText("Copia esta credencial ahora", { exact: false })).toHaveCount(0);
});

test("token and provisioning settings reflow at 320 CSS pixels", async ({ page }) => {
  const workspacePath = await openWorkspace(page);
  await page.setViewportSize({ width: 320, height: 800 });
  for (const section of ["tokens", "provisioning"]) {
    await page.goto(`${workspacePath}/settings/${section}`);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    const width = await page.locator("html").evaluate((element) => ({
      document: element.scrollWidth,
      viewport: element.clientWidth,
    }));
    expect(width.document, `${section} document width`).toBeLessThanOrEqual(width.viewport);
  }
});

test("member settings permissions keep email hidden and general read-only", async ({ browser }) => {
  const memberEmail = process.env.PLAYWRIGHT_MEMBER_EMAIL;
  const memberPassword = process.env.PLAYWRIGHT_MEMBER_PASSWORD;
  if (!memberEmail || !memberPassword) {
    throw new Error("PLAYWRIGHT_MEMBER_EMAIL and PLAYWRIGHT_MEMBER_PASSWORD are required");
  }
  const page = await browser.newPage({ baseURL: process.env.PLAYWRIGHT_BASE_URL });
  try {
    await page.goto("/login");
    await page.getByLabel("Email").fill(memberEmail);
    await page.getByLabel("Password").fill(memberPassword);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL(new URL("/", process.env.PLAYWRIGHT_BASE_URL).href);
    await page.locator(".cards a").first().click();
    const workspacePath = new URL(page.url()).pathname;

    await page.goto(`${workspacePath}/settings/general`);
    await expect(page.locator('a[href$="/settings/email"]')).toHaveCount(0);
    await expect(page.locator("#workspace-locale")).toBeDisabled();
    await expect(page.locator('form[action$="/settings/general"] button[type="submit"]')).toHaveCount(0);
    const emailResponse = await page.goto(`${workspacePath}/settings/email`);
    expect(emailResponse?.status()).toBe(403);
  } finally {
    await page.close();
  }
});

test.describe("settings forms without JavaScript", () => {
  test.use({ javaScriptEnabled: false });

  test("general settings submit and email validation render without JavaScript", async ({ page }) => {
    const workspacePath = await openWorkspace(page);
    await page.goto(`${workspacePath}/settings/general`);
    const generalForm = page.locator('form[action$="/settings/general"][method="post"]');
    await expect(generalForm.locator('input[name="csrf"]')).not.toHaveValue("");
    await Promise.all([
      page.waitForURL("**/settings/general?saved=1"),
      generalForm.locator('button[type="submit"]').click(),
    ]);
    await expect(page.getByRole("status")).toBeVisible();

    await page.goto(`${workspacePath}/settings/email`);
    const emailForm = page.locator('form[action$="/settings/email"][method="post"]');
    await expect(emailForm.locator('input[name="csrf"]')).not.toHaveValue("");
    await expect(page.locator('fieldset[data-show-when="email-provider=smtp"]')).toBeVisible();
    await expect(page.locator('fieldset[data-show-when="email-provider=resend"]')).toBeVisible();
    await expect(page.locator('fieldset[data-show-when="email-provider=postmark"]')).toBeVisible();
    await emailForm.locator('[name="provider"]').selectOption("smtp");
    await emailForm.locator('[name="sender_address"]').fill("invalid@example.com");
    await emailForm.locator('[name="host"]').fill("");
    await emailForm.locator('[name="port"]').fill("587");
    const response = await Promise.all([
      page.waitForResponse((candidate) => candidate.url().includes("/settings/email") && candidate.request().method() === "POST"),
      emailForm.locator('button[type="submit"]').click(),
    ]);
    expect(response[0].status()).toBe(422);
    await expect(page.getByRole("alert")).toContainText("The email settings are invalid.");

    await page.goto(`${workspacePath}/settings/tokens`);
    const tokenForm = page.locator('form[action$="/settings/tokens"][method="post"]');
    await expect(tokenForm.locator('input[name="csrf"]')).not.toHaveValue("");
    await expect(tokenForm.locator('input[name="name"]')).toBeVisible();

    await page.goto(`${workspacePath}/settings/provisioning`);
    const provisioningForms = page.locator('form[action$="/settings/provisioning"][method="post"]');
    await expect(provisioningForms).toHaveCount(2);
    await expect(provisioningForms.nth(0).locator('input[name="csrf"]')).not.toHaveValue("");
    await expect(provisioningForms.nth(1).locator('input[name="csrf"]')).not.toHaveValue("");
  });
});
