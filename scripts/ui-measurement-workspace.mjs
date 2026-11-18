import assert from "node:assert/strict";
import os from "node:os";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { chromium } from "playwright";
import { observeNetwork } from "./ui-measurement-network.mjs";
import { observeWorkspaceAttribution } from "./ui-measurement-workspace-attribution.mjs";

const browser = await chromium.launch({ channel: "chrome" });
try {
  const context = await browser.newContext({
    baseURL: process.env.PLAYWRIGHT_BASE_URL,
    viewport: { width: 1280, height: 900 }, deviceScaleFactor: 1,
    locale: "en-US", colorScheme: "light", reducedMotion: "reduce", serviceWorkers: "block",
  });
  const page = await context.newPage();
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
    await cdp.send("Network.clearBrowserCache");
    for (const cache of ["cold", "warm"]) {
      const stop = observeNetwork(cdp);
      const stopAttribution = observeWorkspaceAttribution(cdp);
      try {
        const response = await page.goto(home + suffix, { waitUntil: "load" });
        assert.equal(response.status(), 200, `${name} navigation`);
        const timing = await page.evaluate(() => {
          const entry = performance.getEntriesByType("navigation")[0];
          return { domContentLoadedMs: entry.domContentLoadedEventEnd, loadMs: entry.loadEventEnd };
        });
        results.push({ name, cache, ...stop(), attribution: stopAttribution(), timing });
      } finally {
        stop();
        stopAttribution();
      }
    }
  }
  console.log(JSON.stringify({
    commit: execFileSync("git", ["rev-parse", "HEAD"], { encoding: "utf8" }).trim(),
    date: new Date().toISOString(), os: `${os.type()} ${os.release()} ${os.arch()}`,
    browser: `Chrome ${browser.version()}`, node: process.version,
    playwright: JSON.parse(readFileSync("node_modules/@playwright/test/package.json", "utf8")).version,
    fixture: "fresh SQLite: one owner, one workspace, zero items, clients, files and invitations",
    viewport: "1280x900, device scale 1", locale: "en-US", theme: "light",
    reducedMotion: "reduce", network: "unthrottled loopback", results,
  }, null, 2));
} finally {
  await browser.close();
}
