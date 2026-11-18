import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { setAccountTheme } from "./theme.mjs";

const require = createRequire(import.meta.url);
const axeSource = readFileSync(require.resolve("axe-core"), "utf8");

const email = process.env.PLAYWRIGHT_EMAIL;
const password = process.env.PLAYWRIGHT_PASSWORD;

async function signIn(page) {
  if (!email || !password) throw new Error("PLAYWRIGHT_EMAIL and PLAYWRIGHT_PASSWORD are required");
  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(new URL("/", process.env.PLAYWRIGHT_BASE_URL).href);
}

async function openClients(page) {
  await signIn(page);
  await page.locator(".cards a").first().click();
  await page.locator(".workspace-cards a[href$='/clients']").first().click();
}

test("Clients list adapts across desktop/mobile and light/dark themes", async ({ page }) => {
  await openClients(page);
  for (const colorScheme of ["light", "dark"]) {
    await page.emulateMedia({ colorScheme, reducedMotion: "reduce" });
    const restoreTheme = await setAccountTheme(page, colorScheme);
    try {
      await expect(page.locator("html")).toHaveAttribute("data-theme", colorScheme);
      const transitionDuration = await page.evaluate(() => {
        const wrapper = document.createElement("div");
        wrapper.className = "cards";
        const link = document.createElement("a");
        wrapper.append(link);
        document.body.append(wrapper);
        const duration = getComputedStyle(link).transitionDuration;
        wrapper.remove();
        return duration;
      });
      expect(transitionDuration.split(",").every((duration) => duration.trim() === "0s")).toBe(true);
      for (const width of [1280, 390]) {
        await page.setViewportSize({ width, height: 900 });
        const layout = await page.evaluate(() => ({
          table: getComputedStyle(document.querySelector(".list-view-table")).display,
          cards: getComputedStyle(document.querySelector(".list-view-cards")).display,
          canvas: getComputedStyle(document.documentElement).getPropertyValue("--canvas").trim(),
          overflow: document.documentElement.scrollWidth > document.documentElement.clientWidth,
        }));
        expect(layout.table).toBe(width <= 608 ? "none" : "block");
        expect(layout.cards).toBe(width <= 608 ? "grid" : "none");
        expect(layout.canvas).toBe(colorScheme === "dark" ? "#111826" : "#f3f6fa");
        expect(layout.overflow).toBe(false);
        await expect(page.locator("#realtime-status")).toContainText("Live updates connected");
        await expect(page).toHaveScreenshot(`clients-${colorScheme}-${width <= 608 ? "mobile" : "desktop"}.png`, { animations: "disabled", fullPage: true });
      }
    } finally {
      await restoreTheme();
    }
  }
});

test("Clients list renders, audits, and preserves a server 422 draft during SSE refresh", async ({ page }) => {
  await openClients(page);
  await expect(page.locator("#client-list")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Clients", level: 1 })).toBeVisible();
  const axeURL = new URL("/__test/axe.js", process.env.PLAYWRIGHT_BASE_URL).href;
  await page.route(axeURL, (route) => route.fulfill({ contentType: "application/javascript", body: axeSource }));
  await page.addScriptTag({ url: axeURL });
  const accessibility = await page.evaluate(async () => window.axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"] } }));
  expect(accessibility.violations.map(({ id, impact }) => `${id}:${impact}`)).toEqual([]);
  await page.locator("#client-create > details.client-create > summary").click();
  await expect(page.locator("#realtime-status")).toContainText(/connected|connecting/);
  await page.getByLabel("Display name").fill("Server draft survives refresh");
  await page.getByLabel("Email").fill("Ada <ada@example.com>");
  const invalidPost = page.waitForResponse((response) => response.request().method() === "POST" && new URL(response.url()).pathname.endsWith("/clients"));
  await page.locator("#client-create > details.client-create > form").evaluate((form) => HTMLFormElement.prototype.submit.call(form));
  expect((await invalidPost).status()).toBe(422);
  await expect(page.getByRole("alert")).toBeVisible();
  await expect(page.getByLabel("Email")).toHaveValue("Ada <ada@example.com>");
  await expect(page.locator("#client-create > details.client-create > form")).toHaveAttribute("data-dirty", "true");
  await expect(page.getByLabel("Display name")).toHaveValue("Server draft survives refresh");

  const peer = await page.context().newPage();
  try {
    await peer.goto(page.url());
    await peer.locator("#client-create > details.client-create > summary").click();
    await peer.getByLabel("Display name").fill("SSE refresh trigger");
    await peer.getByRole("button", { name: "Add client" }).click();
    await expect(peer).toHaveURL(/\/clients\/[^/?]+$/);
    await expect(page.locator("#realtime-status")).toContainText("New changes are waiting");
    await expect(page.getByLabel("Display name")).toHaveValue("Server draft survives refresh");
  } finally {
    await peer.close();
  }
});

test("Clients list follows Spanish workspace locale", async ({ page }) => {
  await openClients(page);
  const clientsURL = new URL(page.url());
  await page.goto(`/language?locale=es&return_to=${encodeURIComponent(clientsURL.pathname + clientsURL.search)}`);
  await expect(page.locator("html")).toHaveAttribute("lang", "es");
  await expect(page.getByRole("heading", { name: "Clientes", level: 1 })).toBeVisible();
  const connected = (await page.locator("#client-list").getAttribute("data-live-connected")) ?? "";
  expect(connected).not.toBe("");
  await expect(page.locator("#realtime-status")).toHaveText(connected);
  await page.locator("#client-create > details.client-create > summary").click();
  await expect(page.getByLabel("Nombre para mostrar")).toBeVisible();
});

test("multi-select draft survives blur and a list fragment replacement", async ({ page }) => {
  await openClients(page);
  await page.getByRole("link", { name: "Manage custom fields" }).click();
  await page.locator(".client-create > summary").click();
  await page.getByLabel("Field label").fill("Browser multi-select");
  await page.locator("#client-field-type").selectOption("single_choice");
  await page.getByRole("button", { name: "Add field" }).click();
  for (const label of ["First", "Second", "Third"]) {
    const optionForm = page.locator(".client-field-card").filter({ hasText: "Browser multi-select" }).locator(".option-create");
    await optionForm.locator('input[name="label"]').fill(label);
    await optionForm.getByRole("button", { name: "Add option" }).click();
  }
  await page.getByRole("link", { name: "Back to clients" }).click();
  await page.locator(".client-filters > summary").click();
  await page.locator("#client-filter-field").selectOption({ label: "Browser multi-select" });
  await page.locator(".client-query button[type=submit]").click();
  const options = page.locator("#client-filter-option");
  await expect(options).toBeVisible();
  await options.selectOption([{ label: "First" }, { label: "Third" }]);
  await page.locator("#client-sort").focus();
  const queryForm = page.locator(".client-query");
  await expect(queryForm).toHaveAttribute("data-dirty", "true");

  const peer = await page.context().newPage();
  try {
    await peer.goto(page.url());
    await peer.locator("#client-create > details.client-create > summary").click();
    await peer.getByLabel("Display name").fill("Multi-select refresh trigger");
    await peer.getByRole("button", { name: "Add client" }).click();
    await expect(peer).toHaveURL(/\/clients\/[^/?]+$/);
    await expect(page.locator("#realtime-status")).toContainText("New changes are waiting");
    await expect(options.locator("option:checked")).toHaveText(["First", "Third"]);
    await expect(queryForm).toHaveAttribute("data-dirty", "true");
  } finally {
    await peer.close();
  }
});

test("query-open filter details do not block realtime refresh", async ({ page }) => {
  await openClients(page);
  const clientsURL = new URL(page.url());
  clientsURL.searchParams.set("sort", "displayName");
  await page.goto(clientsURL.href);
  await expect(page.locator(".client-filters")).toHaveAttribute("open", "");
  await expect(page.locator("#realtime-status")).toContainText(/connected|connecting/);

  const peer = await page.context().newPage();
  try {
    await peer.goto(clientsURL.href);
    await peer.locator("#client-create > details.client-create > summary").click();
    await peer.getByLabel("Display name").fill("Query disclosure refresh trigger");
    await peer.getByRole("button", { name: "Add client" }).click();
    await expect(peer).toHaveURL(/\/clients\/[^/?]+$/);
    await expect(page.locator("#client-list tbody")).toContainText("Query disclosure refresh trigger");
    const connected = (await page.locator("#client-list").getAttribute("data-live-connected")) ?? "";
    expect(connected).not.toBe("");
    await expect(page.locator("#realtime-status")).toHaveText(connected);
  } finally {
    await peer.close();
  }
});

test.describe("Clients list without JavaScript", () => {
  test.use({ javaScriptEnabled: false });

  test("renders native controls and validation fallback", async ({ page }) => {
    await openClients(page);
    await expect(page.locator("#client-list")).toBeVisible();
    await page.locator("#client-create > details.client-create > summary").click();
    await page.getByLabel("Display name").fill("   ");
    await page.getByRole("button", { name: "Add client" }).click();
    await expect(page.getByRole("alert")).toBeVisible();
  });
});
