import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const mfaJsCode = fs.readFileSync(path.join(__dirname, "mfa.js"), "utf8");

const element = (props = {}) => ({
  disabled: false,
  hidden: false,
  textContent: "",
  className: "",
  value: "",
  action: "",
  dataset: {},
  parentElement: null,
  listeners: {},
  addEventListener(type, fn) { this.listeners[type] = fn; },
  setAttribute() {},
  focus() {},
  prepend() {},
  querySelector() { return null; },
  querySelectorAll() { return []; },
  ...props
});

const jsonResponse = (ok, body) => ({
  ok,
  headers: { get: () => null },
  json: async () => {
    if (body === undefined) throw new SyntaxError("not json");
    return body;
  }
});

const deferred = () => {
  let resolve, reject;
  const promise = new Promise((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
};

const cancelError = () => Object.assign(new Error("user dismissed"), { name: "NotAllowedError" });

// Loads mfa.js against minimal DOM stubs. Login and enrollment are both present unless disabled.
// The injected atob is lenient because mfa.js's decode() appends "===" to base64url values, which
// the browser atob rejects (InvalidCharacterError); see the follow-up note in the change report.
function load({ login = false, enroll = false, credentialsGet, credentialsCreate, fetchImpl } = {}) {
  const main = element();
  const loginButton = element({ dataset: { csrf: "test-csrf" } });
  const loginSubmit = element();
  const optionsNode = element({
    textContent: JSON.stringify({ publicKey: { challenge: "AAEC", allowCredentials: [] } })
  });
  const enrollButtons = [element(), element()];
  const enrollParent = element();
  const csrfInput = { value: "test-csrf" };
  const passwordInput = { value: "not-a-real-password" };
  const enrollForm = element({
    action: "/api/v1/auth/mfa/passkeys/options",
    parentElement: enrollParent,
    hidden: false,
    querySelectorAll: () => enrollButtons,
    querySelector: sel => (sel === "[name=csrf]" ? csrfInput : sel === "[name=password]" ? passwordInput : null)
  });
  const output = element({ hidden: true });
  const values = element();
  const elements = {
    "passkey-login": login ? loginButton : null,
    "mfa-options": login ? optionsNode : null,
    "passkey-enroll": enroll ? enrollForm : null,
    "mfa-recovery-output": output,
    "mfa-recovery-values": values,
    main
  };
  const notice = { current: null };
  const document = {
    getElementById: id => elements[id] ?? null,
    querySelector: sel => (sel === "p.error[role=alert]" ? notice.current : null),
    querySelectorAll: sel => (sel === "button, input[type=submit]" && login ? [loginButton, loginSubmit] : []),
    createElement: () => { notice.current = element(); return notice.current; },
    body: element()
  };
  const window = {
    PublicKeyCredential: function PublicKeyCredential() {},
    location: { reloads: 0, reload() { this.reloads++; } }
  };
  const navigator = { credentials: { get: credentialsGet, create: credentialsCreate } };
  const fetch = fetchImpl || (async () => { throw new TypeError("no fetch stub"); });
  const atob = value => Buffer.from(value, "base64").toString("binary");
  new Function("window", "document", "navigator", "fetch", "atob", mfaJsCode)(window, document, navigator, fetch, atob);
  return { window, loginButton, loginSubmit, enrollForm, enrollButtons, enrollParent, notice, output, values };
}

test("login: buttons stay busy during the prompt and re-enable with a notice on cancel", async () => {
  const prompt = deferred();
  const env = load({ login: true, credentialsGet: () => prompt.promise });
  const click = env.loginButton.listeners.click();
  assert.equal(env.loginButton.disabled, true);
  assert.equal(env.loginSubmit.disabled, true);
  prompt.reject(cancelError());
  await click;
  assert.equal(env.loginButton.disabled, false);
  assert.equal(env.loginSubmit.disabled, false);
  assert.equal(env.notice.current.textContent, "Passkey verification was canceled or timed out.");
  assert.equal(env.window.location.reloads, 0);
});

test("login: other errors reload the page", async () => {
  const env = load({
    login: true,
    credentialsGet: async () => { throw new TypeError("unexpected"); }
  });
  await env.loginButton.listeners.click();
  assert.equal(env.window.location.reloads, 1);
  assert.equal(env.notice.current, null);
});

test("enrollment: server error detail is shown and buttons re-enable without reload", async () => {
  let created = false;
  const env = load({
    enroll: true,
    credentialsCreate: async () => { created = true; },
    fetchImpl: async () => jsonResponse(false, { title: "Unauthorized", code: "mfa-invalid", detail: "The multi-factor authentication request is invalid." })
  });
  await env.enrollForm.listeners.submit({ preventDefault() {} });
  assert.equal(env.notice.current.textContent, "The multi-factor authentication request is invalid.");
  assert.equal(env.enrollButtons.every(b => b.disabled === false), true);
  assert.equal(env.window.location.reloads, 0);
  assert.equal(created, false);
});

test("enrollment: non-OK without a usable safe message shows the generic text, never raw body", async () => {
  const env = load({
    enroll: true,
    fetchImpl: async () => jsonResponse(false, { detail: { internal: "stack trace" } })
  });
  await env.enrollForm.listeners.submit({ preventDefault() {} });
  assert.equal(env.notice.current.textContent, "The passkey registration failed.");
  assert.equal(env.window.location.reloads, 0);
});

test("enrollment: cancel shows the registration notice in the form's container", async () => {
  const env = load({
    enroll: true,
    credentialsCreate: async () => { throw cancelError(); },
    fetchImpl: async () => jsonResponse(true, { challenge: "tok", options: { publicKey: { challenge: "AAEC", user: { id: "AQI" } } } })
  });
  await env.enrollForm.listeners.submit({ preventDefault() {} });
  assert.equal(env.notice.current.textContent, "Passkey registration was canceled or timed out.");
  assert.equal(env.enrollButtons.every(b => b.disabled === false), true);
  assert.equal(env.window.location.reloads, 0);
});

test("enrollment: other errors reload the page", async () => {
  const env = load({
    enroll: true,
    credentialsCreate: async () => { throw new TypeError("unexpected"); },
    fetchImpl: async () => jsonResponse(true, { challenge: "tok", options: { publicKey: { challenge: "AAEC", user: { id: "AQI" } } } })
  });
  await env.enrollForm.listeners.submit({ preventDefault() {} });
  assert.equal(env.window.location.reloads, 1);
});

test("enrollment: success with recovery codes keeps buttons disabled and shows the codes", async () => {
  const env = load({
    enroll: true,
    credentialsCreate: async () => ({
      id: "cred",
      rawId: new Uint8Array([1]).buffer,
      type: "public-key",
      response: { clientDataJSON: new Uint8Array([2]).buffer, attestationObject: new Uint8Array([3]).buffer, getTransports: () => [] }
    }),
    fetchImpl: async url => (url === "/api/v1/auth/mfa/passkeys/register"
      ? jsonResponse(true, { recoveryCodes: ["AAAA-BBBB", "CCCC-DDDD"] })
      : jsonResponse(true, { challenge: "tok", options: { publicKey: { challenge: "AAEC", user: { id: "AQI" } } } }))
  });
  await env.enrollForm.listeners.submit({ preventDefault() {} });
  assert.equal(env.enrollButtons.every(b => b.disabled === true), true);
  assert.equal(env.output.hidden, false);
  assert.equal(env.values.textContent, "AAAA-BBBB\nCCCC-DDDD");
  assert.equal(env.enrollForm.hidden, true);
  assert.equal(env.window.location.reloads, 0);
});
