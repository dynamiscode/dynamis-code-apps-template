import { expect, test } from "@playwright/test";
import { execFileSync } from "node:child_process";

const email = process.env.PLAYWRIGHT_EMAIL;
const password = process.env.PLAYWRIGHT_PASSWORD;
test.setTimeout(30_000);

async function openNotifications(page) {
  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(new URL("/", process.env.PLAYWRIGHT_BASE_URL).href);
  await page.goto("/notifications");
  await expect(page.getByRole("heading", { name: "Notifications", exact: true })).toBeVisible();
}

const screenshots = [
  { name: "desktop-light", width: 1280, height: 900, colorScheme: "light" },
  { name: "desktop-dark", width: 1280, height: 900, colorScheme: "dark" },
  { name: "mobile-light", width: 390, height: 844, colorScheme: "light" },
  { name: "mobile-dark", width: 390, height: 844, colorScheme: "dark" },
];

for (const screenshot of screenshots) {
  test(`notifications empty screenshot: ${screenshot.name}`, async ({ page }) => {
    await page.setViewportSize({ width: screenshot.width, height: screenshot.height });
    await page.emulateMedia({ colorScheme: screenshot.colorScheme, reducedMotion: "reduce" });
    await openNotifications(page);
    await expect(page.locator(".notification-empty")).toBeVisible();
    const heading = await page.getByRole("heading", { name: "Notifications", exact: true }).boundingBox();
    const description = await page.locator(".notification-description").boundingBox();
    expect(description.y).toBeGreaterThanOrEqual(heading.y + heading.height);
    await expect(page).toHaveScreenshot(`notifications-empty-${screenshot.name}.png`, {
      animations: "disabled",
      fullPage: true,
    });
  });
}

test("notifications keep keyboard and Spanish no-JavaScript navigation", async ({ page }) => {
  await openNotifications(page);
  await page.keyboard.press("Tab");
  await expect(page.locator(".skip-link")).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/#main$/);

  await page.goto("/language?locale=es&return_to=%2Fnotifications");
  await expect(page.locator("html")).toHaveAttribute("lang", "es");
  await expect(page.getByRole("heading", { name: "Notificaciones", exact: true })).toBeVisible();
  await page.getByRole("link", { name: "Actualizar notificaciones" }).click();
  await expect(page.getByRole("heading", { name: "Notificaciones", exact: true })).toBeVisible();
});

test.describe("notifications fallback without JavaScript", () => {
  test.use({ javaScriptEnabled: false });

  test("shows the empty state and reload link in Spanish", async ({ page }) => {
    await openNotifications(page);
    await page.goto("/language?locale=es&return_to=%2Fnotifications");
    await expect(page.locator("html")).toHaveAttribute("lang", "es");
    await expect(page.getByRole("heading", { name: "No hay notificaciones." })).toBeVisible();
    await page.getByRole("link", { name: "Actualizar notificaciones" }).click();
    await expect(page.getByRole("heading", { name: "No hay notificaciones." })).toBeVisible();
  });

  test("marks a notification read without JavaScript", async ({ page }) => {
    test.skip(process.env.PLAYWRIGHT_NOTIFICATION_SEEDED !== "1", "requires disposable SQLite fixture");
    await openNotifications(page);
    await expect(page.getByRole("heading", { name: "Playwright notification" })).toBeVisible();
    await page.getByRole("button", { name: "Mark as read" }).click();
    await expect(page.getByText("read", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Mark as read" })).toHaveCount(0);
  });
});

test("replays a gap after native lifetime reconnect", async ({ page }) => {
  test.skip(process.env.PLAYWRIGHT_NOTIFICATION_RECONNECT !== "1", "requires disposable SQLite and short SSE lifetime");
  test.setTimeout(90_000);
  await page.exposeFunction("notificationLifetimeClosed", () => {
    const sql = `INSERT INTO notifications (user_id, workspace_id, notification_type, title, body, created_at)
      SELECT u.id, wm.workspace_id, 'system', 'Reconnect after lifetime', 'Replay body',
             strftime('%Y-%m-%dT%H:%M:%fZ','now')
      FROM users u JOIN workspace_members wm ON wm.user_id=u.id
      WHERE u.email='playwright@example.com' LIMIT 1;`;
    execFileSync("sqlite3", ["-batch", "-bail", process.env.PLAYWRIGHT_SQLITE_PATH, sql], {
      env: { PATH: process.env.PATH },
      stdio: "pipe",
    });
  });
  await page.addInitScript(() => {
    const NativeEventSource = window.EventSource;
    window.EventSource = class extends NativeEventSource {
      constructor(...args) {
        super(...args);
        this.addEventListener("close", () => window.notificationLifetimeClosed());
      }
    };
  });
  await openNotifications(page);
  await expect(page.locator("[data-notification-cursor]")).toHaveAttribute("data-notification-cursor", "");
  await expect(page.locator("[data-unread-count]")).toHaveText("0");
  await expect(page.locator("[data-unread-count]")).toHaveText("1", { timeout: 75_000 });
});
