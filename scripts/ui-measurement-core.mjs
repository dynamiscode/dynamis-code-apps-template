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

  phase = "client detail";
  await page.goto(`${workspacePath}/clients`);
  await page.locator("#client-create > details.client-create > summary").click();
  await page.getByLabel("Display name").fill("UI measurement client");
  await page.getByLabel("Email").fill("ui-measurement-client@example.test");
  const clientCreateCDP = await context.newCDPSession(page);
  let clientPath;
  try {
    await clientCreateCDP.send("Network.enable");
    const stopCreateClient = observeNetwork(clientCreateCDP);
    const createClientStart = performance.now();
    const createClientResponse = page.waitForResponse((response) =>
      response.request().method() === "POST" && new URL(response.url()).pathname === `${workspacePath}/clients`);
    await page.getByRole("button", { name: "Add client" }).click();
    const status = (await createClientResponse).status();
    await page.waitForURL(new RegExp(`${workspacePath}/clients/[^/?]+$`));
    await page.locator("#client-detail").waitFor();
    clientPath = new URL(page.url()).pathname;
    results.push({ interaction: "create synthetic client", status, ...stopCreateClient(),
      feedbackMs: performance.now() - createClientStart });
  } finally {
    await clientCreateCDP.detach();
  }
  await measureNavigation("Client detail", clientPath, "#client-detail");

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

  phase = "item edit";
  const editForm = page.locator("#item-list form[data-item-edit]").first();
  await editForm.locator('input[name="title"]').fill("UI measurement item edited");
  const editCDP = await context.newCDPSession(page);
  try {
    await editCDP.send("Network.enable");
    const stopEdit = observeNetwork(editCDP);
    const editStart = performance.now();
    const editResponse = page.waitForResponse((response) =>
      response.request().method() === "POST" && new URL(response.url()).pathname.startsWith(`${workspacePath}/items/`));
    await editForm.locator('button[type="submit"][value="update"]').click();
    const status = (await editResponse).status();
    await page.locator('#item-list input[name="title"][value="UI measurement item edited"]').waitFor();
    results.push({ interaction: "edit item title", status, ...stopEdit(), feedbackMs: performance.now() - editStart });
  } finally {
    await editCDP.detach();
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

  phase = "file upload";
  await page.goto(`${workspacePath}/files`);
  const uploadForm = page.locator("form[data-file-upload]");
  await page.locator("#file-upload").setInputFiles({
    name: "ui-measurement-upload.txt",
    mimeType: "text/plain",
    buffer: Buffer.alloc(256 * 1024, "x"),
  });
  let uploadRequestId;
  let uploadRequestBody;
  const captureUploadRequest = (event) => {
    if (event.request.method === "POST" && new URL(event.request.url).pathname === `${workspacePath}/files`) {
      uploadRequestId = event.requestId;
      uploadRequestBody = cdp.send("Network.getRequestPostData", { requestId: uploadRequestId });
    }
  };
  cdp.on("Network.requestWillBeSent", captureUploadRequest);
  const stopUpload = observeNetwork(cdp);
  const uploadStart = performance.now();
  try {
    const [response] = await Promise.all([
      page.waitForResponse((response) =>
        response.request().method() === "POST" && new URL(response.url()).pathname === `${workspacePath}/files`),
      uploadForm.locator('button[type="submit"]').click(),
    ]);
    assert.ok(response.status() >= 200 && response.status() < 400, "file upload failed");
    await page.getByRole("link", { name: /ui-measurement-upload\.txt$/ }).waitFor();
    assert.ok(uploadRequestId, "upload request ID missing");
    const requestBody = await uploadRequestBody;
    assert.ok(Buffer.byteLength(requestBody.postData) >= 256 * 1024, "upload request body missing");
    const network = stopUpload();
    results.push({ interaction: "upload synthetic 256 KiB file", status: response.status(), ...network,
      requestBodyBytes: Buffer.byteLength(requestBody.postData),
      requestsWithUnknownBodyBytes: network.requestsWithUnknownBodyBytes - 1,
      feedbackMs: performance.now() - uploadStart });
  } finally {
    cdp.off("Network.requestWillBeSent", captureUploadRequest);
    stopUpload();
  }

  console.log(JSON.stringify({
    commit: execFileSync("git", ["rev-parse", "HEAD"], { encoding: "utf8" }).trim(),
    date: new Date().toISOString(), os: `${os.type()} ${os.release()} ${os.arch()}`,
    browser: `Chrome ${browser.version()}`, node: process.version,
    playwright: JSON.parse(readFileSync("node_modules/@playwright/test/package.json", "utf8")).version,
    fixture: "fresh isolated SQLite: one owner, one workspace, one synthetic client, one item, and one uploaded file",
    scenario: "empty Items, Clients, Files list navigation; synthetic Client creation and detail, Item creation/edit/validation, and file upload",
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
