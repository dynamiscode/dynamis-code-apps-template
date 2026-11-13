// Real-browser checks of the form UX patterns that forms.js wires into the
// pages. Run through scripts/ux-smoke.sh, which starts a fresh server.
// Each check names the skill whose rule it exercises.
import { Builder, By, Key, until } from "selenium-webdriver";
import chrome from "selenium-webdriver/chrome.js";

const base = process.env.UX_BASE_URL;
if (!base) {
  console.error("UX_BASE_URL is required");
  process.exit(2);
}
const email = "ux@example.com";
const password = "ux-password-1234!";

const driver = await new Builder()
  .forBrowser("chrome")
  .setChromeOptions(new chrome.Options().addArguments("--headless=new", "--window-size=1280,900"))
  .build();

const js = (fn, ...args) => driver.executeScript(fn, ...args);
const find = (css) => driver.findElement(By.css(css));
const waitFor = (fn, label, ms = 5000) => driver.wait(async () => Boolean(await js(fn)), ms, label);
const passed = [];
function check(condition, message) {
  if (!condition) throw new Error(message);
  passed.push(message);
}
const typeInto = async (css, text) => {
  const el = await find(css);
  await el.clear();
  await el.sendKeys(text);
};

try {
  // form-wizard: setup is two steps; Next is blocked while step one is invalid.
  await driver.get(`${base}/setup`);
  check(await js(() => document.querySelectorAll("fieldset[data-step]")[1].hidden), "form-wizard: step two starts hidden");
  check(await js(() => document.querySelector('form[data-wizard] button[type="submit"]').hidden), "form-wizard: submit hidden before the last step");
  await find(".wizard-nav button:not(.secondary)").click();
  check(await js(() => document.querySelectorAll("fieldset[data-step]")[1].hidden), "form-wizard: Next is blocked while step one is invalid");
  check(await js(() => document.querySelector("#email").getAttribute("aria-invalid") === "true"), "form-field-input: the invalid field is marked");
  await typeInto("#email", `  ${email}  `);
  await typeInto("#password", "short");
  check(await js(() => document.querySelector(".strength meter").value === 0), "form-field-input: a short password scores 0");
  await typeInto("#password", password);
  check(await js(() => document.querySelector(".strength meter").value >= 1), "form-field-input: a valid password scores above 0");
  await find("#password + .reveal").click();
  check(await js(() => document.querySelector("#password").type === "text"), "form-field-input: reveal shows the password");
  await find("#password + .reveal").click();
  await typeInto("#password-confirmation", "different-password-1");
  await find(".wizard-nav button:not(.secondary)").click();
  check(await js(() => document.querySelector("#password-confirmation").getAttribute("aria-invalid") === "true"), "form-field-input: mismatched confirmation is invalid");
  await typeInto("#password-confirmation", password);
  await find(".wizard-nav button:not(.secondary)").click();
  check(await js(() => !document.querySelectorAll("fieldset[data-step]")[1].hidden), "form-wizard: Next moves to step two once step one is valid");
  check(await js(() => document.activeElement?.tagName === "LEGEND"), "form-wizard: focus moves to the new step");
  check(await js(() => document.querySelector("#email").value === "ux@example.com"), "form-field-input: the email was trimmed");
  await typeInto("#workspace", "UX");
  await find('form[data-wizard] button[type="submit"]').click();
  await driver.wait(async () => !(await driver.getCurrentUrl()).endsWith("/setup"), 5000, "setup finished");
  check(true, "form-wizard: the owner account is created from the last step");

  // form-field-input: a full-page error takes focus.
  await driver.manage().deleteAllCookies();
  await driver.get(`${base}/login`);
  await typeInto("#email", email);
  await typeInto("#password", "wrong-password-1234");
  await find('form[action="/login"] button[type="submit"]').click();
  await waitFor(() => document.activeElement?.matches('.error[role="alert"]'), "login error focus");
  check(true, "form-field-input: a full-page error takes focus");
  await typeInto("#password", password);
  await find('form[action="/login"] button[type="submit"]').click();
  await driver.wait(until.urlIs(`${base}/`), 5000);
  const workspace = new URL(await find(".cards a").getAttribute("href"), base).pathname;

  // async-feedback: the Add button is busy while sending, a double press makes one item, a toast confirms.
  await driver.get(`${base}${workspace}/items`);
  await js(() => {
    window.__busy = false;
    new MutationObserver(() => {
      if (document.querySelector('form.create[aria-busy="true"] button:disabled')) window.__busy = true;
    }).observe(document.body, { subtree: true, attributes: true, childList: true });
  });
  await typeInto("#new-title", "First item");
  await js(() => {
    const button = document.querySelector("form.create button[type=submit]");
    button.click();
    button.click();
  });
  await waitFor(() => document.querySelector(".toasts .toast") && document.querySelector("#item-list li"), "add toast and new row");
  await driver.sleep(300); // let a duplicate request, if any, land before counting
  check(await js(() => window.__busy), "async-feedback: the Add button is disabled while sending");
  const firstCount = await js(() => [...document.querySelectorAll("#item-list li input[name=title]")].filter((el) => el.value === "First item").length);
  check(firstCount === 1, `item-create-submission: a double press makes one item (found ${firstCount})`);
  check(await js(() => document.querySelector(".toasts .toast").textContent.includes("Item added.")), "async-feedback: a toast confirms the add");
  check(await js(() => document.querySelector("#new-title").value === ""), "item-create-submission: the box is empty after a save");

  // form-field-input: a blank title is stopped in the browser with a summary.
  await typeInto("#new-title", "   ");
  await find("form.create button[type=submit]").click();
  check(await js(() => document.activeElement?.classList.contains("error-summary")), "form-field-input: the error summary takes focus");
  check(await js(() => document.querySelectorAll("#item-list li input[name=title]").length === 1), "form-field-input: an invalid add sends nothing");

  // form-drafts: Save is enabled only after a change.
  const row = "#item-list li form[data-dirty-track]";
  check(await js((sel) => document.querySelector(`${sel} button[value=update]`).disabled, row), "form-drafts: Save starts disabled");
  await find(`${row} input[name=title]`).sendKeys(" edited");
  check(await js((sel) => !document.querySelector(`${sel} button[value=update]`).disabled, row), "form-drafts: Save enables after a change");

  // async-feedback: autosave on status change.
  await js(() => document.querySelectorAll(".toasts .toast").forEach((t) => t.remove()));
  await js((sel) => {
    const select = document.querySelector(`${sel} select[name=status]`);
    select.value = "complete";
    select.dispatchEvent(new Event("change", { bubbles: true }));
  }, row);
  await waitFor(() => document.querySelector(".toasts .toast"), "autosave toast");
  check(await js(() => document.querySelector("#item-list select[name=status]").value === "complete"), "async-feedback: autosave keeps the new status");

  // async-feedback: optimistic delete hides the row at once.
  await js(() => { window.confirm = () => true; });
  const hiddenAtOnce = await js(() => {
    const button = document.querySelector("#item-list button[value=delete]");
    const li = button.closest("li");
    button.click();
    return li.dataset.optimistic === "hidden";
  });
  check(hiddenAtOnce, "async-feedback: delete hides the row before the answer");
  await waitFor(() => document.querySelectorAll("#item-list li input[name=title]").length === 0, "row removed");
  check(true, "async-feedback: the row is gone after the answer");

  // form-drafts: an unsent invitation survives leaving the page.
  await driver.get(`${base}${workspace}/settings/invitations`);
  await typeInto("#invite-email", "draft@example.com");
  await driver.get(`${base}${workspace}`);
  await driver.get(`${base}${workspace}/settings/invitations`);
  check(await js(() => document.querySelector("#invite-email").value === "draft@example.com"), "form-drafts: the draft is restored");
  check(await js(() => Boolean(document.querySelector(".draft-notice"))), "form-drafts: the restore is announced");
  await find(".draft-notice button").click();
  check(await js(() => document.querySelector("#invite-email").value === ""), "form-drafts: Discard clears the draft");

  // form-drafts: the unsaved-changes warning fires only when dirty.
  await driver.get(`${base}/account`);
  const prompt = () => js(() => {
    const event = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(event);
    return event.defaultPrevented;
  });
  check(!(await prompt()), "form-drafts: no warning on a clean form");
  await find("#display-name").sendKeys("Ux");
  check(await prompt(), "form-drafts: a warning on unsaved changes");
  check(await js(() => document.querySelector("#account-timezone").value !== ""), "form-field-input: timezone has a smart default");

  // form-wizard: progressive disclosure on email settings.
  await driver.get(`${base}${workspace}/settings/email`);
  const shown = () => js(() => [...document.querySelectorAll("[data-show-when]")].filter((s) => !s.hidden).map((s) => s.dataset.showWhen));
  check(JSON.stringify(await shown()) === JSON.stringify(["email-provider=smtp"]), "form-wizard: only the SMTP section shows for SMTP");
  await js(() => {
    const select = document.querySelector("#email-provider");
    select.value = "resend";
    select.dispatchEvent(new Event("change", { bubbles: true }));
  });
  check(JSON.stringify(await shown()) === JSON.stringify(["email-provider=resend"]), "form-wizard: switching provider swaps the section");

  // async-feedback: the audit table loads after the page.
  await driver.get(`${base}${workspace}/settings/audit`);
  await waitFor(() => !document.querySelector("[data-lazy-src]") && document.querySelector("#audit-table"), "lazy audit table");
  check(true, "async-feedback: the audit table loads lazily");

  // form-field-input: smart default token expiry; keyboard: Escape closes a menu.
  await driver.get(`${base}${workspace}/settings/tokens`);
  check(await js(() => /^\d{4}-\d{2}-\d{2}$/.test(document.querySelector("#token-expiry").value)), "form-field-input: token expiry has a default");
  await find(".account-menu summary").click();
  await driver.actions().sendKeys(Key.ESCAPE).perform();
  check(await js(() => !document.querySelector(".account-menu").open), "async-feedback: Escape closes the account menu");

  for (const line of passed) console.log(`ok - ${line}`);
  console.log(`${passed.length} UX checks passed`);
} catch (error) {
  for (const line of passed) console.log(`ok - ${line}`);
  console.error(`not ok - ${error.message}`);
  console.error(`at ${await driver.getCurrentUrl()}: ${(await js(() => document.body.innerText)).slice(0, 600)}`);
  process.exitCode = 1;
} finally {
  await driver.quit();
}
