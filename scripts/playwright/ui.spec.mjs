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

const screenshots = [
  { name: "desktop-light", width: 1280, height: 900, colorScheme: "light" },
  { name: "desktop-dark", width: 1280, height: 900, colorScheme: "dark" },
  { name: "mobile-light", width: 390, height: 844, colorScheme: "light" },
  { name: "mobile-dark", width: 390, height: 844, colorScheme: "dark" },
];

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
        const results = await page.evaluate(async (source) => {
          eval(source);
          return axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa"] } });
        }, axe.source);
        expect(results.violations.filter((violation) => ["critical", "serious"].includes(violation.impact)).map((item) => item.id)).toEqual([]);
        expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);

        await page.goto(`${oidcBaseURL}/auth/oidc/company/callback?error=access_denied`);
        await expect(page.locator("html")).toHaveAttribute("lang", locale);
        await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
        await expect(page.getByRole("alert")).toContainText(locale === "es" ? /no se pudo completar/i : /could not be completed/i);
        await expect(page.getByRole("alert")).toBeFocused();
        const callbackTarget = await page.locator('a[href$="password-reset"]').boundingBox();
        expect(callbackTarget.width).toBeGreaterThanOrEqual(24);
        expect(callbackTarget.height).toBeGreaterThanOrEqual(24);
        expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
      });
    }
  }
}

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
