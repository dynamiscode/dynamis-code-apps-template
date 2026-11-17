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
