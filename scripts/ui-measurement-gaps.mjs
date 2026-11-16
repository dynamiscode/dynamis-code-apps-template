import assert from "node:assert/strict";
import { performance } from "node:perf_hooks";
import { chromium } from "playwright";
import { observeNetwork } from "./ui-measurement-network.mjs";

const browser = await chromium.launch({ channel: "chrome" });
try {
  const context = await browser.newContext({ baseURL: process.env.PLAYWRIGHT_BASE_URL,
    viewport: { width: 1280, height: 900 }, deviceScaleFactor: 1, locale: "en-US",
    colorScheme: "light", reducedMotion: "reduce", serviceWorkers: "block" });
  const page = await context.newPage();
  page.setDefaultTimeout(10000);
  await page.goto("/login");
  await page.getByLabel("Email").fill(process.env.PLAYWRIGHT_EMAIL);
  await page.getByLabel("Password").fill(process.env.PLAYWRIGHT_PASSWORD);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(new URL("/", process.env.PLAYWRIGHT_BASE_URL).href);
  const home = await page.locator(".cards a").first().getAttribute("href");
  assert.ok(home?.startsWith("/workspaces/"));
  const cdp = await context.newCDPSession(page);
  await cdp.send("Network.enable");
  const results = [];
  async function measure(name, action) {
    console.error(`Measuring ${name}`);
    const stop = observeNetwork(cdp);
    const start = performance.now();
    try {
      const state = await action();
      results.push({ name, elapsedMs: Math.round((performance.now() - start) * 10) / 10,
        ...stop(), ...(state || {}) });
    } finally { stop(); }
  }

  for (const [name, suffix] of [["item filter", "/items?search=missing"],
    ["client filter", "/clients?filter=displayName:contains:missing"],
    ["file filter", "/files?search=missing"], ["member filter", "/settings/members?email=missing"]]) {
    await measure(name, async () => {
      const response = await page.goto(home + suffix, { waitUntil: "load" });
      assert.equal(response.status(), 200);
      return { emptyVisible: await page.locator("#client-results").isVisible().catch(() => false) };
    });
  }

  await page.goto(home + "/clients");
  await page.locator(".client-create summary").click();
  await measure("client required validation", async () => {
    await page.locator(".client-create button[type=submit]").click();
    return { invalid: await page.locator("#client-display-name").evaluate((input) => !input.validity.valid) };
  });
  await page.locator("#client-display-name").fill("Lifecycle fixture");
  await page.locator(".client-create button[type=submit]").click();
  await page.locator("#client-detail").waitFor();
  await page.locator('.client-lifecycle form[action$="/archive"] button').click();
  await page.locator('.client-lifecycle form[action$="/unarchive"]').waitFor();
  await measure("client unarchive", async () => {
    await page.locator('.client-lifecycle form[action$="/unarchive"] button').click();
    await page.locator('.client-lifecycle form[action$="/archive"]').waitFor();
  });
  page.on("dialog", (dialog) => dialog.accept());
  await measure("client delete", async () => {
    await page.locator('.client-lifecycle form[action$="/delete"] button').click();
    await page.locator("#client-list").waitFor();
  });

  await page.goto(home + "/settings/invitations");
  await page.locator("#invite-email").fill("fixture@example.invalid");
  await page.locator('button[value="create"]').click();
  await page.locator('button[value="resend"]').waitFor();
  await measure("invitation resend", async () => {
    await page.locator('button[value="resend"]').click();
    await page.locator('button[value="revoke"]').waitFor();
  });

  const noJS = await browser.newContext({ baseURL: process.env.PLAYWRIGHT_BASE_URL,
    viewport: { width: 1280, height: 900 }, locale: "en-US", javaScriptEnabled: false,
    serviceWorkers: "block", storageState: await context.storageState() });
  try {
    const fallback = await noJS.newPage();
    const fallbackCDP = await noJS.newCDPSession(fallback);
    await fallbackCDP.send("Network.enable");
    await fallback.goto(home + "/items");
    await fallback.getByLabel("New item title").fill("No JavaScript fixture");
    const stop = observeNetwork(fallbackCDP);
    const start = performance.now();
    try {
      await fallback.getByRole("button", { name: "Add item" }).click();
      await fallback.locator('.item-list-view input[value="No JavaScript fixture"]').waitFor();
      results.push({ name: "item create without JavaScript", elapsedMs: Math.round((performance.now() - start) * 10) / 10, ...stop() });
    } finally { stop(); }
  } finally { await noJS.close(); }
  const mobile = await browser.newContext({ baseURL: process.env.PLAYWRIGHT_BASE_URL,
    viewport: { width: 390, height: 844 }, deviceScaleFactor: 1, locale: "es-ES",
    colorScheme: "dark", reducedMotion: "reduce", serviceWorkers: "block",
    storageState: await context.storageState() });
  try {
    const mobilePage = await mobile.newPage();
    await mobilePage.goto("/language?locale=es&return_to=/");
    const mobileCDP = await mobile.newCDPSession(mobilePage);
    await mobileCDP.send("Network.enable");
    await mobileCDP.send("Network.clearBrowserCache");
    const stop = observeNetwork(mobileCDP);
    const start = performance.now();
    try {
      const response = await mobilePage.goto(home + "/items", { waitUntil: "load" });
      assert.equal(response.status(), 200);
      results.push({ name: "mobile Spanish dark Items navigation", elapsedMs: Math.round((performance.now() - start) * 10) / 10,
        language: await mobilePage.locator("html").getAttribute("lang"),
        canvas: await mobilePage.locator("html").evaluate((element) => getComputedStyle(element).getPropertyValue("--canvas").trim()), ...stop() });
    } finally { stop(); }
  } finally { await mobile.close(); }
  console.log(JSON.stringify({ date: new Date().toISOString(), browser: browser.version(), results }, null, 2));
} finally { await browser.close(); }
