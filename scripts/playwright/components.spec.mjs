import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import axe from "axe-core";
import { expect, test } from "@playwright/test";

let fixture;
let directory;
test.beforeAll(() => {
  directory = mkdtempSync(join(tmpdir(), "templ-list-"));
  const path = join(directory, "fixture.html");
  execFileSync("go", ["test", "./internal/web", "-run", "^TestResponsiveListBrowserFixture$", "-count=1"], {
    env: { ...process.env, COMPONENT_FIXTURE: path },
  });
  fixture = readFileSync(path, "utf8");
});
test.afterAll(() => rmSync(directory, { recursive: true, force: true }));

for (const [name, width, theme, visible, hidden] of [
  ["mobile light", 390, "light", "card-action", "table-action"],
  ["mobile dark", 390, "dark", "card-action", "table-action"],
  ["desktop light", 1280, "light", "table-action", "card-action"],
  ["desktop dark", 1280, "dark", "table-action", "card-action"],
]) {
  test(`${name} list exposes one keyboard action and keeps standalone table`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.setContent(fixture);
    await page.locator("html").evaluate((element, value) => element.setAttribute("data-theme", value), theme);
    await page.addStyleTag({ content: readFileSync("internal/web/assets/components.css", "utf8") });
    await page.addStyleTag({ content: readFileSync("internal/web/assets/app.css", "utf8") });
    await expect(page.locator(`#${visible}`)).toBeVisible();
    await expect(page.locator(`#${hidden}`)).toBeHidden();
    await expect(page.locator("#standalone-action")).toBeVisible();
    await expect(page.getByRole("button", { name: "Open item" })).toHaveCount(1);
    await page.locator("#before").focus();
    await page.keyboard.press("Tab");
    await expect(page.locator(`#${visible}`)).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(page.locator("#standalone-action")).toBeFocused();
    await page.addScriptTag({ content: axe.source });
    const results = await page.evaluate(async () => axe.run(document));
    expect(results.violations).toEqual([]);
  });
}
