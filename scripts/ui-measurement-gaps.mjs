import assert from "node:assert/strict";
import { performance } from "node:perf_hooks";
import { chromium } from "playwright";
import { observeNetwork } from "./ui-measurement-network.mjs";
import { observeWorkspaceAttribution } from "./ui-measurement-workspace-attribution.mjs";

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
    const stopAttribution = observeWorkspaceAttribution(cdp);
    const start = performance.now();
    try {
      const state = await action();
      results.push({ name, elapsedMs: Math.round((performance.now() - start) * 10) / 10,
        ...stop(), attribution: stopAttribution(), ...(state || {}) });
    } finally { stop(); stopAttribution(); }
  }
  let notifyBusy = () => {};
  await page.exposeFunction("recordWorkspaceFixtureBusy", () => notifyBusy());
  async function measureDocumentMutationFailure(name, suffix, formSelector, buttonSelector) {
    let release;
    let arrived;
    const gate = new Promise((resolve) => { release = resolve; });
    const intercepted = new Promise((resolve) => { arrived = resolve; });
    const busy = new Promise((resolve) => { notifyBusy = resolve; });
    const handler = async (route) => {
      const request = route.request();
      if (request.method() !== "POST" || new URL(request.url()).pathname !== home + suffix) {
        await route.continue();
        return;
      }
      arrived();
      await gate;
      await route.fulfill({ status: 503, contentType: "text/plain", body: "Synthetic unavailable" });
    };
    await page.evaluate((selector) => {
      const form = document.querySelector(selector);
      if (!form) throw new Error("Fixture form missing");
      new MutationObserver(() => {
        if (form.getAttribute("aria-busy") === "true") window.recordWorkspaceFixtureBusy();
      }).observe(form, { attributes: true, attributeFilter: ["aria-busy"] });
    }, formSelector);
    await page.route("**/*", handler);
    try {
      await measure(name, async () => {
        const response = page.waitForResponse((value) => value.request().method() === "POST" &&
          new URL(value.url()).pathname === home + suffix);
        const click = page.locator(buttonSelector).click();
        await intercepted;
        let timer;
        const pendingBusyObserved = await Promise.race([busy.then(() => true), new Promise((resolve) => {
          timer = setTimeout(() => resolve(false), 2000);
        })]);
        clearTimeout(timer);
        release();
        await click;
        const status = (await response).status();
        assert.equal(status, 503);
        return { pendingBusyObserved, status, response: "synthetic plain-text 503" };
      });
    } finally {
      release();
      notifyBusy = () => {};
      await page.unroute("**/*", handler);
    }
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
  await measureDocumentMutationFailure("client create injected failure", "/clients",
    ".client-create form", ".client-create button[type=submit]");
  await page.goto(home + "/clients");
  await page.locator(".client-create summary").click();
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

  await page.goto(home + "/client-fields");
  await page.locator(".client-create summary").click();
  await page.locator("#client-field-label").fill("Fixture field");
  await measure("client field create", async () => {
    await page.locator(".client-create button[type=submit]").click();
    await page.getByText("Fixture field", { exact: true }).first().waitFor();
  });
  await page.goto(home + "/clients");
  await page.locator(".client-create summary").click();
  await page.locator("#client-display-name").fill("Field fixture client");
  await page.locator(".client-create button[type=submit]").click();
  await page.locator("#client-detail").waitFor();
  await page.locator(".client-value input[name=value]").fill("fixture value");
  await measure("client field value set", async () => {
    await page.locator(".client-value button[type=submit]").click();
    await page.locator(".client-value-clear").waitFor();
  });

  await page.goto(home + "/settings/invitations");
  await page.locator("#invite-email").fill("fixture@example.invalid");
  await measureDocumentMutationFailure("invitation create injected failure", "/settings/invitations",
    'form[data-persist="invitation-create"]', 'button[value="create"]');
  await page.goto(home + "/settings/invitations");
  await page.locator("#invite-email").fill("fixture@example.invalid");
  await page.locator('button[value="create"]').click();
  await page.locator('button[value="resend"]').waitFor();
  await measure("invitation resend", async () => {
    await page.locator('button[value="resend"]').click();
    await page.locator('button[value="revoke"]').waitFor();
  });

  const invite = await page.locator('input[readonly]').inputValue();
  const memberContext = await browser.newContext({ baseURL: process.env.PLAYWRIGHT_BASE_URL,
    viewport: { width: 1280, height: 900 }, locale: "en-US", serviceWorkers: "block" });
  const member = await memberContext.newPage();
  try {
    try {
      await member.goto(invite);
      await member.locator("#invite-password").fill("synthetic-password-123");
      await member.locator('button[value="register"]').click();
      await member.locator("#item-list").waitFor();
    } catch {
      throw new Error("Synthetic member registration failed; invitation details withheld");
    }
  } catch (error) {
    await memberContext.close();
    throw error;
  }
  await page.goto(home + "/settings/members");
  const memberRow = page.locator(".list-view-table tbody tr").filter({ hasText: "fixture@example.invalid" });
  await memberRow.locator("select[name=role]").selectOption("viewer");
  await measureDocumentMutationFailure("member role injected failure", "/settings/members",
    '.list-view-table form[data-webmcp-actions]', '.list-view-table button[value="role"]');
  await page.goto(home + "/settings/members");
  await memberRow.locator("select[name=role]").selectOption("viewer");
  await measure("member role change", async () => {
    await memberRow.locator('button[value="role"]').click();
    await memberRow.locator('select[name="role"]').waitFor();
    assert.equal(await memberRow.locator('select[name="role"]').inputValue(), "viewer");
  });
  const memberCDP = await memberContext.newCDPSession(member);
  await memberCDP.send("Network.enable");
  const stopPermission = observeNetwork(memberCDP);
  const permissionStart = performance.now();
  try {
    const response = await member.goto(home + "/settings/invitations", { waitUntil: "load" });
    assert.equal(response.status(), 403);
    results.push({ name: "viewer invitations forbidden", elapsedMs: Math.round((performance.now() - permissionStart) * 10) / 10,
      status: response.status(), ...stopPermission() });
  } finally { stopPermission(); }
  await measure("ownership transfer", async () => {
    await memberRow.locator('button[value="transfer"]').click();
    await memberRow.locator('button[value="transfer"]').waitFor({ state: "detached" });
  });
  member.on("dialog", (dialog) => dialog.accept());
  await member.goto(home + "/settings/members");
  const formerOwner = member.locator(".list-view-table tbody tr").filter({ hasText: process.env.PLAYWRIGHT_EMAIL });
  await formerOwner.locator('button[value="transfer"]').click();
  await formerOwner.locator('button[value="transfer"]').waitFor({ state: "detached" });
  await memberContext.close();
  await page.goto(home + "/settings/members");
  await measure("member removal", async () => {
    await memberRow.locator('button[value="remove"]').click();
    await memberRow.waitFor({ state: "detached" });
  });

  await page.goto(home + "/items?limit=25");
  for (let index = 0; index < 26; index++) {
    const title = `Pagination fixture ${index}`;
    await page.getByLabel("New item title").fill(title);
    await page.getByRole("button", { name: "Add item" }).click();
    await page.locator(`.item-list-view input[value="${title}"]`).waitFor();
  }
  await measure("item next page", async () => {
    await page.getByRole("link", { name: "Next page" }).click();
    assert.equal(await page.locator(".item-list-view tbody tr").count() > 0, true);
  });
  await page.goto(home + "/items");
  await page.getByLabel("New item title").fill("Failed item fixture");
  let releaseItemFailure;
  const itemFailureGate = new Promise((resolve) => { releaseItemFailure = resolve; });
  await page.route("**/items", async (route) => {
    if (route.request().method() === "POST") {
      await itemFailureGate;
      await route.fulfill({ status: 503, body: "unavailable" });
    }
    else await route.continue();
  });
  await measure("item injected failure", async () => {
    await page.getByRole("button", { name: "Add item" }).click();
    await page.locator('form.create[aria-busy="true"] button:disabled').waitFor();
    releaseItemFailure();
    await page.getByText("Something went wrong. Try again.").first().waitFor();
    return { pendingBusyObserved: true,
      inputPreserved: await page.getByLabel("New item title").inputValue() === "Failed item fixture" };
  });
  await page.unroute("**/items");
  await page.route("**/items/events", (route) => route.abort());
  await measure("item SSE reconnect", async () => {
    await page.goto(home + "/items");
    await page.getByText("Live updates reconnecting…").waitFor();
    await page.unroute("**/items/events");
    const connected = await page.getByText("Live updates connected.").waitFor({ timeout: 10000 }).then(() => true, () => false);
    return { connectedWithin10Seconds: connected };
  });
  await measure("item SSE offline recovery", async () => {
    await page.goto(home + "/items");
    const initiallyConnected = await page.getByText("Live updates connected.").waitFor({ timeout: 30000 }).then(() => true, () => false);
    if (!initiallyConnected) return { initiallyConnected, recovered: null };
    await context.setOffline(true);
    let reconnecting;
    try {
      reconnecting = await page.getByText("Live updates reconnecting…").waitFor({ timeout: 10000 }).then(() => true, () => false);
    } finally { await context.setOffline(false); }
    if (!reconnecting) return { initiallyConnected, reconnecting, recovered: null };
    const recovered = await page.getByText("Live updates connected.").waitFor({ timeout: 30000 }).then(() => true, () => false);
    return { initiallyConnected, reconnecting, recovered };
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
