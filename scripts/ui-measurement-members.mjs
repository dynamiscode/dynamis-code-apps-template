import assert from "node:assert/strict";
import os from "node:os";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { performance } from "node:perf_hooks";
import { chromium } from "playwright";
import { observeNetwork } from "./ui-measurement-network.mjs";

const browser = await chromium.launch({ channel: "chrome" });
let phase = "launch";
try {
  const context = await browser.newContext({
    baseURL: process.env.PLAYWRIGHT_BASE_URL,
    viewport: { width: 1280, height: 900 },
    deviceScaleFactor: 1,
    locale: "en-US",
    colorScheme: "light",
    reducedMotion: "reduce",
    serviceWorkers: "block",
  });
  const page = await context.newPage();
  await page.goto("/login");
  phase = "login";
  await page.getByLabel("Email").fill(process.env.PLAYWRIGHT_EMAIL);
  await page.getByLabel("Password").fill(process.env.PLAYWRIGHT_PASSWORD);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(new URL("/", process.env.PLAYWRIGHT_BASE_URL).href);
  const workspaceLink = page.locator('.cards a[href^="/workspaces/"]').first();
  const workspacePath = new URL(await workspaceLink.getAttribute("href"), process.env.PLAYWRIGHT_BASE_URL).pathname;
  const cdp = await context.newCDPSession(page);
  await cdp.send("Network.enable");
  const results = [];

  async function measureNavigation(screen, suffix, marker) {
    for (const cache of ["cold", "warm"]) {
      if (cache === "cold") await cdp.send("Network.clearBrowserCache");
      const stop = observeNetwork(cdp);
      try {
        const response = await page.goto(`${workspacePath}${suffix}`, { waitUntil: "load" });
        assert.equal(response.status(), 200, `${screen} navigation failed`);
        await page.locator(marker).waitFor();
        const timing = await page.evaluate(() => {
          const entry = performance.getEntriesByType("navigation")[0];
          return { domContentLoadedMs: entry.domContentLoadedEventEnd, loadMs: entry.loadEventEnd };
        });
        const network = stop();
        assert.ok(network.requests > 0 && timing.loadMs > 0, "navigation metrics missing");
        results.push({ screen, cache, ...network, timing });
      } finally {
        stop();
      }
    }
  }

  phase = "members navigation";
  await measureNavigation("members", "/settings/members", "main h1");
  phase = "invitations navigation";
  await measureNavigation("invitations", "/settings/invitations", "main h1");

  phase = "invitation creation";
  const form = page.locator('form[data-persist="invitation-create"]');
  await form.locator('input[name="email"]').fill("ui-measurement-invite@example.test");
  const stop = observeNetwork(cdp);
  const start = performance.now();
  try {
    const responsePromise = page.waitForResponse((response) =>
      response.request().method() === "POST" && new URL(response.url()).pathname === `${workspacePath}/settings/invitations`);
    await form.getByRole("button", { name: "Create invitation" }).click();
    const response = await responsePromise;
    assert.ok(response.status() >= 200 && response.status() < 400, "invitation creation failed");
    await page.getByLabel("Copy this invitation link").waitFor();
    results.push({ interaction: "create invitation", status: response.status(), ...stop(), feedbackMs: performance.now() - start });
  } finally {
    stop();
  }

  console.log(JSON.stringify({
    commit: execFileSync("git", ["rev-parse", "HEAD"], { encoding: "utf8" }).trim(),
    date: new Date().toISOString(), os: `${os.type()} ${os.release()} ${os.arch()}`,
    browser: `Chrome ${browser.version()}`, node: process.version,
    playwright: JSON.parse(readFileSync("node_modules/@playwright/test/package.json", "utf8")).version,
    fixture: "fresh isolated SQLite: one owner, one workspace, one synthetic member, no invitations",
    scenario: "cold/warm members and invitations navigation; create one synthetic invitation",
    viewport: "1280x900, device scale 1", locale: "en-US", theme: "light",
    reducedMotion: "reduce", network: "unthrottled loopback", results,
  }, null, 2));
  await cdp.detach();
  await context.close();
} catch (error) {
  console.error("Members measurement failed; inspect the isolated fixture locally without sharing logs.");
  console.error(`Phase: ${phase}`);
  console.error(error instanceof Error ? error.name : "UnknownError");
  process.exitCode = 1;
} finally {
  await browser.close();
}
