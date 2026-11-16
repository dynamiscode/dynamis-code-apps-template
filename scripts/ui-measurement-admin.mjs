import assert from "node:assert/strict";
import os from "node:os";
import { execFileSync } from "node:child_process";
import { readFileSync, statSync } from "node:fs";
import { performance } from "node:perf_hooks";
import { chromium } from "playwright";
import { observeNetwork } from "./ui-measurement-network.mjs";

const browser = await chromium.launch({ channel: "chrome" });
try {
  const context = await browser.newContext({ baseURL: process.env.PLAYWRIGHT_BASE_URL,
    viewport: { width: 1280, height: 900 }, deviceScaleFactor: 1, locale: "en-US",
    colorScheme: "light", reducedMotion: "reduce", serviceWorkers: "block" });
  await context.addInitScript(() => {
    window.auditLoadingObserved = false;
    const observer = new MutationObserver(() => {
      if (document.querySelector('#audit-table[aria-busy="true"] [role="status"]')) {
        window.auditLoadingObserved = true;
      }
    });
    observer.observe(document, { childList: true, subtree: true });
  });
  const page = await context.newPage();
  await page.goto("/login");
  await page.getByLabel("Email").fill(process.env.PLAYWRIGHT_EMAIL);
  await page.getByLabel("Password").fill(process.env.PLAYWRIGHT_PASSWORD);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(new URL("/", process.env.PLAYWRIGHT_BASE_URL).href);
  const workspace = await page.locator(".cards a").first().getAttribute("href");
  assert.ok(workspace?.startsWith("/workspaces/"), "workspace home link missing");
  const cdp = await context.newCDPSession(page);
  await cdp.send("Network.enable");
  const results = [];
  for (const screen of ["general", "email", "tokens", "provisioning", "webhooks", "audit", "import", "export"]) {
    for (const cache of ["cold", "warm"]) {
      if (cache === "cold") await cdp.send("Network.clearBrowserCache");
      const stop = observeNetwork(cdp);
      try {
        const response = await page.goto(`${workspace}/settings/${screen}`, { waitUntil: "load" });
        assert.equal(response.status(), 200, `${screen} navigation failed`);
        await page.locator("main h1").waitFor();
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
  // Create one synthetic webhook without triggering a delivery or recording its one-time secret.
  await page.goto(`${workspace}/settings/webhooks`);
  await page.locator("#webhook-name").fill("Measurement fixture");
  await page.locator("#webhook-url").fill("https://example.com/hook");
  await page.locator('input[name="events"][value="item.created"]').check();
  await Promise.all([
    page.waitForNavigation({ waitUntil: "load" }),
    page.locator('button[name="action"][value="create"]').click(),
  ]);
  assert.equal(await page.locator(".webhook-secret").count(), 1, "webhook fixture creation failed");
  const deliveries = await page.locator('.webhook-card a[href$="/deliveries"]').getAttribute("href");
  assert.ok(deliveries, "deliveries link missing");
  for (const cache of ["cold", "warm"]) {
    if (cache === "cold") await cdp.send("Network.clearBrowserCache");
    const stop = observeNetwork(cdp);
    try {
      const response = await page.goto(deliveries, { waitUntil: "load" });
      assert.equal(response.status(), 200, "deliveries navigation failed");
      await page.locator("main h1").waitFor();
      const timing = await page.evaluate(() => {
        const entry = performance.getEntriesByType("navigation")[0];
        return { domContentLoadedMs: entry.domContentLoadedEventEnd, loadMs: entry.loadEventEnd };
      });
      results.push({ screen: "webhook deliveries", cache, ...stop(), timing });
    } finally {
      stop();
    }
  }

  // Observe the automatic request and replacement separately from the synthetic fetch below.
  const [auditResponse] = await Promise.all([
    page.waitForResponse((response) => {
      const url = new URL(response.url());
      return url.pathname === `${workspace}/settings/audit` && url.searchParams.get("part") === "table";
    }),
    page.goto(`${workspace}/settings/audit`, { waitUntil: "load" }),
  ]);
  assert.equal(auditResponse.status(), 200, "automatic audit fragment failed");
  await page.locator("#audit-table:not([data-lazy-src])").waitFor();
  const auditSwap = await page.evaluate(() => ({ elapsedMs: performance.now(),
    loadingObserved: window.auditLoadingObserved }));
  const auditSizes = await auditResponse.request().sizes();
  results.push({ interaction: "automatic audit fragment and DOM swap", cache: "warm",
    ...auditSwap, responseHeaderBytes: auditSizes.responseHeadersSize,
    responseBodyBytes: auditSizes.responseBodySize });

  // Request the same fragment as forms.js, separately from the navigation interval.
  await page.goto(`${workspace}/settings/audit?all=1`, { waitUntil: "load" });
  let stop = observeNetwork(cdp);
  try {
    const start = performance.now();
    const status = await page.evaluate(async (path) => {
      const response = await fetch(`${path}?part=table`, { headers: { "HX-Request": "true" }, credentials: "same-origin" });
      await response.arrayBuffer();
      return response.status;
    }, `${workspace}/settings/audit`);
    const elapsedMs = performance.now() - start;
    assert.equal(status, 200, "audit fragment request failed");
    results.push({ interaction: "audit fragment", cache: "warm", ...stop(), elapsedMs });
  } finally {
    stop();
  }

  await page.goto(`${workspace}/settings/import`, { waitUntil: "load" });
  await page.locator("#import-file").setInputFiles({ name: "items.csv", mimeType: "text/csv",
    buffer: Buffer.from("title,status\nMeasurement item,active\n") });
  stop = observeNetwork(cdp);
  try {
    const start = performance.now();
    const [response] = await Promise.all([
      page.waitForNavigation({ waitUntil: "load" }),
      page.locator('.import-preview-form button[type="submit"]').click(),
    ]);
    assert.equal(response.status(), 200, "import preview failed");
    await page.locator(".import-preview").waitFor();
    results.push({ interaction: "import preview", cache: "warm", ...stop(), elapsedMs: performance.now() - start });
  } finally {
    stop();
  }

  await page.goto(`${workspace}/settings/export`, { waitUntil: "load" });
  stop = observeNetwork(cdp);
  try {
    const start = performance.now();
    const [download] = await Promise.all([
      page.waitForEvent("download"),
      page.locator("[data-webmcp-export]").click(),
    ]);
    const downloadPath = await download.path();
    assert.ok(downloadPath, "export download failed");
    const decodedBytes = statSync(downloadPath).size;
    results.push({ interaction: "export download", cache: "warm", ...stop(),
      elapsedMs: performance.now() - start, decodedBytes, transferBytes: "unavailable" });
  } finally {
    stop();
  }
  console.log(JSON.stringify({ commit: execFileSync("git", ["rev-parse", "HEAD"], { encoding: "utf8" }).trim(),
    date: new Date().toISOString(), os: `${os.type()} ${os.release()} ${os.arch()}`,
    browser: `Chrome ${browser.version()}`, node: process.version,
    playwright: JSON.parse(readFileSync("node_modules/@playwright/test/package.json", "utf8")).version,
    fixture: "fresh SQLite: one owner, one workspace, zero items; one webhook with zero deliveries added after settings navigation",
    scenario: "settings navigation and separately bounded interactions; audit fragment may overlap navigation",
    viewport: "1280x900, device scale 1", locale: "en-US", theme: "light",
    reducedMotion: "reduce", network: "unthrottled loopback", results }, null, 2));
} catch {
  // Playwright errors can include URLs or page contents, including one-time secrets.
  console.error("Administration measurement failed; inspect the isolated fixture locally without sharing logs.");
  process.exitCode = 1;
} finally {
  await browser.close();
}
