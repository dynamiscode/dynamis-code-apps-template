import http from "node:http";
import axe from "axe-core";
import { expect, test } from "@playwright/test";

const email = process.env.PLAYWRIGHT_EMAIL;
const password = process.env.PLAYWRIGHT_PASSWORD;

async function openWorkspace(page) {
  if (!email || !password) throw new Error("PLAYWRIGHT_EMAIL and PLAYWRIGHT_PASSWORD are required");
  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(new URL("/", process.env.PLAYWRIGHT_BASE_URL).href);
  await page.locator(".cards a").first().click();
  await expect(page.locator("[data-workspace-home]")).toBeVisible();
  return new URL(page.url()).pathname;
}

async function createWebhook(page, workspacePath, name, url) {
  await page.goto(`${workspacePath}/settings/webhooks`);
  await page.getByLabel("Name").fill(name);
  await page.getByLabel("HTTPS endpoint").fill(url);
  await page.locator('input[name="events"][value="item.created"]').check();
  const responsePromise = page.waitForResponse((response) =>
    response.url().includes("/settings/webhooks") && response.request().method() === "POST");
  await page.getByRole("button", { name: "Create webhook" }).click();
  const response = await responsePromise;
  expect(response.status()).toBe(200);
  expect(response.headers()["cache-control"]).toBe("no-store");
  await expect(page.locator(".webhook-secret")).toBeVisible();
  return page.locator(".webhook-secret").innerText();
}

async function expectAccessible(page, label) {
  const axeURL = new URL("/__axe.js", page.url()).href;
  await page.route(axeURL, (route) => route.fulfill({ contentType: "application/javascript", body: axe.source }));
  try {
    await page.addScriptTag({ url: axeURL });
    const violations = await page.evaluate(async () => {
      const result = await window.axe.run(document, {
        runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"] },
      });
      return result.violations.map(({ id, impact, nodes }) => ({ id, impact, count: nodes.length }));
    });
    expect(violations, `${label} accessibility violations`).toEqual([]);
  } finally {
    await page.unroute(axeURL);
  }
}

async function listenForDeliveries() {
  const receiver = http.createServer((request, response) => {
    request.resume();
    response.writeHead(204).end();
  });
  await new Promise((resolve) => receiver.listen(0, "127.0.0.1", resolve));
  return receiver;
}

test("webhook registry and delivery history cover responsive, themed, keyboard and redacted states", async ({ page }) => {
  test.setTimeout(60_000);
  const workspacePath = await openWorkspace(page);
  const receiver = await listenForDeliveries();
  try {
    const endpoint = `http://127.0.0.1:${receiver.address().port}/hook`;
    const firstSecret = await createWebhook(page, workspacePath, "Browser fixture", endpoint);
    expect(firstSecret).toMatch(/^whsec_/);
    await expectAccessible(page, "webhook secret state");

    for (const view of [
      { name: "desktop-light", width: 1280, height: 900, colorScheme: "light" },
      { name: "desktop-dark", width: 1280, height: 900, colorScheme: "dark" },
      { name: "mobile-light", width: 390, height: 844, colorScheme: "light" },
      { name: "mobile-dark", width: 390, height: 844, colorScheme: "dark" },
    ]) {
      await page.setViewportSize({ width: view.width, height: view.height });
      await page.emulateMedia({ colorScheme: view.colorScheme, reducedMotion: "reduce" });
      await page.goto(`${workspacePath}/settings/webhooks`);
      await expect(page.locator(".webhook-card")).toContainText("Browser fixture");
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
      await expect(page).toHaveScreenshot(`webhooks-${view.name}.png`, {
        animations: "disabled",
        fullPage: true,
        mask: [page.locator(".webhook-endpoint")],
      });
    }

    await page.goto(`${workspacePath}/settings/webhooks`);
    await page.keyboard.press("Tab");
    await expect(page.locator(".skip-link")).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(/#main$/);
    await page.getByLabel("Name").focus();
    await page.keyboard.press("Tab");
    await expect(page.getByLabel("HTTPS endpoint")).toBeFocused();

    const firstCard = page.locator(".webhook-card").filter({ hasText: "Browser fixture" });
    const secretOnList = firstCard.locator(".webhook-secret");
    expect(await secretOnList.count()).toBe(0);
    await firstCard.getByLabel(/Delete this webhook/).check();
    await firstCard.getByRole("button", { name: "Delete webhook" }).click();
    await expect(firstCard).toHaveCount(0);

    const secret = await createWebhook(page, workspacePath, "Delivery fixture", endpoint);
    expect(secret).toMatch(/^whsec_/);
    await expect(page.locator(".webhook-secret")).toContainText(secret);
    await page.goto(`${workspacePath}/settings/webhooks`);
    await expect(page.locator(".webhook-secret")).toHaveCount(0);
    await expect(page.locator(".webhook-card").filter({ hasText: secret })).toHaveCount(0);
    const deliveryPath = await page.locator(".webhook-card").filter({ hasText: "Delivery fixture" })
      .locator('a[href$="/deliveries"]').getAttribute("href");
    expect(deliveryPath).toBeTruthy();

    await page.goto(`${workspacePath}/items`);
    await page.getByLabel("New item title").fill("WEBHOOK_PRIVATE_PAYLOAD_FIXTURE");
    await page.getByRole("button", { name: "Add item" }).click();
    await expect(page.locator('#item-list input[value="WEBHOOK_PRIVATE_PAYLOAD_FIXTURE"]')).toBeVisible();

    const historyResponse = await page.goto(deliveryPath);
    await expect(page.locator("main table tbody tr")).toHaveCount(1);
    expect(historyResponse.headers()["cache-control"]).toBe("no-store");
    await expect(page.locator("main table")).toContainText("item.created");
    const historyText = await page.locator("main").innerText();
    expect(historyText).not.toContain(secret);
    expect(historyText).not.toContain("WEBHOOK_PRIVATE_PAYLOAD_FIXTURE");
    await expectAccessible(page, "webhook delivery history");
    await page.setViewportSize({ width: 390, height: 844 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
    const tableScroll = page.locator(".table-scroll");
    expect(await tableScroll.evaluate((element) => getComputedStyle(element).overflowX)).toMatch(/auto|scroll/);

    await page.goto(`${workspacePath}/settings/webhooks`);
    const deliveryFixture = page.locator(".webhook-card").filter({ hasText: "Delivery fixture" });
    await deliveryFixture.getByLabel(/Delete this webhook/).check();
    await deliveryFixture.getByRole("button", { name: "Delete webhook" }).click();
    await expect(deliveryFixture).toHaveCount(0);
  } finally {
    await new Promise((resolve) => receiver.close(resolve));
  }
});

test.describe("webhook delete without JavaScript", () => {
  test.use({ javaScriptEnabled: false });

  test("requires the native confirmation checkbox before deletion", async ({ page }) => {
    test.setTimeout(30_000);
    const workspacePath = await openWorkspace(page);
    const fixtureName = `No-JS fixture ${Date.now()}`;
    await createWebhook(page, workspacePath, fixtureName, "http://localhost:58123/hook");
    await page.goto(`${workspacePath}/settings/webhooks`);
    const card = page.locator(".webhook-card").filter({ hasText: fixtureName });
    const historyPath = await card.locator('a[href$="/deliveries"]').getAttribute("href");
    const emptyHistory = await page.goto(historyPath);
    expect(emptyHistory.headers()["cache-control"]).toBe("no-store");
    await expect(page.getByText("No deliveries yet.")).toBeVisible();
    await page.goto(`${workspacePath}/settings/webhooks`);
    const reloadedCard = page.locator(".webhook-card").filter({ hasText: fixtureName });
    const confirm = reloadedCard.getByRole("checkbox");
    expect(await confirm.evaluate((element) => element.required)).toBe(true);
    expect(await confirm.evaluate((element) => element.checkValidity())).toBe(false);
    await reloadedCard.getByRole("button", { name: "Delete webhook" }).click();
    await expect(reloadedCard).toBeVisible();
    await confirm.check();
    await Promise.all([
      page.waitForNavigation({ waitUntil: "load" }),
      reloadedCard.getByRole("button", { name: "Delete webhook" }).click(),
    ]);
    await expect(reloadedCard).toHaveCount(0);
  });
});
