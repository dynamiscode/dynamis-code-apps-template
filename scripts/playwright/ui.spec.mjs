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

async function openItems(page) {
  await openWorkspaceHome(page);
  await page.locator(".workspace-cards a[href$='/items']").first().click();
  await expect(page.locator("#item-list")).toBeVisible();
}

async function injectAxe(page) {
  const url = new URL("/__test/axe.js", process.env.PLAYWRIGHT_BASE_URL).href;
  await page.route(url, (route) => route.fulfill({
    contentType: "application/javascript",
    body: axe.source,
  }));
  await page.addScriptTag({ url });
}

async function expectNoCriticalOrSeriousAxeViolations(page) {
  const results = await page.evaluate(async (source) => {
    eval(source);
    return axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"] } });
  }, axe.source);
  expect(results.violations.filter((violation) => ["critical", "serious"].includes(violation.impact)).map((item) => item.id)).toEqual([]);
}

const screenshots = [
  { name: "desktop-light", width: 1280, height: 900, colorScheme: "light" },
  { name: "desktop-dark", width: 1280, height: 900, colorScheme: "dark" },
  { name: "mobile-light", width: 390, height: 844, colorScheme: "light" },
  { name: "mobile-dark", width: 390, height: 844, colorScheme: "dark" },
];

test.describe("configured OIDC fixture", () => {
  test.use({ ignoreHTTPSErrors: true });

  for (const viewport of ["desktop", "mobile"]) {
    for (const theme of ["light", "dark"]) {
      for (const locale of ["en", "es"]) {
        test(`configured OIDC login and callback failure: ${viewport}-${theme}-${locale}`, async ({ page }) => {
          const width = viewport === "desktop" ? 1280 : 390;
          const oidcBaseURL = process.env.PLAYWRIGHT_OIDC_BASE_URL;
          await page.setViewportSize({ width, height: viewport === "desktop" ? 900 : 844 });
          await page.emulateMedia({ colorScheme: theme, reducedMotion: "reduce" });
          await page.context().addCookies([{ name: "theme", value: theme, url: oidcBaseURL }]);
          await page.goto(`${oidcBaseURL}/language?locale=${locale}&return_to=%2Flogin`);
          await page.goto(`${oidcBaseURL}/login`);
          await expect(page.locator("html")).toHaveAttribute("lang", locale);
          await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
          await page.addStyleTag({ content: "html { font-size: 12px; }" });
          const recoveryLink = page.locator('a[href$="password-reset"]');
          const loginTarget = await recoveryLink.boundingBox();
          expect(loginTarget.width).toBeGreaterThanOrEqual(24);
          expect(loginTarget.height).toBeGreaterThanOrEqual(24);
          const provider = page.getByRole("link", {
            name: locale === "es" ? "Iniciar sesión con Company SSO" : "Sign in with Company SSO",
          });
          await expect(provider).toHaveAttribute("href", "/auth/oidc/company");
          expect(new URL(await provider.getAttribute("href"), oidcBaseURL).origin).toBe(new URL(oidcBaseURL).origin);
          await page.keyboard.press("Tab");
          await expect(page.locator(".skip-link")).toBeFocused();
          for (let tab = 0; tab < 6; tab += 1) await page.keyboard.press("Tab");
          await expect(provider).toBeFocused();
          await expectNoCriticalOrSeriousAxeViolations(page);
          expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);

          await page.goto(`${oidcBaseURL}/auth/oidc/company/callback?error=access_denied`);
          await expect(page.locator("html")).toHaveAttribute("lang", locale);
          await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
          await expect(page.getByRole("alert")).toContainText(locale === "es" ? /no se pudo completar/i : /could not be completed/i);
          await expect(page.getByRole("alert")).toBeFocused();
          const callbackTarget = await page.locator('a[href$="password-reset"]').boundingBox();
          expect(callbackTarget.width).toBeGreaterThanOrEqual(24);
          expect(callbackTarget.height).toBeGreaterThanOrEqual(24);
          await expectNoCriticalOrSeriousAxeViolations(page);
          expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
        });
      }
    }
  }

  test("configured OIDC completes a verified callback, creates a session, and denies unrelated workspace access", async ({ page }) => {
    const oidcBaseURL = process.env.PLAYWRIGHT_OIDC_BASE_URL;
    await page.goto(`${oidcBaseURL}/login`);
    await page.getByLabel("Email").fill("oidc-fixture@example.com");
    await page.getByLabel("Password").fill("oidc-fixture-password-123");
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL(new URL("/", oidcBaseURL).href);
    const workspacePath = await page.locator(".cards a").first().getAttribute("href");
    expect(workspacePath).toMatch(/^\/workspaces\//);
    await page.context().clearCookies();
    await page.goto(`${oidcBaseURL}/login`);
    await page.getByRole("link", { name: "Sign in with Company SSO" }).click();
    await page.waitForURL(new URL("/", oidcBaseURL).href);
    expect((await page.context().cookies(oidcBaseURL)).some((cookie) => cookie.name === "session")).toBe(true);
    await expect(page.locator("#main")).toBeVisible();
    await expect(page.getByRole("heading").first()).toBeVisible();
    await expectNoCriticalOrSeriousAxeViolations(page);
    const denied = await page.goto(new URL(workspacePath, oidcBaseURL).href);
    expect(denied.status()).toBe(404);
  });

  test("unconfigured OIDC provider is unavailable", async ({ page }) => {
    const response = await page.goto(`${process.env.PLAYWRIGHT_OIDC_BASE_URL}/auth/oidc/missing`);
    expect(response.status()).toBe(404);
  });
});

test.describe("setup browser", () => {
  for (const viewport of ["desktop", "mobile"]) {
    for (const theme of ["light", "dark"]) {
      for (const locale of ["en", "es"]) {
        test(`initial setup is accessible: ${viewport}-${theme}-${locale}`, async ({ page }) => {
          const setupBaseURL = process.env.PLAYWRIGHT_SETUP_BASE_URL;
          const width = viewport === "desktop" ? 1280 : 390;
          await page.setViewportSize({ width, height: viewport === "desktop" ? 900 : 844 });
          await page.emulateMedia({ colorScheme: theme, reducedMotion: "reduce" });
          await page.context().addCookies([{ name: "theme", value: theme, url: setupBaseURL }]);
          await page.goto(`${setupBaseURL}/language?locale=${locale}&return_to=%2Fsetup`);
          await page.goto(`${setupBaseURL}/setup`);
          await expect(page.locator("html")).toHaveAttribute("lang", locale);
          await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
          await expect(page.locator('form[action="/setup"]')).toBeVisible();
          await expectNoCriticalOrSeriousAxeViolations(page);
          expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
        });
      }
    }
  }

  test("setup validates required fields, creates the first owner, then disables setup", async ({ page }) => {
    const setupBaseURL = process.env.PLAYWRIGHT_SETUP_BASE_URL;
    await page.goto(`${setupBaseURL}/setup`);
    await page.getByRole("button", { name: "Next" }).click();
    await expect(page.locator("#email")).toBeFocused();
    await page.getByLabel("Email").fill("setup-owner@example.com");
    await page.getByLabel("Password", { exact: true }).fill("setup-owner-password-123");
    await page.getByLabel("Confirm password").fill("setup-owner-password-123");
    await page.getByRole("button", { name: "Next" }).click();
    await expect(page.locator("#workspace")).toBeVisible();
    await page.locator("#workspace").fill("Setup smoke workspace");
    await page.locator('form[action="/setup"] button[type="submit"]').click();
    await page.waitForURL(`${setupBaseURL}/login`);
    await expect(page.locator('form[action="/login"]')).toBeVisible();
    const unavailable = await page.goto(`${setupBaseURL}/setup`);
    expect(unavailable.status()).toBe(404);
  });
});

for (const colorScheme of ["light", "dark"]) {
  test(`login system theme follows ${colorScheme} preference`, async ({ page }) => {
    await page.emulateMedia({ colorScheme });
    await page.goto("/login");
    await expect(page.locator("html")).toHaveAttribute("data-theme", "system");
    const canvas = await page.locator("html").evaluate((element) => getComputedStyle(element).getPropertyValue("--canvas").trim());
    expect(canvas).toBe(colorScheme === "dark" ? "#111826" : "#f3f6fa");
    await expectNoCriticalOrSeriousAxeViolations(page);
  });
}

test("invalid login preserves the form and exposes an accessible error", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Email").fill("missing-user@example.com");
  await page.getByLabel("Password").fill("invalid-login-password-123");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("alert")).toContainText("The email or password is invalid.");
  await expect(page.locator('form[action="/login"]')).toBeVisible();
  await expect(page.locator('input[name="csrf"]')).not.toHaveValue("");
  await expectNoCriticalOrSeriousAxeViolations(page);
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

for (const screenshot of screenshots) {
  test(`Items screenshot: ${screenshot.name}`, async ({ page }) => {
    await page.setViewportSize({ width: screenshot.width, height: screenshot.height });
    await page.emulateMedia({ colorScheme: screenshot.colorScheme, reducedMotion: "reduce" });
    await openItems(page);
    await expect(page.getByRole("heading", { name: "Items", level: 1 })).toBeVisible();
    const connected = await page.locator("#item-list").getAttribute("data-live-connected");
    await expect(page.locator("#realtime-status")).toHaveText(connected);
    await expect(page).toHaveScreenshot(`items-${screenshot.name}.png`, {
      animations: "disabled",
      fullPage: true,
    });
  });
}

test("workspace home opens Items and creates the first task", async ({ page }) => {
  await openItems(page);
  const create = async (title) => {
    await page.getByLabel("New item title").fill(title);
    await page.getByRole("button", { name: "Add item" }).click();
    await expect(page.locator(`#item-list .item-list-view input[value="${title}"]`)).toBeVisible();
  };
  await create("Playwright first item");
  await create("Playwright second item");

  const source = page.locator('#item-list tr[data-item-row]:has(input[name="title"][value="Playwright second item"])');
  const preserved = page.locator('#item-list tr[data-item-row]:has(input[name="title"][value="Playwright first item"])');
  const sourceTitle = await source.locator('input[name="title"]').inputValue();
  const preservedID = await preserved.getAttribute("data-item-row");
  await source.locator('input[name="title"]').fill("Playwright updated item");
  await preserved.locator('input[name="title"]').fill("Unsaved Playwright item");
  await source.getByRole("button", { name: `Save ${sourceTitle}` }).click();

  const updated = page.locator('#item-list tr[data-item-row]:has(input[name="title"][value="Playwright updated item"])');
  const stillDirty = page.locator('#item-list tr[data-item-row]:has(input[name="title"][value="Playwright first item"])');
  await expect(updated.locator('input[name="title"]')).toHaveValue("Playwright updated item");
  await expect(stillDirty).toHaveAttribute("data-item-row", preservedID);
  await expect(stillDirty.locator('input[name="title"]')).toHaveValue("Unsaved Playwright item");
  await expect(stillDirty.locator("form[data-item-edit]")).toHaveAttribute("data-dirty", "true");

  const invalidTitle = "x".repeat(201);
  const updatedTitle = updated.locator('input[name="title"]');
  await updatedTitle.evaluate((input) => input.removeAttribute("maxlength"));
  await updatedTitle.fill(invalidTitle);
  await updated.getByRole("button", { name: "Save Playwright updated item" }).click();
  await expect(page.getByRole("alert")).toContainText("item values are invalid");
  await expect(updatedTitle).toHaveValue(invalidTitle);

  await updated.locator('input[name="version"]').evaluate((input) => { input.value = "0"; });
  page.once("dialog", (dialog) => dialog.accept());
  await updated.getByRole("button", { name: "Delete permanently Playwright updated item" }).click();
  await expect(page.getByRole("alert")).toContainText("item version is invalid");
  await expect(updated).toBeVisible();

  await injectAxe(page);
  const results = await page.evaluate(async () => axe.run(document.querySelector("#item-list")));
  expect(results.violations).toEqual([]);
});

test("workspace home passes automated axe checks", async ({ page }) => {
  await openWorkspaceHome(page);
  await injectAxe(page);
  const results = await page.evaluate(async () => axe.run(document));
  expect(results.violations).toEqual([]);
});

test("Items sharing creates a read-only public link and revokes it", async ({ page }) => {
  await openItems(page);
  const title = "Playwright shared item";
  await page.getByLabel("New item title").fill(title);
  await page.getByRole("button", { name: "Add item" }).click();
  const row = page.locator(`#item-list tr[data-item-row]:has(input[name="title"][value="${title}"])`);
  await expect(row).toBeVisible();
  await row.locator("form.share button[value=create]").click();
  const link = page.locator("#item-list .success a[href*='/share/']");
  await expect(link).toBeVisible();
  const shareURL = await link.getAttribute("href");
  await page.goto(shareURL);
  await expect(page.getByRole("heading", { name: "Shared item" })).toBeVisible();
  await expect(page.locator("main.public-share-card")).toBeVisible();
  await expect(page.locator("main.public-share-card")).toContainText(title);
  await expect(page.locator("main.public-share-card")).not.toContainText("UI Baselines");

  await page.goBack();
  const activeRow = page.locator(`#item-list tr[data-item-row]:has(input[name="title"][value="${title}"])`);
  page.once("dialog", (dialog) => dialog.accept());
  await activeRow.locator("form.share button[value=revoke]").click();
  await page.goto(shareURL);
  await expect(page.getByRole("heading", { name: /unavailable|no disponible/i })).toBeVisible();
});

test("Items query navigation keeps native browser history", async ({ page }) => {
  await openItems(page);
  const basePath = new URL(page.url()).pathname;
  await page.locator("details.client-filters summary").click();
  await page.locator('input[name="search"]').fill("history marker");
  await page.locator("form.client-query button[type=submit]").click();
  await expect(page).toHaveURL(/search=history\+marker/);
  await expect(page.locator('input[name="search"]')).toHaveValue("history marker");

  await page.goBack();
  await expect(page).toHaveURL(new RegExp(`${basePath}$`));
  await expect(page.locator('input[name="search"]')).toHaveValue("");
  await page.goForward();
  await expect(page).toHaveURL(/search=history\+marker/);
  await expect(page.locator('input[name="search"]')).toHaveValue("history marker");
});

test("Items create form drops overlapping HTMX submissions", async ({ page }) => {
  await openItems(page);
  let posts = 0;
  let releaseRequest;
  const requestGate = new Promise((resolve) => { releaseRequest = resolve; });
  await page.route("**/workspaces/*/items", async (route) => {
    if (route.request().method() === "POST") {
      posts++;
      await requestGate;
    }
    await route.continue();
  });

  await page.evaluate(() => {
    const form = document.querySelector("form.create");
    const values = Object.fromEntries(new FormData(form));
    values.title = "One accepted submit";
    htmx.ajax("POST", form.action, { source: form, target: "#item-list", swap: "outerHTML", values });
    htmx.ajax("POST", form.action, { source: form, target: "#item-list", swap: "outerHTML", values });
  });
  await expect.poll(() => posts).toBe(1);
  releaseRequest();
  await expect(page.locator('#item-list input[name="title"][value="One accepted submit"]')).toBeVisible();
  expect(posts).toBe(1);
});

test("Items autosave updates version before a later delete", async ({ page }) => {
  await openItems(page);
  await page.getByLabel("New item title").fill("Autosave seed");
  await page.getByRole("button", { name: "Add item" }).click();
  const row = page.locator('#item-list tr[data-item-row]:has(input[name="title"][value="Autosave seed"])');
  await expect(row).toBeVisible();
  const createTitle = page.getByLabel("New item title");
  await createTitle.fill("   ");
  await createTitle.press("Enter");
  await expect(page.locator(".error-summary")).toBeFocused();
  await createTitle.fill("Unsaved create draft");
  const rowID = await row.getAttribute("data-item-row");
  const currentRow = page.locator(`#item-list tr[data-item-row="${rowID}"]`);
  await currentRow.locator('input[name="title"]').fill("Autosave updated");
  await currentRow.locator('select[name="status"]').evaluate((select) => {
    select.value = "complete";
    select.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await expect(currentRow.locator('input[name="version"]')).toHaveValue("2");
  await expect(currentRow.locator('input[name="title"]')).toHaveValue("Autosave updated");
  await expect(createTitle).toHaveValue("Unsaved create draft");
  await expect(page.locator(".error-summary")).toBeVisible();

  page.once("dialog", (dialog) => dialog.accept());
  await currentRow.getByRole("button", { name: "Delete permanently Autosave updated" }).click();
  await expect(currentRow).toHaveCount(0);
});

test("Spanish Items list passes axe", async ({ page }) => {
  await openItems(page);
  const path = new URL(page.url()).pathname;
  await page.goto(`/language?locale=es&return_to=${encodeURIComponent(path)}`);
  await expect(page.locator("html")).toHaveAttribute("lang", "es");
  await expect(page.getByRole("heading", { name: "Elementos", level: 1 })).toBeVisible();
  await expect(page.getByLabel("Título del nuevo elemento")).toBeVisible();
  const connected = (await page.locator("#item-list").getAttribute("data-live-connected")) ?? "";
  expect(connected).not.toBe("");
  await expect(page.locator("#realtime-status")).toHaveText(connected);

  await injectAxe(page);
  const results = await page.evaluate(async () => axe.run(document.querySelector("#item-list")));
  expect(results.violations).toEqual([]);
});

test("Items honors explicit and system theme preferences", async ({ page }) => {
  await openItems(page);
  const path = new URL(page.url()).pathname;
  const cases = [
    { theme: "light", colorScheme: "dark", canvas: "#f3f6fa" },
    { theme: "dark", colorScheme: "light", canvas: "#111826" },
    { theme: "system", colorScheme: "light", canvas: "#f3f6fa" },
    { theme: "system", colorScheme: "dark", canvas: "#111826" },
  ];

  for (const item of cases) {
    await page.goto("/account");
    const theme = page.locator("#account-theme");
    if (await theme.inputValue() !== item.theme) {
      await theme.selectOption(item.theme);
      await page.locator('form[action="/account/profile"] button[type=submit]').click();
      await expect(page).toHaveURL(/saved=1/);
    }
    await page.emulateMedia({ colorScheme: item.colorScheme });
    await page.goto(path);
    await expect(page.locator("html")).toHaveAttribute("data-theme", item.theme);
    const canvas = await page.locator("html").evaluate((element) => getComputedStyle(element).getPropertyValue("--canvas").trim());
    expect(canvas).toBe(item.canvas);
  }
});

test.describe("Items without JavaScript", () => {
  test.use({ javaScriptEnabled: false });

  test("creates and edits with ordinary forms", async ({ page }) => {
    await openItems(page);
    await page.getByLabel("New item title").fill("No JavaScript item");
    await page.getByRole("button", { name: "Add item" }).click();
    const item = page.locator('#item-list tr[data-item-row]:has(input[name="title"][value="No JavaScript item"])');
    await expect(item).toBeVisible();
    await item.locator('input[name="title"]').fill("No JavaScript edit");
    await item.getByRole("button", { name: "Save No JavaScript item" }).click();
    await expect(page.locator('#item-list input[name="title"][value="No JavaScript edit"]')).toBeVisible();
  });
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
