import assert from "node:assert/strict";
import http from "node:http";
import { createRequire } from "node:module";
import { expect } from "@playwright/test";
import { chromium } from "playwright";
import { recordRequest } from "./ui-measurement-admin-attribution.mjs";

const baseURL = process.env.PLAYWRIGHT_BASE_URL;
const axePath = createRequire(import.meta.url).resolve("axe-core/axe.min.js");
const browser = await chromium.launch({ channel: "chrome" });
const receiver = http.createServer((request, response) => {
  request.resume();
  response.writeHead(request.url === "/ok" ? 204 : 503).end();
});
let phase = "startup";
try {
  await new Promise((resolve) => receiver.listen(0, "127.0.0.1", resolve));
  const context = await browser.newContext({ baseURL, locale: "en-US", serviceWorkers: "block" });
  const page = await context.newPage();
  await page.goto("/login");
  await page.getByLabel("Email").fill(process.env.PLAYWRIGHT_EMAIL);
  await page.getByLabel("Password").fill(process.env.PLAYWRIGHT_PASSWORD);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(new URL("/", baseURL).href);
  const workspace = await page.locator(".cards a").first().getAttribute("href");
  assert.ok(workspace?.startsWith("/workspaces/"));
  const results = [];
  for (const screen of ["general", "email", "tokens", "provisioning", "webhooks", "audit", "import", "export"]) {
    const counts = {};
    const record = (request) => recordRequest(counts, request.url(), baseURL, request.resourceType());
    page.on("request", record);
    try {
      const response = await page.goto(`${workspace}/settings/${screen}`, { waitUntil: "load" });
      assert.equal(response.status(), 200);
      await page.locator("main h1").waitFor();
      results.push({ screen, requests: counts });
    } finally {
      page.off("request", record);
    }
  }

  phase = "audit failure";
  await page.route("**/settings/audit?part=table", (route) => route.fulfill({ status: 503, body: "unavailable" }));
  await page.goto(`${workspace}/settings/audit`, { waitUntil: "load" });
  await page.locator('#audit-table:not([aria-busy]) [role="status"]').waitFor();
  results.push({ state: "audit fragment injected HTTP 503", failureFeedbackVisible: true });
  await page.unrouteAll();

  phase = "invalid import";
  await page.goto(`${workspace}/settings/import`, { waitUntil: "load" });
  await page.locator("#import-file").setInputFiles({ name: "invalid.csv", mimeType: "text/csv",
    buffer: Buffer.from("title,status\n,not-a-status\n") });
  await Promise.all([page.waitForNavigation({ waitUntil: "load" }),
    page.locator('.import-preview-form button[type="submit"]').click()]);
  await page.locator('.import-preview [role="alert"]').waitFor();
  results.push({ state: "invalid CSV preview", validationFeedbackVisible: true });

  phase = "delivery fixture";
  await page.goto(`${workspace}/settings/webhooks`, { waitUntil: "load" });
  await page.locator("#webhook-name").fill("State fixture");
  await page.locator("#webhook-url").fill(`http://127.0.0.1:${receiver.address().port}/hook`);
  await page.locator('input[name="events"][value="item.created"]').check();
  await Promise.all([page.waitForNavigation({ waitUntil: "load" }),
    page.locator('button[name="action"][value="create"]').click()]);
  assert.equal(await page.locator(".webhook-secret").count(), 1);
  const deliveryPath = await page.locator('.webhook-card a[href$="/deliveries"]').getAttribute("href");
  assert.ok(deliveryPath);
  await page.goto(`${workspace}/settings/webhooks`, { waitUntil: "load" });
  await page.locator("#webhook-name").fill("Successful state fixture");
  await page.locator("#webhook-url").fill(`http://127.0.0.1:${receiver.address().port}/ok`);
  await page.locator('input[name="events"][value="item.created"]').check();
  await Promise.all([page.waitForNavigation({ waitUntil: "load" }),
    page.locator('button[name="action"][value="create"]').click()]);
  const successPath = await page.locator('.webhook-card').filter({ hasText: "Successful state fixture" })
    .locator('a[href$="/deliveries"]').getAttribute("href");
  assert.ok(successPath);
  await page.goto(`${workspace}/items`, { waitUntil: "load" });
  await page.getByLabel("New item title").fill("State fixture item");
  await page.getByRole("button", { name: "Add item" }).click();
  await page.locator('#item-list input[value="State fixture item"]').waitFor();
  const deliveryResponse = await page.goto(deliveryPath, { waitUntil: "load" });
  assert.equal(deliveryResponse.status(), 200);
  const rows = await page.locator("main table tbody tr").count();
  const pending = await page.getByText("Pending", { exact: true }).count();
  results.push({ state: "synthetic delivery with loopback HTTP 503 receiver", rows, pending,
    pendingObserved: pending > 0 });
  for (const [name, path, label] of [["delivered", successPath, "Delivered"], ["exhausted", deliveryPath, "Failed"]]) {
    let observed = false;
    try {
      await expect.poll(async () => {
        const viewed = await page.goto(path, { waitUntil: "load" });
        assert.equal(viewed.status(), 200);
        return page.getByText(label, { exact: true }).count();
      }, { timeout: 55_000, intervals: [1000, 2000, 4000] }).toBeGreaterThan(0);
      observed = true;
    } catch {
      // The worker may not reach the requested state within the bounded observation window.
    }
    results.push({ state: `synthetic delivery ${name}`, observed });
  }

  phase = "no-JavaScript fallback";
  const noJS = await browser.newContext({ baseURL, locale: "en-US", javaScriptEnabled: false,
    serviceWorkers: "block", storageState: await context.storageState() });
  const fallback = await noJS.newPage();
  const audit = await fallback.goto(`${workspace}/settings/audit`, { waitUntil: "load" });
  assert.equal(audit.status(), 200);
  const link = fallback.locator('#audit-table a[href$="?all=1"]');
  await Promise.all([fallback.waitForNavigation({ waitUntil: "load" }), link.click()]);
  await fallback.locator("#audit-table:not([data-lazy-src])").waitFor();
  results.push({ state: "audit no-JavaScript fallback", tableRendered: true });
  await fallback.goto(`${workspace}/settings/import`, { waitUntil: "load" });
  await fallback.locator("#import-file").setInputFiles({ name: "invalid.csv", mimeType: "text/csv",
    buffer: Buffer.from("title,status\n,not-a-status\n") });
  await Promise.all([fallback.waitForNavigation({ waitUntil: "load" }),
    fallback.locator('.import-preview-form button[type="submit"]').click()]);
  await fallback.locator('.import-preview [role="alert"]').waitFor();
  results.push({ state: "invalid CSV no-JavaScript fallback", validationFeedbackVisible: true });
  await noJS.close();

  const anonymous = await browser.newContext({ baseURL, javaScriptEnabled: false });
  const denied = await anonymous.newPage();
  const response = await denied.goto(`${workspace}/settings/audit`, { waitUntil: "load" });
  results.push({ state: "anonymous audit request", finalStatus: response.status(),
    reachedLogin: new URL(denied.url()).pathname === "/login" });
  await anonymous.close();

  phase = "viewer permissions";
  await page.goto(`${workspace}/settings/invitations`, { waitUntil: "load" });
  await page.locator("#invite-email").fill("viewer@example.com");
  await page.locator("#invite-role").selectOption("viewer");
  await Promise.all([page.waitForNavigation({ waitUntil: "load" }),
    page.locator('button[name="action"][value="create"]').click()]);
  const invite = await page.locator('input[aria-label="Copy this invitation link"]').inputValue();
  const inviteURL = new URL(invite, baseURL);
  assert.equal(inviteURL.origin, new URL(baseURL).origin);
  assert.ok(inviteURL.pathname.startsWith("/invitations/"));
  const viewerContext = await browser.newContext({ baseURL, locale: "en-US", serviceWorkers: "block" });
  const viewer = await viewerContext.newPage();
  await viewer.goto(inviteURL.href, { waitUntil: "load" });
  await viewer.locator("#invite-password").fill("viewer-fixture-password-123");
  await Promise.all([viewer.waitForNavigation({ waitUntil: "load" }),
    viewer.locator('button[name="action"][value="register"]').click()]);
  assert.ok(new URL(viewer.url()).pathname.endsWith("/items"));
  for (const screen of ["general", "email", "tokens", "provisioning", "webhooks", "audit", "import", "export"]) {
    const viewed = await viewer.goto(`${workspace}/settings/${screen}`, { waitUntil: "load" });
    results.push({ state: "viewer settings navigation", screen, status: viewed.status() });
  }
  await viewer.goto(`${workspace}/settings/general`, { waitUntil: "load" });
  const csrf = await viewer.locator('form[action$="/settings/general"] input[name="csrf"]').inputValue();
  for (const screen of ["general", "email", "provisioning", "webhooks"]) {
    const status = await viewer.evaluate(async ({ path, csrf }) => {
      const body = new URLSearchParams({ csrf, locale: "es", action: "create" });
      const response = await fetch(path, { method: "POST", body, redirect: "manual" });
      return response.status;
    }, { path: `${workspace}/settings/${screen}`, csrf });
    results.push({ state: "viewer mutation", screen, status });
  }
  await viewerContext.close();

  phase = "Spanish mobile accessibility";
  const mobile = await browser.newContext({ baseURL, locale: "es-ES", colorScheme: "dark",
    viewport: { width: 390, height: 844 }, deviceScaleFactor: 1, serviceWorkers: "block",
    storageState: await context.storageState() });
  await mobile.addInitScript({ path: axePath });
  const mobilePage = await mobile.newPage();
  await mobilePage.goto("/language?locale=es&return_to=%2F", { waitUntil: "load" });
  for (const screen of ["general", "email", "tokens", "provisioning", "webhooks", "audit", "import", "export"]) {
    phase = `Spanish mobile ${screen} navigation`;
    const viewed = await mobilePage.goto(`${workspace}/settings/${screen}`, { waitUntil: "load" });
    assert.equal(viewed.status(), 200);
    await mobilePage.locator("main h1").waitFor();
    if (screen === "audit") await mobilePage.locator("#audit-table:not([data-lazy-src])").waitFor();
    phase = `Spanish mobile ${screen} axe scan`;
    const state = await mobilePage.evaluate(async () => {
      const scan = await window.axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa"] } });
      return { lang: document.documentElement.lang, theme: document.documentElement.dataset.theme,
        horizontalOverflow: document.documentElement.scrollWidth > document.documentElement.clientWidth,
        violations: scan.violations.map((rule) => ({ id: rule.id, impact: rule.impact, count: rule.nodes.length })) };
    });
    results.push({ state: "Spanish mobile dark-system axe scan", screen, ...state });
  }
  await mobile.close();
  console.log(JSON.stringify({ scenario: "isolated owner and viewer fixture; no timing metrics", results }, null, 2));
} catch {
  console.error(`Administration state checks failed during ${phase}; inspect locally without sharing logs.`);
  process.exitCode = 1;
} finally {
  await browser.close();
  await new Promise((resolve) => receiver.close(resolve));
}
