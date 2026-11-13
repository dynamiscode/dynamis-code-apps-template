// Exhaustive check of the file upload form (FileUploader in app.js). It runs
// the real class through every outcome of its upload request, with and without
// an extra press while the request is in flight, and checks the rules in the
// file-upload-form skill after every step. See that skill before
// changing this file.
import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const code = fs.readFileSync(path.join(path.dirname(fileURLToPath(import.meta.url)), "app.js"), "utf8");

function load(fetch) {
  const window = { location: { assigned: [], assign(url) { this.assigned.push(url); } } };
  const document = { querySelector: () => null, addEventListener: () => {} };
  return new Function("window", "document", "fetch", `${code}; return window;`)(window, document, fetch);
}

function form() {
  const status = {
    textContent: "", classes: new Set(["status"]), attributes: { hidden: "" },
    classList: { add(c) { status.classes.add(c); }, remove(c) { status.classes.delete(c); } },
    setAttribute(k, v) { this.attributes[k] = v; }, removeAttribute(k) { delete this.attributes[k]; },
  };
  const button = { disabled: false };
  const file = { files: [new File(["abc"], "a.pdf", { type: "application/pdf" })], disabled: false };
  const csrf = { value: "csrf" };
  const el = {
    dataset: {}, action: "/w/files",
    querySelector(sel) {
      if (sel === "button" || sel === 'button[type="submit"]') return button;
      if (sel === 'input[type="file"]') return file;
      if (sel === 'input[name="csrf"]') return csrf;
      if (sel === "[data-upload-status]") return status;
      return null;
    },
    appendChild: (x) => x,
  };
  return { el, status, button, file };
}

// Outcomes of the one multipart POST.
const UPLOAD = ["ok", "http", "throw"];
const paths = () => UPLOAD.map((outcome) => [outcome]);

function respond(stage, outcome) {
  if (outcome === "throw") return Promise.reject(new TypeError("network"));
  if (outcome === "http") return Promise.resolve({ ok: false, text: async () => "" });
  return Promise.resolve({ ok: true });
}

const IN_FLIGHT = new Set(["UPLOADING", "SUCCESS"]);

function checkRules(u, f, label) {
  // Controls are disabled exactly while an upload is in flight or done.
  assert.equal(f.button.disabled, IN_FLIGHT.has(u.phase), `${label}: button disabled in ${u.phase}`);
  assert.equal(f.file.disabled, IN_FLIGHT.has(u.phase), `${label}: file input disabled in ${u.phase}`);
  // Every phase but idle tells the user what is happening.
  if (u.phase !== "IDLE") {
    assert.ok(f.status.textContent, `${label}: empty status in ${u.phase}`);
    assert.ok(!("hidden" in f.status.attributes), `${label}: status hidden in ${u.phase}`);
  }
  // Only the error phase looks like an error.
  assert.equal(f.status.classes.has("error"), u.phase === "ERROR", `${label}: error style in ${u.phase}`);
}

const flush = () => new Promise((r) => setImmediate(r));

for (const outcomes of paths()) {
  for (let presses = 0; presses < 1 << outcomes.length; presses++) {
    const label = `${outcomes.join(">")} extra-press-mask=${presses}`;
    test(`upload form: ${label}`, { timeout: 5000 }, async () => {
      // Each request waits until the test releases it, so presses land while it is in flight.
      const calls = [];
      const release = [];
      const window = load((url) => {
        calls.push(url);
        const stage = calls.length - 1;
        return new Promise((resolve) => release.push(resolve)).then(() => respond(stage, outcomes[stage]));
      });
      const f = form();
      const u = new window.FileUploader(f.el);
      const run = u.handleSubmit({ preventDefault() {} });
      for (let stage = 0; stage < outcomes.length; stage++) {
        for (let i = 0; i < 20 && calls.length <= stage; i++) await flush();
        assert.equal(calls.length, stage + 1, `${label}: request ${stage} never started`);
        assert.ok(IN_FLIGHT.has(u.phase), `${label}: phase ${u.phase} while request ${stage} is in flight`);
        checkRules(u, f, label);
        // An extra press while a request is in flight starts nothing.
        if (presses & (1 << stage)) {
          u.handleSubmit({ preventDefault() {} });
          assert.equal(calls.length, stage + 1, `${label}: extra press during request ${stage} sent a request`);
        }
        release[stage]();
        await flush();
        checkRules(u, f, label);
      }
      await run;
      checkRules(u, f, label);
      const success = outcomes[0] === "ok";
      assert.equal(u.phase, success ? "SUCCESS" : "ERROR", label);
      // Success leaves the page once; a failure never does.
      assert.deepEqual(window.location.assigned, success ? ["/w/files"] : [], label);
      // After a failure, pressing again starts a fresh upload.
      if (!success) {
        const before = calls.length;
        u.handleSubmit({ preventDefault() {} });
        assert.equal(calls.length, before + 1, `${label}: retry did not start a new upload`);
        assert.equal(u.phase, "UPLOADING", label);
        checkRules(u, f, label);
      }
    });
  }
}

test("upload form: pressing with no file shows an error and sends nothing", async () => {
  const calls = [];
  const window = load((url) => { calls.push(url); return respond(0, "ok"); });
  const f = form();
  f.file.files = [];
  const u = new window.FileUploader(f.el);
  await u.handleSubmit({ preventDefault() {} });
  assert.equal(calls.length, 0);
  assert.equal(u.phase, "ERROR");
  checkRules(u, f, "no file");
});
