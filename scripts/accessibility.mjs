import axe from "axe-core";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { Builder, By, Key, until } from "selenium-webdriver";
import chrome from "selenium-webdriver/chrome.js";

const baseURL = process.env.A11Y_BASE_URL;
const email = process.env.A11Y_EMAIL;
const password = process.env.A11Y_PASSWORD;
const locale = process.env.A11Y_LOCALE || "en";

if (!baseURL || !email || !password || !["en", "es"].includes(locale)) {
  console.error("A11Y_BASE_URL, A11Y_EMAIL, A11Y_PASSWORD, and A11Y_LOCALE=en|es are required");
  process.exit(2);
}

const driver = await new Builder()
  .forBrowser("chrome")
  .setChromeOptions(new chrome.Options().addArguments("--headless=new", "--window-size=1280,900"))
  .build();

let failed = false;
const importFixtureDir = mkdtempSync(join(tmpdir(), "dynamis-import-preview-"));
const importFixturePath = join(importFixtureDir, "items.csv");
writeFileSync(importFixturePath, "title,status\nAccessibility preview,active\n");
try {
  await driver.get(`${baseURL}/language?locale=${locale}&return_to=/login`);
  await audit("login");
	await expectFormActionSpacing("#password", 'form[action="/login"] button[type="submit"]', "login");
	await driver.findElement(By.css("body")).sendKeys(Key.TAB);
	if ((await driver.switchTo().activeElement().getAttribute("class")) !== "skip-link") {
		throw new Error("first Tab does not reach the skip link");
	}
	await driver.actions().sendKeys(Key.TAB).perform();
	await expectActive("email");
	await driver.actions().sendKeys(Key.TAB).perform();
	await expectActive("password");
	await driver.actions().sendKeys(Key.TAB).perform();
	const reveal = driver.switchTo().activeElement();
	if ((await reveal.getTagName()) !== "button" || (await reveal.getAttribute("type")) !== "button") {
		throw new Error("login keyboard order does not reach the password reveal control");
	}
	await driver.actions().sendKeys(Key.TAB).perform();
	const submit = driver.switchTo().activeElement();
	if ((await submit.getTagName()) !== "button" || (await submit.getAttribute("type")) !== "submit") {
		throw new Error("login keyboard order does not reach submit after password reveal");
	}
	await driver.actions().sendKeys(Key.TAB).perform();
	const reset = driver.switchTo().activeElement();
	if ((await reset.getTagName()) !== "a" || new URL(await reset.getAttribute("href")).pathname !== "/password-reset") {
		throw new Error("login keyboard order does not reach the password reset link");
	}
  await driver.findElement(By.id("email")).sendKeys(email);
  await driver.findElement(By.id("password")).sendKeys(password);
  await driver.findElement(By.css('button[type="submit"]')).click();
  await driver.wait(until.urlIs(`${baseURL}/`), 5000);
  await audit("workspaces");
  await expectFormActionSpacing("#workspace-locale", 'form[action="/workspaces"] button[type="submit"]', "workspace creation");
  const workspacePath = new URL(await driver.findElement(By.css(".cards a")).getAttribute("href"), baseURL).pathname;
  await driver.findElement(By.css(".cards a")).click();
  await driver.wait(until.elementLocated(By.css("[data-workspace-home]")), 5000);
  await audit("workspace");
  const workspaceURL = new URL(await driver.getCurrentUrl());
  await driver.get(`${workspaceURL.origin}${workspaceURL.pathname}/settings/members`);
  await driver.wait(until.elementLocated(By.id("member-list")), 5000);
  await audit("members");
  await driver.get(`${workspaceURL.origin}${workspaceURL.pathname}/settings/invitations`);
  await driver.wait(until.elementLocated(By.id("invite-email")), 5000);
  await audit("invitations");
  await driver.get(`${baseURL}/notifications`);
  await driver.wait(until.elementLocated(By.css("main h1")), 5000);
  await audit("notifications");
  await driver.get(`${baseURL}${workspacePath}/settings/general`);
  await driver.wait(until.elementLocated(By.id("workspace-locale")), 5000);
  await audit("general settings");
  for (const [settingsPath, settingsName] of [
    ["email", "email settings"],
    ["tokens", "API tokens"],
    ["webhooks", "webhooks"],
    ["provisioning", "SCIM provisioning"],
    ["audit", "audit history"],
  ]) {
    console.log(`checking ${settingsName} (${locale})`);
    await driver.get(`${workspaceURL.origin}${workspaceURL.pathname}/settings/${settingsPath}`);
    try {
      await driver.wait(until.elementLocated(By.css("main h1")), 5000);
    } catch (error) {
      console.error(`failed URL: ${await driver.getCurrentUrl()}\n${await driver.findElement(By.css("body")).getText()}`);
      throw error;
    }
    await audit(settingsName);
  }
  await driver.get(`${baseURL}${workspacePath}/settings/general`);
  await driver.wait(until.elementLocated(By.id("workspace-locale")), 5000);
  await expectFormActionSpacing("#workspace-locale", 'form[action$="/settings/general"] button[type="submit"]', "general settings");
  await driver.manage().window().setRect({ width: 320, height: 800 });
  const settingsOverflow = await driver.executeScript(() => ({
    document: document.documentElement.scrollWidth > document.documentElement.clientWidth,
    navigation: Array.from(document.querySelectorAll(".workspace-settings-nav > *"))
      .some((element) => element.scrollWidth > element.clientWidth + 1),
  }));
  if (settingsOverflow.document || settingsOverflow.navigation) {
    throw new Error(`general settings has horizontal overflow at 320 CSS pixels: ${JSON.stringify(settingsOverflow)}`);
  }
  await driver.manage().window().setRect({ width: 1280, height: 900 });
  await driver.get(`${baseURL}/account`);
  await driver.wait(until.elementLocated(By.id("delete-confirm")), 5000);
  await audit("account");
  const deletionForm = await driver.executeScript(() => ({
    confirmationRequired: document.querySelector('#delete-confirm[required]') !== null,
    passwordRequired: document.querySelector('#delete-password[required]') !== null,
  }));
  if (!deletionForm.confirmationRequired || !deletionForm.passwordRequired) {
    throw new Error("account deletion confirmation and password are required");
  }
  await expectMobileNoOverflow("account");
  await driver.get(`${baseURL}/settings/language`);
  await driver.wait(until.elementLocated(By.id("language")), 5000);
  await audit("language settings");
  await expectMobileNoOverflow("language settings");
  await driver.get(`${baseURL}/sessions`);
  await driver.wait(until.elementLocated(By.css(".session-card")), 5000);
  await audit("sessions");
  await expectNoExtraButtonMargin(".inline button", "inline actions");
  await expectMobileNoOverflow("sessions");
  await driver.get(`${baseURL}/security`);
  await driver.wait(until.elementLocated(By.css(".settings-panel")), 5000);
  await audit("security");
  await expectMobileNoOverflow("security");
  await driver.get(`${baseURL}/password-reset`);
  try {
    await driver.wait(until.elementLocated(By.css('form[action="/password-reset"]')), 5000);
  } catch (error) {
    console.error(`password reset route ${await driver.getCurrentUrl()} title ${await driver.getTitle()}`);
    throw error;
  }
  await audit("password reset");
  await expectMobileNoOverflow("password reset");
  await driver.get(`${workspaceURL.origin}${workspaceURL.pathname}/items`);
  await driver.wait(until.elementLocated(By.id("item-list")), 5000);
  await audit("items");
	await driver.get(`${workspaceURL.origin}${workspaceURL.pathname}/clients`);
	await driver.wait(until.elementLocated(By.id("client-list")), 5000);
	await audit("clients");
	await driver.findElement(By.css(".client-create > summary")).click();
	await driver.findElement(By.id("client-display-name")).sendKeys(`Accessibility client ${locale}`);
	await driver.findElement(By.css("form.create button[type=submit]")).click();
	await driver.wait(until.elementLocated(By.id("client-detail")), 5000);
	await audit("client");
	await driver.get(`${workspaceURL.origin}${workspaceURL.pathname}/client-fields`);
	await driver.wait(until.elementLocated(By.id("client-field-list")), 5000);
	await audit("client fields");
	await driver.get(`${workspaceURL.origin}${workspaceURL.pathname}/items`);
	await driver.wait(until.elementLocated(By.id("item-list")), 5000);
	await driver.executeScript(() => {
		const input = document.querySelector("#new-title");
		input.required = false;
		input.value = "";
		input.form.requestSubmit();
	});
	await driver.wait(until.elementLocated(By.css('[role="alert"]')), 5000);
	await audit("item-validation");
	if ((await driver.switchTo().activeElement().getAttribute("role")) !== "alert") {
		throw new Error("HTMX validation error did not receive focus");
	}
	const spacingTitle = `Spacing check ${locale}`;
	await driver.findElement(By.id("new-title")).sendKeys(spacingTitle);
	await driver.findElement(By.css("form.create button[type=submit]")).click();
	await driver.wait(until.elementLocated(By.css(`#item-list .item-list-view input[value="${spacingTitle}"]`)), 5000);
	await expectNoExtraButtonMargin(".row button", "item row actions");
	await expectNoExtraButtonMargin(".actions button", "item action group");
	await expectNoExtraButtonMargin(".account-menu-panel button", "account menu");
	await driver.manage().window().setRect({ width: 320, height: 800 });
	if (await driver.executeScript(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)) {
		throw new Error("items page has horizontal overflow at 320 CSS pixels");
	}
	await driver.sendDevToolsCommand("Emulation.setEmulatedMedia", {
		features: [{ name: "prefers-reduced-motion", value: "reduce" }]
	});
	if (!(await driver.executeScript(() => matchMedia("(prefers-reduced-motion: reduce)").matches))) {
		throw new Error("reduced-motion preference was not honored");
	}
  const tree = await driver.sendAndGetDevToolsCommand("Accessibility.getFullAXTree");
  const names = locale === "es"
    ? [["heading", "Elementos"], ["textbox", "Título del nuevo elemento"], ["button", "Añadir elemento"]]
    : [["heading", "Items"], ["textbox", "New item title"], ["button", "Add item"]];
	for (const [role, name] of names) {
		if (!tree.nodes.some((node) => node.role?.value === role && node.name?.value === name)) {
			throw new Error(`accessibility tree lacks ${role} named ${name}`);
		}
	}
	await driver.manage().window().setRect({ width: 1280, height: 900 });
	await driver.get(`${workspaceURL.origin}${workspaceURL.pathname}/settings/import`);
	await driver.wait(until.elementLocated(By.id("import-file")), 5000);
	await audit("import");
	const importContract = await driver.executeScript(() => ({
		form: document.querySelector('form[enctype="multipart/form-data"]') !== null,
		fileRequired: document.querySelector("#import-file[required]") !== null,
		previewAction: document.querySelector('input[name="action"][value="preview"]') !== null,
		confirmationAbsent: document.querySelector("#import-confirm") === null,
		noScript: document.querySelector('script[src="/assets/app.js"]') === null,
		noWebMCP: document.querySelector("[data-webmcp-page]") === null,
	}));
	if (!Object.values(importContract).every(Boolean)) {
		throw new Error("import page ordinary-form contract failed");
	}
	const importTree = await driver.sendAndGetDevToolsCommand("Accessibility.getFullAXTree");
	const importNames = locale === "es"
		? [["heading", "Accessibility Importar"], ["button", "Previsualizar archivo"]]
		: [["heading", "Accessibility Import"], ["button", "Preview file"]];
	for (const [role, name] of importNames) {
		if (!importTree.nodes.some((node) => node.role?.value === role && node.name?.value === name)) {
			throw new Error(`import accessibility tree lacks ${role} named ${name}`);
		}
	}
	await driver.findElement(By.id("import-file")).sendKeys(importFixturePath);
	await driver.findElement(By.css('form.import-preview-form button[type="submit"]')).click();
	await driver.wait(until.elementLocated(By.id("import-preview-heading")), 5000);
	await audit("import preview");
	const previewTree = await driver.sendAndGetDevToolsCommand("Accessibility.getFullAXTree");
	const previewNames = locale === "es"
		? [["heading", "Vista previa de importación"], ["checkbox", "Revisé la vista previa y entiendo que todo el archivo se importará como elementos nuevos en este espacio de trabajo."], ["button", "Importar elementos previsualizados"]]
		: [["heading", "Import preview"], ["checkbox", "I reviewed the preview and understand this imports the whole file as new items in this workspace."], ["button", "Import previewed items"]];
	for (const [role, name] of previewNames) {
		if (!previewTree.nodes.some((node) => node.role?.value === role && node.name?.value === name)) {
			throw new Error(`import preview accessibility tree lacks ${role} named ${name}`);
		}
	}
	await driver.manage().window().setRect({ width: 320, height: 800 });
	const overflow = await driver.executeScript(() => {
		const root = document.documentElement;
		const viewport = root.clientWidth;
		const round = (value) => Math.round(value * 10) / 10;
		const offenders = Array.from(document.body.querySelectorAll("*"), (element) => {
			const rect = element.getBoundingClientRect();
			const style = getComputedStyle(element);
			return {
				tag: element.tagName.toLowerCase(),
				id: element.id || undefined,
				className: element.getAttribute("class") || undefined,
				href: element instanceof HTMLAnchorElement && element.hasAttribute("href")
					? new URL(element.href).pathname
					: undefined,
				left: round(rect.left),
				right: round(rect.right),
				width: round(rect.width),
				clientWidth: element.clientWidth,
				scrollWidth: element.scrollWidth,
				minWidth: style.minWidth,
				overflowX: style.overflowX,
			};
		}).filter((element) =>
			element.width > 0 &&
			(element.left < -1 || element.right > viewport + 1 || element.scrollWidth > element.clientWidth + 1)
		).sort((left, right) => right.right - left.right).slice(0, 12);
		return {
			userAgent: navigator.userAgent,
			innerWidth: window.innerWidth,
			outerWidth: window.outerWidth,
			clientWidth: root.clientWidth,
			scrollWidth: root.scrollWidth,
			bodyScrollWidth: document.body.scrollWidth,
			offenders,
		};
	});
	if (overflow.scrollWidth > overflow.clientWidth) {
		throw new Error(`import page has horizontal overflow at 320 CSS pixels: ${JSON.stringify(overflow)}`);
	}
	await driver.manage().window().setRect({ width: 1280, height: 900 });
	await driver.get(`${workspaceURL.origin}${workspaceURL.pathname}/files`);
	await driver.wait(until.elementLocated(By.id("file-upload")), 5000);
	await audit("files");
	const fileUploadContract = await driver.executeScript(() => {
		const form = document.querySelector('form[data-file-upload][method="post"][enctype="multipart/form-data"]');
		const progress = form?.querySelector("progress[data-upload-progress]");
		return Boolean(form && progress && progress.max === 100 && progress.getAttribute("aria-label"));
	});
	if (!fileUploadContract) throw new Error("files page ordinary upload and named progress contract failed");
	await driver.executeScript(() => {
		const progress = document.querySelector("progress[data-upload-progress]");
		progress.value = 1;
		progress.removeAttribute("hidden");
	});
	const fileTree = await driver.sendAndGetDevToolsCommand("Accessibility.getFullAXTree");
	const progressName = locale === "es" ? "Progreso de carga" : "Upload progress";
	if (!fileTree.nodes.some((node) => node.role?.value === "progressbar" && node.name?.value === progressName)) {
		throw new Error(`files accessibility tree lacks progressbar named ${progressName}`);
	}
	await driver.manage().window().setRect({ width: 320, height: 800 });
	if (await driver.executeScript(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)) {
		throw new Error("files page has horizontal overflow at 320 CSS pixels");
	}
	console.log("manual-contract: keyboard, focus, 320px reflow, reduced motion, and accessibility tree passed");
} finally {
  await driver.quit();
	rmSync(importFixtureDir, { recursive: true, force: true });
}

process.exit(failed ? 1 : 0);

async function audit(name) {
  await driver.executeScript(axe.source);
  const results = await driver.executeAsyncScript((done) => {
    axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"] } })
      .then(done)
      .catch((error) => done({ error: error.message }));
  });
  if (results.error) throw new Error(results.error);
  const blocking = results.violations.filter((violation) =>
    violation.impact === "critical" || violation.impact === "serious"
  );
  console.log(`${name}: ${results.violations.length} violations, ${blocking.length} critical/serious`);
  for (const violation of blocking) {
    console.error(`${name}: ${violation.impact} ${violation.id}: ${violation.help}`);
    console.error(`${name}: targets ${JSON.stringify(violation.nodes.map((node) => node.target))}`);
    console.error(violation.nodes.map((node) => `${node.target.join(" ")}: ${node.failureSummary}`).join("\n"));
  }
  failed ||= blocking.length > 0;
}

async function expectActive(id) {
	const active = await driver.switchTo().activeElement();
	if ((await active.getAttribute("id")) !== id) {
		throw new Error(`keyboard focus expected ${id}`);
	}
	const style = await driver.executeScript((element) => getComputedStyle(element).boxShadow, active);
	if (!style || style === "none") {
		throw new Error(`visible focus style missing on ${id}`);
	}
}

async function expectFormActionSpacing(controlSelector, buttonSelector, name) {
	const metrics = await driver.executeScript((controlQuery, buttonQuery) => {
		const control = document.querySelector(controlQuery);
		const button = document.querySelector(buttonQuery);
		if (!control || !button) return null;
		return {
			gap: button.getBoundingClientRect().top - control.getBoundingClientRect().bottom,
			marginTop: getComputedStyle(button).marginTop,
			height: button.getBoundingClientRect().height,
		};
	}, controlSelector, buttonSelector);
	if (!metrics) throw new Error(`${name} spacing controls missing`);
	if (metrics.marginTop !== "12px" || metrics.gap < 12 || metrics.height < 44) {
		throw new Error(`${name} button spacing invalid: ${JSON.stringify(metrics)}`);
	}
}

async function expectNoExtraButtonMargin(selector, name) {
	const marginTop = await driver.executeScript((query) => {
		const button = document.querySelector(query);
		return button ? getComputedStyle(button).marginTop : null;
	}, selector);
	if (marginTop !== "0px") throw new Error(`${name} button has unexpected top margin: ${marginTop}`);
}

async function expectMobileNoOverflow(name) {
	await driver.manage().window().setRect({ width: 320, height: 800 });
	if (await driver.executeScript(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)) {
		throw new Error(`${name} page has horizontal overflow at 320 CSS pixels`);
	}
	await driver.manage().window().setRect({ width: 1280, height: 900 });
}
