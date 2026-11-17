import assert from "node:assert/strict";
import os from "node:os";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { performance } from "node:perf_hooks";
import { chromium } from "playwright";
import { observeNetwork } from "./ui-measurement-network.mjs";

const browser = await chromium.launch({ channel: "chrome" });
try {
  const contextOptions = { baseURL: process.env.PLAYWRIGHT_BASE_URL,
    viewport: { width: 1280, height: 900 }, deviceScaleFactor: 1, locale: "en-US",
    colorScheme: "light", reducedMotion: "reduce", serviceWorkers: "block" };
  const context = await browser.newContext(contextOptions);
  await context.addInitScript(() => {
    window.notificationStream = { state: "pending", events: 0 };
    const NativeEventSource = window.EventSource;
    window.EventSource = class extends NativeEventSource {
      constructor(...args) {
        super(...args);
        this.addEventListener("open", () => { window.notificationStream.state = "open"; });
        this.addEventListener("message", () => { window.notificationStream.events++; });
        this.addEventListener("error", () => {
          if (this.readyState === NativeEventSource.CLOSED) window.notificationStream.state = "closed";
        });
      }
    };
  });
  const page = await context.newPage();
  const results = [];

  async function measure(target, screen, path, expectedStatus, cache) {
    const cdp = await target.context().newCDPSession(target);
    await cdp.send("Network.enable");
    if (cache === "cold") await cdp.send("Network.clearBrowserCache");
    const stop = observeNetwork(cdp);
    try {
      const response = await target.goto(path, { waitUntil: "load" });
      assert.ok(response, `${screen} navigation returned no response`);
      assert.equal(response.status(), expectedStatus, `${screen} navigation failed`);
      await target.locator("main h1").waitFor();
      const timing = await target.evaluate(() => {
        const entry = performance.getEntriesByType("navigation")[0];
        return { domContentLoadedMs: entry.domContentLoadedEventEnd, loadMs: entry.loadEventEnd };
      });
      const network = stop();
      assert.ok(network.requests > 0 && timing.loadMs > 0, "navigation metrics missing");
      results.push({ screen, cache, ...network, timing });
    } finally {
      stop();
      await cdp.detach();
    }
  }

  const anonymous = await browser.newContext(contextOptions);
  const publicPage = await anonymous.newPage();
  for (const [screen, path, expectedStatus] of [["login", "/login", 200],
    ["password recovery", "/password-reset", 200],
    ["public share unavailable", "/share/invalid-measurement-token", 404]]) {
    for (const cache of ["cold", "warm"]) {
      await measure(publicPage, screen, path, expectedStatus, cache);
    }
  }
  const invalidLoginPage = await anonymous.newPage();
  await invalidLoginPage.goto("/login");
  await invalidLoginPage.getByLabel("Email").fill("missing-user@example.com");
  await invalidLoginPage.getByLabel("Password").fill("invalid-login-password-123");
  const invalidLoginCDP = await invalidLoginPage.context().newCDPSession(invalidLoginPage);
  await invalidLoginCDP.send("Network.enable");
  const stopInvalidLoginNetwork = observeNetwork(invalidLoginCDP);
  const invalidLoginStart = performance.now();
  const invalidLoginResponse = invalidLoginPage.waitForResponse((response) =>
    response.url().endsWith("/login") && response.request().method() === "POST");
  await invalidLoginPage.getByRole("button", { name: "Sign in" }).click();
  const invalidLoginStatus = (await invalidLoginResponse).status();
  await invalidLoginPage.getByRole("alert").waitFor();
  const invalidLogin = stopInvalidLoginNetwork();
  assert.equal(invalidLoginStatus, 401, "invalid login status changed");
  assert.ok(invalidLogin.requests > 0, "invalid login request metrics missing");
  results.push({ interaction: "invalid login", status: invalidLoginStatus, ...invalidLogin,
    feedbackMs: performance.now() - invalidLoginStart });
  await invalidLoginCDP.detach();
  await invalidLoginPage.close();
  const recoveryPage = await anonymous.newPage();
  await recoveryPage.goto("/password-reset");
  await recoveryPage.getByLabel("Email").fill(process.env.PLAYWRIGHT_EMAIL);
  const recoveryCDP = await recoveryPage.context().newCDPSession(recoveryPage);
  await recoveryCDP.send("Network.enable");
  const stopRecoveryNetwork = observeNetwork(recoveryCDP);
  const recoveryStart = performance.now();
  const recoveryResponse = recoveryPage.waitForResponse((response) =>
    response.url().endsWith("/password-reset") && response.request().method() === "POST");
  await recoveryPage.getByRole("button", { name: "Send reset link" }).click();
  const recoveryStatus = (await recoveryResponse).status();
  await recoveryPage.getByRole("status").waitFor();
  const recovery = stopRecoveryNetwork();
  assert.equal(recoveryStatus, 200, "password recovery request status changed");
  assert.ok(recovery.requests > 0, "password recovery request metrics missing");
  results.push({ interaction: "password recovery request", status: recoveryStatus, ...recovery,
    feedbackMs: performance.now() - recoveryStart });
  await recoveryCDP.detach();
  await recoveryPage.close();
  const setupPage = await anonymous.newPage();
  for (const cache of ["cold", "warm"]) {
    await measure(setupPage, "setup", `${process.env.PLAYWRIGHT_SETUP_BASE_URL}/setup`, 200, cache);
  }
  await anonymous.close();

  await page.goto("/login");
  await page.getByLabel("Email").fill(process.env.PLAYWRIGHT_EMAIL);
  await page.getByLabel("Password").fill(process.env.PLAYWRIGHT_PASSWORD);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(new URL("/", process.env.PLAYWRIGHT_BASE_URL).href);
  for (const [screen, path] of [["account", "/account"], ["security", "/security"],
    ["sessions", "/sessions"], ["notifications", "/notifications"]]) {
    for (const cache of ["cold", "warm"]) await measure(page, screen, path, 200, cache);
  }
  await page.goto("/notifications", { waitUntil: "load" });
  const streamStart = performance.now();
  await page.waitForFunction(() => window.notificationStream.state !== "pending", null, { timeout: 10000 });
  results.push({ interaction: "notification SSE connect", state: await page.evaluate(() => window.notificationStream.state),
    events: await page.evaluate(() => window.notificationStream.events), elapsedMs: performance.now() - streamStart });

  console.log(JSON.stringify({ commit: execFileSync("git", ["rev-parse", "HEAD"], { encoding: "utf8" }).trim(),
    date: new Date().toISOString(), os: `${os.type()} ${os.release()} ${os.arch()}`,
    browser: `Chrome ${browser.version()}`, node: process.version,
    playwright: JSON.parse(readFileSync("node_modules/@playwright/test/package.json", "utf8")).version,
    fixture: "fresh isolated SQLite: one owner and workspace; zero notifications; no MFA, consent client, or valid public share token",
    scenario: "anonymous and authenticated identity navigation, invalid login, password recovery request, and notification SSE connection",
    viewport: "1280x900, device scale 1", locale: "en-US", theme: "light",
    reducedMotion: "reduce", network: "unthrottled loopback", results }, null, 2));
  await context.close();
} catch (error) {
  console.error("Identity measurement failed; inspect the isolated fixture locally without sharing logs.");
  console.error(error instanceof Error ? error.name : "UnknownError");
  process.exitCode = 1;
} finally {
  await browser.close();
}
