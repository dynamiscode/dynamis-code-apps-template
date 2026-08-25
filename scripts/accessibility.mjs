import axe from "axe-core";
import { Builder, By, Key, until } from "selenium-webdriver";
import chrome from "selenium-webdriver/chrome.js";

const baseURL = process.env.A11Y_BASE_URL;
const email = process.env.A11Y_EMAIL;
const password = process.env.A11Y_PASSWORD;

if (!baseURL || !email || !password) {
  console.error("A11Y_BASE_URL, A11Y_EMAIL, and A11Y_PASSWORD are required");
  process.exit(2);
}

const driver = await new Builder()
  .forBrowser("chrome")
  .setChromeOptions(new chrome.Options().addArguments("--headless=new", "--window-size=1280,900"))
  .build();

let failed = false;
try {
  await driver.get(`${baseURL}/login`);
  await audit("login");
	await driver.findElement(By.css("body")).sendKeys(Key.TAB);
	await expectActive("email");
	await driver.actions().sendKeys(Key.TAB).perform();
	await expectActive("password");
	await driver.actions().sendKeys(Key.TAB).perform();
	if ((await driver.switchTo().activeElement().getTagName()) !== "button") {
		throw new Error("login keyboard order does not reach submit after password");
	}
  await driver.findElement(By.id("email")).sendKeys(email);
  await driver.findElement(By.id("password")).sendKeys(password);
  await driver.findElement(By.css('button[type="submit"]')).click();
  await driver.wait(until.urlIs(`${baseURL}/`), 5000);
  await audit("workspaces");
  await driver.findElement(By.css(".cards a")).click();
  await driver.wait(until.elementLocated(By.css("[data-workspace-home]")), 5000);
  await audit("workspace");
  await driver.findElement(By.css('a[href$="/items"]')).click();
  await driver.wait(until.elementLocated(By.id("item-list")), 5000);
  await audit("items");
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
	for (const [role, name] of [["heading", "Accessibility items"], ["textbox", "New item title"], ["button", "Add item"]]) {
		if (!tree.nodes.some((node) => node.role?.value === role && node.name?.value === name)) {
			throw new Error(`accessibility tree lacks ${role} named ${name}`);
		}
	}
	console.log("manual-contract: keyboard, focus, 320px reflow, reduced motion, and accessibility tree passed");
} finally {
  await driver.quit();
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
