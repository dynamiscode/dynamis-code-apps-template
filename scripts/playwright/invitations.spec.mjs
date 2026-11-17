import { expect, test } from "@playwright/test";

const ownerEmail = process.env.PLAYWRIGHT_EMAIL;
const ownerPassword = process.env.PLAYWRIGHT_PASSWORD;
const memberEmail = process.env.PLAYWRIGHT_MEMBER_EMAIL;
const memberPassword = process.env.PLAYWRIGHT_MEMBER_PASSWORD;

test.use({ javaScriptEnabled: false, trace: "off", screenshot: "off", video: "off" });

test("existing account signs in and accepts invitation without JavaScript", async ({ browser, page }) => {
    if (!ownerEmail || !ownerPassword || !memberEmail || !memberPassword) {
      throw new Error("Playwright owner and member credentials are required");
    }
    await page.goto("/login");
    await page.getByLabel("Email").fill(ownerEmail);
    await page.getByLabel("Password").fill(ownerPassword);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL(new URL("/", process.env.PLAYWRIGHT_BASE_URL).href);

    const workspaceName = `Invitation handoff ${Date.now()}`;
    await page.locator("#workspace-name").fill(workspaceName);
    await page.locator('form[action="/workspaces"] button[type="submit"]').click();
    const workspaceLink = page.locator(".cards a", { hasText: workspaceName });
    await expect(workspaceLink).toBeVisible();
    await workspaceLink.click();
    const workspacePath = new URL(page.url()).pathname;

    await page.goto(`${workspacePath}/settings/invitations`);
    const createForm = page.locator('form[data-persist="invitation-create"]');
    await createForm.locator('[name="email"]').fill(memberEmail);
    await createForm.locator('[name="role"]').selectOption("member");
    await createForm.locator('button[name="action"][value="create"]').click();
    const invitationURL = await page.getByLabel("Copy this invitation link").inputValue();
    const inviteURL = new URL(invitationURL, process.env.PLAYWRIGHT_BASE_URL);
    if (!inviteURL.pathname.startsWith("/invitations/")) {
      throw new Error("Invitation link path missing");
    }

    const inviteContext = await browser.newContext({ javaScriptEnabled: false });
    try {
      const invitePage = await inviteContext.newPage();
      try {
        await invitePage.goto(inviteURL.href);
      } catch {
        throw new Error("Invitation page did not load");
      }
      const signInForm = invitePage.locator('form[action$="/sign-in"]');
      try {
        await Promise.all([
          invitePage.waitForURL((url) => url.pathname === "/login" && url.searchParams.get("return_to") === "/invitations/continue"),
          signInForm.locator('button[type="submit"]').click(),
        ]);
      } catch {
        throw new Error("Invitation sign-in did not start");
      }

      await invitePage.getByLabel("Email").fill(memberEmail);
      await invitePage.getByLabel("Password").fill(memberPassword);
      await invitePage.getByRole("button", { name: "Sign in" }).click();
      try {
        await invitePage.waitForURL((url) => url.pathname === "/invitations/continue");
      } catch {
        throw new Error("Invitation sign-in did not return to the acceptance page");
      }
      await invitePage.getByRole("button", { name: "Accept invitation" }).click();
      await invitePage.waitForURL((url) => url.pathname === `${workspacePath}/items`);
      await expect(invitePage.locator(".topbar-context")).toContainText(workspaceName);
    } finally {
      await inviteContext.close();
    }
});
