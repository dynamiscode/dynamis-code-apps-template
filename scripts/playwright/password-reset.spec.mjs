import { expect, test } from "@playwright/test";

const layouts = [
  { name: "desktop-light", width: 1280, height: 900, colorScheme: "light" },
  { name: "desktop-dark", width: 1280, height: 900, colorScheme: "dark" },
  { name: "mobile-light", width: 390, height: 844, colorScheme: "light" },
  { name: "mobile-dark", width: 390, height: 844, colorScheme: "dark" },
];

for (const layout of layouts) {
  test(`password recovery states: ${layout.name}`, async ({ page }) => {
    await page.setViewportSize({ width: layout.width, height: layout.height });
    await page.emulateMedia({ colorScheme: layout.colorScheme, reducedMotion: "reduce" });
    await page.goto("/password-reset");
    await expect(page.locator("html")).toHaveAttribute("data-theme", "system");
    await expect(page.locator('form[action="/password-reset"]')).toBeVisible();

    await page.keyboard.press("Tab");
    await expect(page.locator(".skip-link")).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(page.locator("#reset-email")).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(page.locator('form[action="/password-reset"] button[type="submit"]')).toBeFocused();

    if (layout.width < 608) {
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(layout.width);
    }
    await page.screenshot({ fullPage: true });

    await page.goto("/password-reset/unavailable");
    await page.locator("#reset-password").fill("first-valid-password-123");
    await page.locator("#reset-password-confirmation").fill("different-valid-password-456");
    await page.locator('form[action="/password-reset/unavailable"] button[type="submit"]').click();
    const mismatch = page.getByRole("alert");
    await expect(mismatch).toBeFocused();
    await expect(mismatch).toContainText(/passwords do not match/i);
    await expect(page.locator('form[action="/password-reset/unavailable"]')).toBeVisible();
    await expect(page.locator('input[name="csrf"]')).not.toHaveValue("");
    await page.screenshot({ fullPage: true });

    await page.locator("#reset-password-confirmation").fill("first-valid-password-123");
    await page.locator('form[action="/password-reset/unavailable"] button[type="submit"]').click();
    await expect(page.getByRole("alert")).toBeFocused();
    await expect(page.locator('form[action="/password-reset"]')).toBeVisible();
    await expect(page.getByRole("alert")).toContainText(/invalid|válido/i);
    await page.screenshot({ fullPage: true });
  });
}
