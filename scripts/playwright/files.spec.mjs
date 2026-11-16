import { expect, test } from "@playwright/test";
import axe from "axe-core";

const email = process.env.PLAYWRIGHT_EMAIL;
const password = process.env.PLAYWRIGHT_PASSWORD;

async function openFiles(page) {
  if (!email || !password) {
    throw new Error("PLAYWRIGHT_EMAIL and PLAYWRIGHT_PASSWORD are required");
  }
  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(new URL("/", process.env.PLAYWRIGHT_BASE_URL).href);
  await page.locator(".cards a").first().click();
  const workspacePath = new URL(page.url()).pathname;
  await page.locator(".sidebar").getByRole("link", { name: "Files", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Files", exact: true })).toBeVisible();
  return workspacePath;
}

async function visibleDownload(page, name) {
  const visibleList = await page.locator(".list-view-table").evaluate((element) =>
    getComputedStyle(element).display === "none" ? ".list-view-cards" : ".list-view-table",
  );
  return page.locator(`${visibleList} a[aria-label$="${name}"]`).first();
}

const filesScreenshots = [
  { name: "desktop-light", width: 1280, height: 900, colorScheme: "light" },
  { name: "desktop-dark", width: 1280, height: 900, colorScheme: "dark" },
  { name: "mobile-light", width: 390, height: 844, colorScheme: "light" },
  { name: "mobile-dark", width: 390, height: 844, colorScheme: "dark" },
];

for (const screenshot of filesScreenshots) {
  test(`Files screenshot: ${screenshot.name}`, async ({ page }) => {
    await page.setViewportSize({ width: screenshot.width, height: screenshot.height });
    await page.emulateMedia({ colorScheme: screenshot.colorScheme, reducedMotion: "reduce" });
    await openFiles(page);
    const canvas = await page.locator("html").evaluate((element) => getComputedStyle(element).getPropertyValue("--canvas").trim());
    expect(canvas).toBe(screenshot.colorScheme === "dark" ? "#111826" : "#f3f6fa");
    const listDisplay = await page.locator(".list-view-table").evaluate((element) => getComputedStyle(element).display);
    const cardsDisplay = await page.locator(".list-view-cards").evaluate((element) => getComputedStyle(element).display);
    expect(listDisplay).toBe(screenshot.width < 608 ? "none" : "block");
    expect(cardsDisplay).toBe(screenshot.width < 608 ? "grid" : "none");
    await expect(page).toHaveScreenshot(`files-${screenshot.name}.png`, { animations: "disabled", fullPage: true });
  });
}

test("Files keyboard and Spanish labels stay usable", async ({ page }) => {
  await openFiles(page);
  await page.keyboard.press("Tab");
  await expect(page.locator(".skip-link")).toBeFocused();
  await page.locator("#file-upload").focus();
  await page.keyboard.press("Tab");
  await expect(page.locator('.file-upload-form button[type="submit"]')).toBeFocused();

  const filesPath = new URL(page.url()).pathname;
  await page.goto(`/language?locale=es&return_to=${encodeURIComponent(filesPath)}`);
  await expect(page.locator("html")).toHaveAttribute("lang", "es");
  await expect(page.getByRole("heading", { name: "Archivos", exact: true })).toBeVisible();
  await expect(page.getByLabel("Archivo")).toBeVisible();
});

test("Files page passes automated axe checks", async ({ page }) => {
  await openFiles(page);
  await page.addScriptTag({ content: axe.source });
  const results = await page.evaluate(async () => axe.run(document));
  expect(results.violations).toEqual([]);
});

test("upload reports byte progress, recovers from failure, and retries", async ({ page }) => {
  const workspacePath = await openFiles(page);
  const uploadURL = await page.evaluate(() => {
    const form = document.querySelector("form[data-file-upload]");
    const originalXHR = window.XMLHttpRequest;
    // Capture the uploader's listener so native progress cannot race this synthetic event.
    window.XMLHttpRequest = class extends originalXHR {
      constructor() {
        super();
        const upload = this.upload;
        const addEventListener = upload.addEventListener.bind(upload);
        let reportProgress;
        upload.addEventListener = (type, listener, options) => {
          if (type === "progress") {
            reportProgress = listener;
            return;
          }
          return addEventListener(type, listener, options);
        };
        addEventListener("loadstart", () => {
          reportProgress?.call(upload, new ProgressEvent("progress", { lengthComputable: true, loaded: 512, total: 1024 }));
        });
      }
    };
    return form.action;
  });
  let failNextUpload = true;
  let releaseFailure;
  const heldFailure = new Promise((resolve) => { releaseFailure = resolve; });
  await page.route(uploadURL, async (route) => {
    if (failNextUpload) {
      failNextUpload = false;
      await heldFailure;
      await route.fulfill({
        status: 422,
        contentType: "text/html",
        body: '<div role="alert">The selected file is invalid.</div>',
      });
      return;
    }
    await route.continue();
  });
  await page.locator("#file-upload").setInputFiles({
    name: "retry-upload.txt",
    mimeType: "text/plain",
    buffer: Buffer.alloc(1024 * 1024, "x"),
  });

  const requestPromise = page.waitForRequest((request) => request.url() === uploadURL && request.method() === "POST");
  await page.locator('.file-upload-form button[type="submit"]').click();
  await requestPromise;
  await expect(page.locator('.file-upload-form button[type="submit"]')).toBeDisabled();
  await expect(page.locator("#file-upload")).toBeDisabled();
  await expect(page.locator("[data-upload-progress]")).toBeVisible();
  await expect(page.locator("[data-upload-progress]")).toHaveJSProperty("value", 50);
  await expect(page.locator("[data-upload-status]")).toContainText("Uploading file… 50%");
  releaseFailure();
  await expect(page.locator("[data-upload-status]")).toContainText("The selected file is invalid.");
  await expect(page.locator("#file-upload")).toBeEnabled();
  await expect(page.locator('.file-upload-form button[type="submit"]')).toBeEnabled();
  await expect(page.locator("[data-upload-progress]")).toBeHidden();

  const retryRequest = page.waitForRequest((candidate) => candidate.url() === uploadURL && candidate.method() === "POST");
  const retryResponse = page.waitForResponse((response) => response.url() === uploadURL && response.request().method() === "POST");
  await page.locator('.file-upload-form button[type="submit"]').click();
  await retryRequest;
  await retryResponse;
  await expect(await visibleDownload(page, "retry-upload.txt")).toBeVisible();
});

test.describe("Files without JavaScript", () => {
  test.use({ javaScriptEnabled: false });

  test("native upload, filtering, and localized validation work", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    const workspacePath = await openFiles(page);
    const options = page.locator("details.client-filters");
    if (!(await options.locator("form").isVisible())) await options.locator("summary").click();
    await page.locator("#file-upload").setInputFiles({
      name: "native-upload.txt",
      mimeType: "text/plain",
      buffer: Buffer.from("native upload"),
    });
    await page.locator('.file-upload-form button[type="submit"]').click();
    await expect(await visibleDownload(page, "native-upload.txt")).toBeVisible();

    const optionsAfterUpload = page.locator("details.client-filters");
    if (!(await optionsAfterUpload.locator("form").isVisible())) await optionsAfterUpload.locator("summary").click();
    await page.locator('input[name="search"]').fill("missing-file");
    await page.locator('.client-query button[type="submit"]').click();
    await expect(page.getByText("No files yet.", { exact: true })).toBeVisible();

    const filesPath = `${workspacePath}/files`;
    await page.goto(`/language?locale=es&return_to=${encodeURIComponent(filesPath)}`);
    await page.locator("#file-upload").setInputFiles({
      name: "unsupported.exe",
      mimeType: "application/octet-stream",
      buffer: Buffer.from("unsupported"),
    });
    await page.locator('.file-upload-form button[type="submit"]').click();
    await expect(page.getByRole("alert")).toContainText("El archivo seleccionado no es válido.");
  });
});

test("read-only viewer lists and downloads files without upload controls", async ({ browser, page }) => {
  const workspacePath = await openFiles(page);
  await page.locator("#file-upload").setInputFiles({
    name: "viewer-readable.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("private file"),
  });
  await page.locator('.file-upload-form button[type="submit"]').click();
  await expect(await visibleDownload(page, "viewer-readable.txt")).toBeVisible();

  await page.goto(`${workspacePath}/settings/invitations`);
  await page.locator("#invite-email").fill("viewer-417@example.com");
  await page.locator("#invite-role").selectOption("viewer");
  await page.locator('form[action$="/settings/invitations"] button[name="action"][value="create"]').click();
  const invitationURL = await page.getByLabel("Copy this invitation link").inputValue();

  const viewerPage = await browser.newPage();
  try {
    await viewerPage.goto(invitationURL);
    await viewerPage.locator("#invite-password").fill("viewer-long-password");
    await viewerPage.locator('button[name="action"][value="register"]').click();
    await viewerPage.waitForURL(new RegExp(`${workspacePath}/items$`));
    await viewerPage.goto(`${workspacePath}/files`);
    await expect(viewerPage.getByRole("heading", { name: "Files", exact: true })).toBeVisible();
    const downloadLink = await visibleDownload(viewerPage, "viewer-readable.txt");
    await expect(downloadLink).toBeVisible();
    await expect(viewerPage.locator("form[data-file-upload]")).toHaveCount(0);
    await expect(viewerPage.locator('input[type="file"]')).toHaveCount(0);
    await expect(viewerPage.locator('script[src="/assets/app.js"]')).toHaveCount(0);
    const [download] = await Promise.all([
      viewerPage.waitForEvent("download"),
      downloadLink.click(),
    ]);
    expect(download.suggestedFilename()).toBe("viewer-readable.txt");
  } finally {
    await viewerPage.close();
  }
});
