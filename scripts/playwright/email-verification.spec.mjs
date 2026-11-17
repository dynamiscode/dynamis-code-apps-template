import { expect, test } from "@playwright/test";
import axe from "axe-core";

const copy = {
  en: {
    heading: "Email verification",
    error: "The email verification link is invalid or expired.",
  },
  es: {
    heading: "Verificación de correo",
    error: "El enlace de verificación de correo no es válido o venció.",
  },
};

const layouts = ["desktop", "mobile"].flatMap((viewport) =>
  ["light", "dark", "system"].flatMap((theme) => ["en", "es"].map((locale) => ({
    name: `${viewport}-${theme}-${locale}`,
    width: viewport === "desktop" ? 1280 : 390,
    height: viewport === "desktop" ? 900 : 844,
    theme,
    colorScheme: theme === "system" ? "dark" : theme,
    locale,
  }))),
);

for (const layout of layouts) {
  test(`email verification failure: ${layout.name}`, async ({ page }) => {
    await page.setViewportSize({ width: layout.width, height: layout.height });
    await page.emulateMedia({ colorScheme: layout.colorScheme, reducedMotion: "reduce" });
    await page.context().addCookies([{ name: "theme", value: layout.theme, url: process.env.PLAYWRIGHT_BASE_URL }]);
    await page.goto(`/language?locale=${layout.locale}&return_to=%2Flogin`);
    const response = await page.goto("/account/verify-email/unavailable");

    expect(response.status()).toBe(422);
    await expect(page.locator("html")).toHaveAttribute("lang", layout.locale);
    await expect(page.locator("html")).toHaveAttribute("data-theme", layout.theme);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(copy[layout.locale].heading);
    await expect(page.getByRole("alert")).toContainText(copy[layout.locale].error);
    await expect(page.locator('a[href="/login"]')).toBeVisible();

    await expect(page.getByRole("alert")).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(page.locator('a[href="/login"]')).toBeFocused();
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(layout.width);

    const axeURL = new URL("/__test/axe.js", process.env.PLAYWRIGHT_BASE_URL).href;
    await page.route(axeURL, (route) => route.fulfill({
      contentType: "application/javascript",
      body: axe.source,
    }));
    await page.addScriptTag({ url: axeURL });
    const violations = await page.evaluate(async () => axe.run(document));
    expect(violations.violations).toEqual([]);
  });
}
