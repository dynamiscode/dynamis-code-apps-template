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
  let sseMessages = 0;
  cdp.on("Network.eventSourceMessageReceived", () => { sseMessages++; });
  async function measure(name, action) {
    console.error(`Measuring ${name}`);
    const stop = observeNetwork(cdp);
    const start = performance.now();
    const initialMessages = sseMessages;
    try {
      await action();
      results.push({ name, elapsedMs: Math.round((performance.now() - start) * 10) / 10,
        sseMessages: sseMessages - initialMessages, ...stop() });
    } finally { stop(); }
  }
  await page.goto(home + "/items");
  await page.waitForFunction(() => document.querySelector("#realtime-status")?.textContent === document.querySelector("#item-list")?.dataset.liveConnected, null, { timeout: 30000 });
  const peer = await context.newPage();
  peer.setDefaultTimeout(10000);
  await peer.goto(home + "/items");
  await peer.getByLabel("New item title").fill("Peer measurement item");
  await measure("item SSE event and refresh", async () => {
    await peer.getByRole("button", { name: "Add item" }).click();
    await page.locator('.item-list-view input[value="Peer measurement item"]').waitFor();
  });
  await peer.close();
  await page.getByLabel("New item title").fill("Measurement item");
  await measure("item create and fragment", async () => {
    await page.getByRole("button", { name: "Add item" }).click();
    await page.locator('.item-list-view input[value="Measurement item"]').waitFor();
  });
  const edit = page.locator(".item-list-view .item-edit-form").first();
  await edit.locator('input[name="title"]').fill("Updated measurement item");
  await measure("item edit and fragment", async () => {
    await edit.locator('button[value="update"]').click();
    await page.locator('.item-list-view input[value="Updated measurement item"]').waitFor();
  });
  page.on("dialog", (dialog) => dialog.accept());
  await measure("item delete and fragment", async () => {
    await page.locator('.item-list-view .item-edit-form').filter({ has: page.locator('input[value="Updated measurement item"]') }).locator('button[value="delete"]').click();
    await page.locator('.item-list-view input[value="Updated measurement item"]').waitFor({ state: "detached" });
  });
  await page.goto(home + "/clients");
  assert.equal(await page.locator('script[src="/assets/app.js"]').count(), 0);
  await page.locator(".client-create summary").click();
  await page.locator("#client-display-name").fill("Measurement client");
  await measure("client create", async () => {
    await page.locator(".client-create button[type=submit]").click();
    await page.locator("#client-detail").waitFor();
  });
  await page.goto(home + "/clients");
  await measure("client detail navigation", async () => {
    await page.locator('.list-view-table tbody a').first().click();
    await page.locator("#client-detail").waitFor();
  });
  await page.locator("#client-display-name").fill("Updated measurement client");
  await measure("client edit", async () => {
    await page.locator('#client-detail form button[type="submit"]').first().click();
    await page.getByRole("heading", { name: "Updated measurement client" }).waitFor();
  });
  await measure("client archive", async () => {
    await page.locator('.client-lifecycle form[action$="/archive"] button').click();
    await page.locator('.client-lifecycle form[action$="/unarchive"]').waitFor();
  });
  await page.goto(home + "/files");
  const fileBytes = Buffer.alloc(1024 * 1024, 65);
  const progress = [];
  await page.exposeFunction("recordFixtureUploadProgress", (value) => { progress.push(value); });
  await page.evaluate(() => {
    const element = document.querySelector("[data-upload-progress]");
    new MutationObserver(() => {
      if (!element.hidden) window.recordFixtureUploadProgress(element.value);
    }).observe(element, { attributes: true, attributeFilter: ["value", "hidden"] });
  });
  await page.locator("#file-upload").setInputFiles({ name: "measurement.txt", mimeType: "text/plain", buffer: fileBytes });
  await measure("file upload and reload", async () => {
    await page.locator(".file-upload-form button").click();
    await page.getByText("measurement.txt", { exact: true }).first().waitFor();
  });
  results.at(-1).uploadProgress = { observedEvents: progress.length, maximumPercent: progress.length ? Math.max(...progress) : null };
  await measure("file download", async () => {
    const downloadPromise = page.waitForEvent("download");
    await page.locator('.list-view-table a[href$="/content"]').first().click();
    const download = await downloadPromise;
    assert.equal(await download.failure(), null);
    const chunks = [];
    for await (const chunk of await download.createReadStream()) chunks.push(chunk);
    const contents = Buffer.concat(chunks);
    assert.deepEqual(contents, fileBytes);
    results.push({ name: "download payload size", payloadBytes: contents.length });
  });
  await page.locator("#file-upload").setInputFiles({ name: "failure.txt", mimeType: "text/plain", buffer: Buffer.from("synthetic failure") });
  let releaseFailure;
  const failureGate = new Promise((resolve) => { releaseFailure = resolve; });
  await page.route("**/files", async (route) => {
    if (route.request().method() === "POST") {
      await failureGate;
      await route.fulfill({ status: 503, body: "unavailable" });
    }
    else await route.continue();
  });
  await measure("file upload injected failure", async () => {
    await page.locator(".file-upload-form button").click();
    await page.locator("[data-upload-status]:visible").waitFor();
    assert.equal(await page.locator(".file-upload-form button").isDisabled(), true);
    releaseFailure();
    await page.locator("[data-upload-status].error").waitFor();
    assert.equal(await page.locator(".file-upload-form button").isEnabled(), true);
  });
  await page.goto(home + "/settings/invitations");
  await page.locator("#invite-email").fill("fixture@example.invalid");
  await measure("invitation create", async () => {
    await page.locator('button[value="create"]').click();
    await page.locator('button[value="revoke"]').waitFor();
  });
  await measure("invitation revoke", async () => {
    await page.locator('button[value="revoke"]').click();
    await page.locator('button[value="revoke"]').waitFor({ state: "detached" });
  });
  console.log(JSON.stringify({ date: new Date().toISOString(), browser: browser.version(), results }, null, 2));
} finally { await browser.close(); }
