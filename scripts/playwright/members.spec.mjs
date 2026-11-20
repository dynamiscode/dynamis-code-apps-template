import { expect, test } from "@playwright/test";
import axe from "axe-core";

const email = process.env.PLAYWRIGHT_EMAIL;
const password = process.env.PLAYWRIGHT_PASSWORD;
const baseURL = process.env.PLAYWRIGHT_MEMBERS_BASE_URL || process.env.PLAYWRIGHT_BASE_URL;

async function openWorkspaceMembers(page) {
	let loginResponse;
	let loginNavigationFailed = false;
	try {
		loginResponse = await page.goto(new URL("/login", baseURL).href);
	} catch {
		loginNavigationFailed = true;
	}
	const emailField = page.getByLabel("Email");
	const loginFormCount = await page.locator('form[action="/login"]').count();
	const emailFieldCount = await emailField.count();
	const passwordFieldCount = await page.getByLabel("Password").count();
	const title = await page.title().catch(() => "");
	const emailFieldVisible = emailFieldCount === 1 && await emailField.isVisible();
	if (loginNavigationFailed || loginFormCount !== 1 || emailFieldCount !== 1 || passwordFieldCount !== 1 || !emailFieldVisible) {
		throw new Error(`Members login page unexpected (HTTP ${loginResponse?.status() ?? 0}; title ${JSON.stringify(title)}; forms ${loginFormCount}; email fields ${emailFieldCount}; password fields ${passwordFieldCount}; email visible ${emailFieldVisible})`);
	}
	await emailField.fill(email);
	await page.getByLabel("Password").fill(password);
	await page.getByRole("button", { name: "Sign in" }).click();
	await page.waitForURL(new URL("/", baseURL).href);
	const workspaceLink = page.locator('.cards a[href^="/workspaces/"]').filter({ hasText: "UI Baselines" }).first();
	const workspacePath = new URL(await workspaceLink.getAttribute("href"), baseURL).pathname;
	await page.goto(new URL(`${workspacePath}/settings/members`, baseURL).href);
	return workspacePath;
}

test.use({ trace: "off" });

test.describe("members route", () => {
	for (const [name, width, theme] of [
		["desktop light", 1280, "light"],
		["desktop dark", 1280, "dark"],
		["mobile light", 390, "light"],
		["mobile dark", 390, "dark"],
	]) {
		test(`${name} renders accessible member management`, async ({ page }) => {
			await page.setViewportSize({ width, height: 900 });
			await page.emulateMedia({ colorScheme: theme, reducedMotion: "reduce" });
			await openWorkspaceMembers(page);
			await expect(page.getByRole("heading", { name: /Members/ })).toBeVisible();
			if (width < 608) {
				await expect(page.locator(".list-view-cards")).toBeVisible();
			} else {
				await expect(page.locator(".list-view-table")).toBeVisible();
			}
			await expect(page.locator("html")).toHaveAttribute("data-theme", "system");
			const renderedTheme = await page.locator("html").evaluate((element) => getComputedStyle(element).colorScheme);
			expect(renderedTheme).toBe(theme);
			const overflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
			expect(overflow).toBe(false);
			const maskColor = await page.locator("body").evaluate((body) => getComputedStyle(body).backgroundColor);
			const platformSuffix = process.platform === "linux" ? "-linux" : "";
			await expect(page).toHaveScreenshot(`members-${name.replaceAll(" ", "-")}${platformSuffix}.png`, {
				fullPage: true,
				mask: [page.locator('[data-column="id"]')],
				maskColor,
			});
		});
	}

	test("supports separate member actions, confirmations, permissions, and Spanish", async ({ page, browser }) => {
		const workspacePath = await openWorkspaceMembers(page);
		await page.addInitScript(() => {
			if (document.modelContext) return;
			Object.defineProperty(document, "__membersWebMCPTestShim", { value: true });
			const tools = [];
			Object.defineProperty(document, "modelContext", { value: {
				registerTool(tool) { tools.push(tool); },
				getTools() { return Promise.resolve(tools); },
				executeTool(tool, input) { return Promise.resolve(tool.execute(JSON.parse(input))); },
			} });
		});
		await page.reload();
		const membersPath = `${workspacePath}/settings/members`;

		async function inviteMember(memberEmail, role) {
			await page.goto(new URL(`${workspacePath}/settings/invitations`, baseURL).href);
			await page.getByLabel("Email").fill(memberEmail);
			await page.getByLabel("Role").selectOption(role);
			await page.locator('button[name="action"][value="create"]').click();
			const inviteLink = await page.locator('input[aria-label="Copy this invitation link"]').evaluate((input) => {
				const value = input.value;
				input.value = "";
				return value;
			});
			await page.goto(new URL(membersPath, baseURL).href);
			const memberContext = await browser.newContext({ baseURL });
			const memberPage = await memberContext.newPage();
			let step = "open invitation";
			let status = 0;
			let passwordCount = 0;
			try {
				const response = await memberPage.goto(new URL(inviteLink, baseURL).href);
				status = response?.status() ?? 0;
				step = "enter password";
				passwordCount = await memberPage.locator("#invite-password").count();
				await memberPage.locator("#invite-password").fill("members-fixture-password-123");
				step = "register member";
				await memberPage.locator('button[name="action"][value="register"]').click();
			} catch {
				await memberContext.close();
				throw new Error(`member browser fixture failed at ${step} (GET ${status}, password fields ${passwordCount})`);
			}
			return { context: memberContext, page: memberPage };
		}

		const removed = await inviteMember("removed@example.com", "member");
		const transferred = await inviteMember("transferred@example.com", "member");
		await page.goto(new URL(membersPath, baseURL).href);
		const targetRow = (memberEmail) => page.locator(".list-view-table tbody tr").filter({ hasText: memberEmail });
		await expect(targetRow("removed@example.com")).toBeVisible();
		await expect(targetRow("transferred@example.com")).toBeVisible();
		await expect(page.locator('form[data-webmcp-actions="member-role-update"][data-webmcp-id]')).toHaveCount(6);
		await expect(page.locator('form[data-webmcp-actions="member-remove"][data-webmcp-id]')).toHaveCount(6);
		await expect(page.locator('form[data-webmcp-actions="ownership-transfer"][data-webmcp-id]')).toHaveCount(6);
		await page.setViewportSize({ width: 390, height: 844 });
		await expect(page.locator(".list-view-cards")).toBeVisible();
		await page.emulateMedia({ colorScheme: "dark", reducedMotion: "reduce" });
		await test.info().attach("members-actions-mobile-dark", {
			body: await page.screenshot({ fullPage: true }), contentType: "image/png",
		});
		await page.route("**/members-test/axe.min.js", (route) => route.fulfill({
			contentType: "application/javascript",
			body: axe.source,
		}));
		await page.addScriptTag({ url: new URL("/members-test/axe.min.js", baseURL).href });
		const axeResult = await page.evaluate(() => window.axe.run(document, {
			runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa"] },
		}));
		expect(axeResult.violations).toEqual([]);
		await page.setViewportSize({ width: 1280, height: 900 });
		const roleForm = targetRow("removed@example.com").locator('form[data-webmcp-actions="member-role-update"]');
		const roleSelect = roleForm.getByLabel("Role");
		const roleMemberID = await roleForm.locator('input[name="member_id"]').inputValue();
		const removeCheckbox = targetRow("removed@example.com").locator('input[name="confirm_remove"]');
		const consequenceID = await removeCheckbox.getAttribute("aria-describedby");
		expect(consequenceID).toBeTruthy();
		await expect(page.locator(`#${consequenceID}`)).toBeVisible();
		const prepared = await page.evaluate(async ({ memberId }) => {
			const context = document.modelContext;
			const tools = await context.getTools();
			const tool = tools.find((candidate) => candidate.name === "member-role-update-v1");
			let submitted = false;
			const form = [...document.querySelectorAll('form[data-webmcp-actions="member-role-update"]')]
				.find((candidate) => candidate.getClientRects().length > 0 && candidate.dataset.webmcpId === memberId);
			form.addEventListener("submit", () => { submitted = true; }, { once: true });
			const result = await context.executeTool(tool, JSON.stringify({ memberId, role: "viewer" }));
			return { result, selected: form.querySelector('select[name="role"]').value, focused: document.activeElement === form.querySelector('button[value="role"]'), submitted, native: !document.__membersWebMCPTestShim };
		}, { memberId: roleMemberID });
		expect(prepared).toMatchObject({ result: { tool: "member-role-update-v1", status: "ready-for-user-submission" }, selected: "viewer", focused: true, submitted: false });
		await test.info().attach("members-webmcp-execution", {
			body: prepared.native ? "Executed through browser-native WebMCP." : "Browser-native WebMCP unavailable; executed the registered production tool through a test shim.",
			contentType: "text/plain",
		});
		const invalidRole = await page.evaluate(async ({ path, memberId, csrf }) => {
			const body = new URLSearchParams({ csrf, member_id: memberId, action: "role", role: "owner" });
			return (await fetch(path, { method: "POST", body })).status;
		}, {
			path: membersPath,
			memberId: roleMemberID,
			csrf: await roleForm.locator('input[name="csrf"]').inputValue(),
		});
		expect(invalidRole).toBe(422);
		await roleSelect.focus();
		await page.keyboard.press("Tab");
		await expect(roleForm.getByRole("button", { name: "Change role" })).toBeFocused();
		await roleSelect.selectOption("viewer");
		await roleForm.getByRole("button", { name: "Change role" }).click();
		await expect(targetRow("removed@example.com")).toContainText("Viewer");
		const viewerPage = await removed.page.goto(new URL(membersPath, baseURL).href);
		expect(viewerPage.status()).toBe(200);
		await expect(removed.page.locator('form[data-webmcp-actions="member-role-update"]')).toHaveCount(0);
		await expect(removed.page.locator('form[data-webmcp-actions="member-remove"]')).toHaveCount(0);
		const viewerDeniedActions = await removed.page.evaluate(async ({ path, memberId, csrf }) => {
			const statuses = [];
			for (const [action, extra] of [["role", { role: "member" }], ["remove", { confirm_remove: "yes" }]]) {
				const body = new URLSearchParams({ csrf, member_id: memberId, action, ...extra });
				statuses.push((await fetch(path, { method: "POST", body })).status);
			}
			return statuses;
		}, {
			path: membersPath,
			memberId: roleMemberID,
			csrf: await removed.page.locator("form.list-view-column input[name=csrf]").first().inputValue(),
		});
		expect(viewerDeniedActions).toEqual([403, 403]);

		const csrfFailure = await page.evaluate(async ({ path, memberId }) => {
			const body = new URLSearchParams({ member_id: memberId, action: "remove", confirm_remove: "yes" });
			return (await fetch(path, { method: "POST", body })).status;
		}, { path: membersPath, memberId: await targetRow("removed@example.com").locator('form[data-webmcp-actions="member-remove"] input[name="member_id"]').inputValue() });
		expect(csrfFailure).toBe(403);

		await page.context().addCookies([{ name: "locale", value: "es", url: baseURL }]);
		await page.reload();
		await expect(page.getByText("Esto quita su acceso al espacio de trabajo y deja de compartir sus recursos con esta persona.", { exact: true }).first()).toBeVisible();

		const noJS = await browser.newContext({ baseURL, javaScriptEnabled: false });
		const noJSPage = await noJS.newPage();
		const noJSWorkspace = await openWorkspaceMembers(noJSPage);
		const noJSMembers = `${noJSWorkspace}/settings/members`;
		const rowFor = (memberEmail) => noJSPage.locator(".list-view-table tbody tr").filter({ hasText: memberEmail });
		const noJSRoleForm = rowFor("removed@example.com").locator('form[data-webmcp-actions="member-role-update"]');
		await noJSRoleForm.getByLabel("Role").selectOption("member");
		await noJSRoleForm.getByRole("button", { name: "Change role" }).click();
		await expect(rowFor("removed@example.com")).toContainText("Member");
		const removeForm = rowFor("removed@example.com").locator('form[data-webmcp-actions="member-remove"]');
		await removeForm.getByRole("button", { name: "Remove" }).click();
		await expect(rowFor("removed@example.com")).toBeVisible();
		await removeForm.getByLabel("Remove this member?").check();
		await removeForm.getByRole("button", { name: "Remove" }).click();
		await expect(rowFor("removed@example.com")).toHaveCount(0);
		const transferForm = rowFor("transferred@example.com").locator('form[data-webmcp-actions="ownership-transfer"]');
		await transferForm.getByRole("button", { name: "Transfer ownership" }).click();
		await expect(rowFor("transferred@example.com")).toBeVisible();
		await transferForm.getByLabel("Transfer ownership to this member? They become owner, and your role becomes admin.").check();
		await transferForm.getByRole("button", { name: "Transfer ownership" }).click();
		await expect(noJSPage).toHaveURL(/settings\/members/);

		const denied = await removed.page.goto(new URL(membersPath, baseURL).href);
		expect(denied.status()).toBe(404);
		await removed.context.close();
		await transferred.context.close();
		await noJS.close();
	});
});
