import { Builder, By } from "selenium-webdriver";
import chrome from "selenium-webdriver/chrome.js";

const baseURL = process.env.WEBMCP_BASE_URL;
const email = process.env.WEBMCP_EMAIL;
const password = process.env.WEBMCP_PASSWORD;
const browsers = (process.env.WEBMCP_BROWSERS || "chrome").split(",").map((value) => value.trim()).filter(Boolean);

if (!baseURL || !email || !password) {
  console.error("WEBMCP_BASE_URL, WEBMCP_EMAIL, and WEBMCP_PASSWORD are required");
  process.exit(2);
}

for (const browser of browsers) {
  const options = browser === "chrome" ? new chrome.Options().addArguments("--headless=new", "--window-size=1280,900") : undefined;
  const builder = new Builder().forBrowser(browser);
  if (options) builder.setChromeOptions(options);
  const driver = await builder.build();
  try {
    await driver.get(`${baseURL}/login`);
    await driver.findElement(By.id("email")).sendKeys(email);
    await driver.findElement(By.id("password")).sendKeys(password);
    await driver.findElement(By.css('button[type="submit"]')).click();
    await driver.wait(async () => (await driver.getCurrentUrl()) === `${baseURL}/`, 5000);
    await driver.findElement(By.css(".cards a")).click();
    await driver.wait(async () => Boolean(await driver.findElements(By.css("[data-workspace-home]"))), 5000);
    await driver.findElement(By.css('a[href$="/items"]')).click();
    await driver.wait(async () => Boolean(await driver.findElements(By.id("item-list"))), 5000);

    const fallback = await driver.executeScript(() => ({
      form: Boolean(document.querySelector('form[data-webmcp-actions="item-create"]')),
      marker: Boolean(document.querySelector('[data-webmcp-page="items"]')),
      script: [...document.scripts].some((script) => script.src.endsWith("/assets/app.js")),
    }));
    if (!fallback.form || !fallback.marker || !fallback.script) throw new Error(`${browser}: ordinary browser fallback is incomplete`);

    const native = await driver.executeAsyncScript((done) => {
      const context = document.modelContext;
      if (!context) return done({ supported: false });
      if (typeof context.getTools !== "function") return done({ supported: true, error: "getTools unavailable" });
      Promise.resolve(context.getTools()).then((tools) => {
        const summaries = tools.map(({ name, inputSchema, description, annotations }) => ({ name, inputSchema, description, annotations }));
        done({
          supported: true,
          names: summaries.map((tool) => tool.name),
          schemas: summaries,
          redacted: !JSON.stringify(summaries).match(/csrf|password|invitationsecret|tokensecret|oidc/i),
        });
      }).catch((error) => done({ supported: true, error: error.message }));
    });
    if (!native.supported) {
      console.log(`${browser}: fallback passed; WebMCP unavailable`);
      continue;
    }
    if (native.error) throw new Error(`${browser}: native WebMCP check failed: ${native.error}`);
    for (const name of ["item-create-v1", "workspace-export-v1"]) {
      if (!native.names.includes(name)) throw new Error(`${browser}: missing native tool ${name}`);
    }
    if (!native.redacted) throw new Error(`${browser}: native tools expose a forbidden secret field`);
    const createTool = native.schemas.find((tool) => tool.name === "item-create-v1");
    if (JSON.stringify(createTool.inputSchema) !== JSON.stringify({ type: "object", properties: { title: { type: "string", minLength: 1, maxLength: 200 } }, required: ["title"], additionalProperties: false })) {
      throw new Error(`${browser}: native item-create schema changed`);
    }
    const prepared = await driver.executeAsyncScript((done) => {
      const context = document.modelContext;
      const form = document.querySelector('form[data-webmcp-actions="item-create"]');
      let submitted = false;
      form.addEventListener("submit", () => { submitted = true; }, { once: true });
      Promise.resolve(context.getTools()).then((tools) => tools.find((tool) => tool.name === "item-create-v1")).then((tool) =>
        context.executeTool(tool, JSON.stringify({ title: "WebMCP smoke" }))).then((result) => done({
          result, value: document.querySelector("#new-title")?.value, focused: document.activeElement?.id, submitted,
        })).catch((error) => done({ error: error.message }));
    });
    if (prepared.error || prepared.submitted || prepared.value !== "WebMCP smoke" || prepared.focused !== "new-title") {
      throw new Error(`${browser}: native preparation failed`);
    }
    const workspaceRoot = (await driver.getCurrentUrl()).replace(/\/items$/, "");
    await driver.get(`${workspaceRoot}/settings/export`);
    await driver.wait(async () => Boolean(await driver.findElements(By.css('a[data-webmcp-export]'))), 5000);
    const exportNative = await driver.executeAsyncScript((done) => {
      const context = document.modelContext;
      const link = document.querySelector("[data-webmcp-export]");
      let activated = false;
      link.addEventListener("click", () => { activated = true; }, { once: true });
      Promise.resolve(context.getTools()).then((tools) => {
        const tool = tools.find((candidate) => candidate.name === "workspace-export-v1");
        if (!tool) return done({ error: "workspace export tool unavailable" });
        if (JSON.stringify(tool.inputSchema) !== JSON.stringify({ type: "object", properties: {}, additionalProperties: false })) {
          return done({ error: "workspace export schema changed" });
        }
        return context.executeTool(tool, JSON.stringify({})).then((result) => done({
          result,
          focused: document.activeElement === link,
          activated,
          redacted: !JSON.stringify(result).match(/csrf|password|invitationsecret|tokensecret|oidc/i),
        }));
      }).catch((error) => done({ error: error.message }));
    });
    if (exportNative.error || !exportNative.focused || exportNative.activated || !exportNative.redacted) {
      throw new Error(`${browser}: native export preparation failed`);
    }
    console.log(`${browser}: fallback and native WebMCP checks passed`);
  } finally {
    await driver.quit();
  }
}
