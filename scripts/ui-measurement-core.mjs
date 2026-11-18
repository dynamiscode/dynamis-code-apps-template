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
  await page.locator(".cards a").first().click();
  const workspacePath = new URL(page.url()).pathname;
  const cdp = await context.newCDPSession(page);
  await cdp.send("Network.enable");
  const results = [];

  async function measureNavigation(screen, path, marker) {
    for (const cache of ["cold", "warm"]) {
      if (cache === "cold") await cdp.send("Network.clearBrowserCache");
      const stop = observeNetwork(cdp);
      try {
        const response = await page.goto(path, { waitUntil: "load" });
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

  for (const [screen, suffix, marker] of [
    ["Items list", "/items", "#item-list"],
    ["Clients list", "/clients", "#client-list"],
    ["Files list", "/files", ".files-page"],
  ]) {
    phase = `${screen} navigation`;
    await measureNavigation(screen, `${workspacePath}${suffix}`, marker);
  }

  phase = "item create";
  await page.goto(`${workspacePath}/items`);
  const itemForm = page.locator("#item-list form.create");
  await itemForm.locator('input[name="title"]').fill("UI measurement item");
  const createCDP = await context.newCDPSession(page);
  try {
    await createCDP.send("Network.enable");
    const stopCreate = observeNetwork(createCDP);
    const createStart = performance.now();
    const createResponse = page.waitForResponse((response) =>
      response.request().method() === "POST" && new URL(response.url()).pathname === `${workspacePath}/items`);
    await itemForm.getByRole("button", { name: "Add item" }).click();
    const status = (await createResponse).status();
    await page.locator('#item-list input[name="title"][value="UI measurement item"]').waitFor();
    results.push({ interaction: "create item", status, ...stopCreate(), feedbackMs: performance.now() - createStart });
  } finally {
    await createCDP.detach();
  }

  phase = "item validation";
  await page.goto(`${workspacePath}/items`);
  const validationForm = page.locator("#item-list form.create");
  await validationForm.locator('input[name="title"]').fill("");
  const validationStart = performance.now();
  await validationForm.getByRole("button", { name: "Add item" }).click();
  const invalid = await validationForm.locator('input[name="title"]').evaluate((input) => input.validity.valueMissing);
  assert.equal(invalid, true, "empty title browser validation missing");
  results.push({ interaction: "empty item title browser validation", feedbackMs: performance.now() - validationStart });

  console.log(JSON.stringify({
    commit: execFileSync("git", ["rev-parse", "HEAD"], { encoding: "utf8" }).trim(),
    date: new Date().toISOString(), os: `${os.type()} ${os.release()} ${os.arch()}`,
    browser: `Chrome ${browser.version()}`, node: process.version,
    playwright: JSON.parse(readFileSync("node_modules/@playwright/test/package.json", "utf8")).version,
    fixture: "fresh isolated SQLite: one owner, one workspace, zero items, clients, and files",
    scenario: "empty Items, Clients, Files list navigation; synthetic Item creation and invalid-title feedback",
    viewport: "1280x900, device scale 1", locale: "en-US", theme: "light",
    reducedMotion: "reduce", network: "unthrottled loopback", results,
  }, null, 2));
  await context.close();
} catch (error) {
  console.error("Core UI measurement failed; inspect the isolated fixture locally without sharing logs.");
  console.error(`Phase: ${phase}`);
  console.error(error instanceof Error ? error.name : "UnknownError");
  process.exitCode = 1;
} finally {
  await browser.close();
}
