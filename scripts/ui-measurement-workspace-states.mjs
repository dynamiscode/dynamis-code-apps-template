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
  for (const [name, suffix] of [["home", ""], ["items", "/items"], ["clients", "/clients"],
    ["files", "/files"], ["members", "/settings/members"], ["invitations", "/settings/invitations"]]) {
    await page.goto(home, { waitUntil: "load" });
    const target = home + suffix;
    let release;
    let arrived;
    const gate = new Promise((resolve) => { release = resolve; });
    const intercepted = new Promise((resolve) => { arrived = resolve; });
    const handler = async (route) => {
      if (route.request().method() !== "GET" || new URL(route.request().url()).pathname !== target) {
        await route.continue();
        return;
      }
      arrived();
      await gate;
      await route.fulfill({ status: 503, contentType: "text/plain", body: "Synthetic unavailable" });
    };
    await page.route("**/*", handler);
    const stop = observeNetwork(cdp);
    const start = performance.now();
    try {
      const navigation = page.goto(target, { waitUntil: "load" });
      await intercepted;
      release();
      const response = await navigation;
      assert.equal(response.status(), 503);
      results.push({ name, elapsedMs: Math.round((performance.now() - start) * 10) / 10,
        status: response.status(), pendingUI: "unavailable during intercepted navigation", ...stop() });
    } finally {
      release();
      stop();
      await page.unroute("**/*", handler);
    }
  }
  console.log(JSON.stringify({ date: new Date().toISOString(), browser: browser.version(), results }, null, 2));
} finally { await browser.close(); }
