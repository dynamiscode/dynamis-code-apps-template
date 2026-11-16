// Exhaustive check of the file upload form (FileUploader in app.js). It runs
// the real class through every outcome of its one multipart request, with and
// without an extra press while the request is in flight, and checks the rules
// in the file-upload-form skill after every step. See that skill before
// changing this file.
import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const code = fs.readFileSync(path.join(path.dirname(fileURLToPath(import.meta.url)), "app.js"), "utf8");

function load(onSend) {
  const window = { location: { assigned: [], assign(url) { this.assigned.push(url); } } };
  const document = { querySelector: () => null, addEventListener: () => {}, createElement: () => ({ setAttribute() {}, removeAttribute() {}, classList: { add() {}, remove() {} } }) };
  class FakeXHR {
    constructor() {
      this.listeners = {};
      this.upload = { addEventListener: (name, fn) => { this.uploadListener = fn; } };
    }
    addEventListener(name, fn) { this.listeners[name] = fn; }
    open(method, url) { this.method = method; this.url = url; }
    send(body) { this.body = body; onSend(this, body); }
    emit(name) { this.listeners[name]?.(); }
    progress(loaded, total) { this.uploadListener?.({ lengthComputable: true, loaded, total }); }
  }
  class FakeDOMParser {
    parseFromString(source) {
      return { querySelector: (selector) => selector === '[role="alert"]' && source.includes('role="alert"') ? { textContent: "Error de prueba seguro." } : null };
    }
  }
  return new Function("window", "document", "XMLHttpRequest", "FormData", "DOMParser", `${code}; return window;`)(window, document, FakeXHR, FormData, FakeDOMParser);
}

function form() {
  const status = {
    textContent: "", classes: new Set(["status"]), attributes: { hidden: "" },
    classList: { add(c) { status.classes.add(c); }, remove(c) { status.classes.delete(c); } },
    setAttribute(k, v) { this.attributes[k] = v; }, removeAttribute(k) { delete this.attributes[k]; },
  };
  const progress = {
    value: 0, attributes: { hidden: "" },
    setAttribute(k, v) { this.attributes[k] = v; }, removeAttribute(k) { delete this.attributes[k]; },
  };
  const button = { disabled: false };
  const file = { files: [new File(["abc"], "a.pdf", { type: "application/pdf" })], disabled: false };
  const csrf = { value: "csrf" };
  const el = {
    dataset: { statusUploading: "Subiendo archivo...", statusSuccess: "Carga completa.", statusError: "No se pudo cargar el archivo.", statusChooseFile: "Elige un archivo." }, action: "/w/files",
    querySelector(sel) {
      if (sel === "button" || sel === 'button[type="submit"]') return button;
      if (sel === 'input[type="file"]') return file;
      if (sel === 'input[name="csrf"]') return csrf;
      if (sel === "[data-upload-status]") return status;
      if (sel === "[data-upload-progress]") return progress;
      return null;
    },
    appendChild: (x) => x,
  };
  return { el, status, progress, button, file };
}

// Outcomes of the one multipart POST.
const UPLOAD = ["ok", "http", "throw"];
const paths = () => UPLOAD.map((outcome) => [outcome]);

function respond(xhr, outcome) {
  if (outcome === "throw") {
    xhr.emit("error");
    return;
  }
  xhr.status = outcome === "http" ? 422 : 200;
  xhr.responseText = outcome === "http" ? '<p role="alert">Error de prueba seguro.</p>' : "";
  xhr.emit("load");
}

const IN_FLIGHT = new Set(["UPLOADING", "SUCCESS"]);

function checkRules(u, f, label) {
  assert.equal(f.button.disabled, IN_FLIGHT.has(u.phase), `${label}: button disabled in ${u.phase}`);
  assert.equal(f.file.disabled, IN_FLIGHT.has(u.phase), `${label}: file input disabled in ${u.phase}`);
  if (u.phase !== "IDLE") {
    assert.ok(f.status.textContent, `${label}: empty status in ${u.phase}`);
    assert.ok(!("hidden" in f.status.attributes), `${label}: status hidden in ${u.phase}`);
  }
  assert.equal(f.status.classes.has("error"), u.phase === "ERROR", `${label}: error style in ${u.phase}`);
  assert.equal("hidden" in f.progress.attributes, u.phase !== "UPLOADING" || f.progress.value === 0, `${label}: progress visibility in ${u.phase}`);
}

const flush = () => new Promise((r) => setImmediate(r));

for (const outcomes of paths()) {
  for (let presses = 0; presses < 1 << outcomes.length; presses++) {
    const label = `${outcomes.join(">")} extra-press-mask=${presses}`;
    test(`upload form: ${label}`, { timeout: 5000 }, async () => {
      const calls = [];
      const release = [];
      const window = load((xhr) => {
        calls.push(xhr);
        const stage = calls.length - 1;
        release.push(() => respond(xhr, outcomes[stage]));
      });
      const f = form();
      const u = new window.FileUploader(f.el);
      const run = u.handleSubmit({ preventDefault() {} });
      for (let stage = 0; stage < outcomes.length; stage++) {
        for (let i = 0; i < 20 && calls.length <= stage; i++) await flush();
        assert.equal(calls.length, stage + 1, `${label}: request ${stage} never started`);
        assert.equal(calls[stage].method, "POST", `${label}: upload method`);
        assert.equal(calls[stage].url, "/w/files", `${label}: upload endpoint`);
        assert.ok(calls[stage].body instanceof FormData, `${label}: multipart request body`);
        assert.equal(calls[stage].body.get("csrf"), "csrf", `${label}: CSRF token`);
        assert.equal(calls[stage].body.get("file").name, "a.pdf", `${label}: selected file`);
        assert.ok(IN_FLIGHT.has(u.phase), `${label}: phase ${u.phase} while request ${stage} is in flight`);
        checkRules(u, f, label);
        calls[stage].progress(25, 100);
        assert.equal(f.progress.value, 25, `${label}: byte progress value`);
        assert.match(f.status.textContent, /25%/, `${label}: visible percentage`);
        calls[stage].progress(30, 100);
        assert.equal(f.progress.value, 30, `${label}: progress bar tracks each computable event`);
        assert.match(f.status.textContent, /25%/, `${label}: live status only announces each quarter`);
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
      assert.deepEqual(window.location.assigned, success ? ["/w/files"] : [], label);
      if (!success) {
        if (outcomes[0] === "http") assert.equal(f.status.textContent, "Error de prueba seguro.", label);
        const before = calls.length;
        const retry = u.handleSubmit({ preventDefault() {} });
        assert.equal(calls.length, before + 1, `${label}: retry did not start a new upload`);
        assert.equal(u.phase, "UPLOADING", label);
        checkRules(u, f, label);
        respond(calls.at(-1), "ok");
        await retry;
      }
    });
  }
}

test("upload form: pressing with no file shows a localized error and sends nothing", async () => {
  const window = load(() => assert.fail("no-file submit started a request"));
  const f = form();
  f.file.files = [];
  const u = new window.FileUploader(f.el);
  await u.handleSubmit({ preventDefault() {} });
  assert.equal(u.phase, "ERROR");
  assert.equal(f.status.textContent, "Elige un archivo.");
  checkRules(u, f, "no file");
});

test("upload form: fallback phase copy remains available without localized page data", () => {
  const window = load(() => assert.fail("phase transition started a request"));
  const f = form();
  f.el.dataset = {};
  const u = new window.FileUploader(f.el);
  u.transition("UPLOADING");
  assert.equal(f.status.textContent, "Uploading file...");
  u.transition("ERROR");
  assert.equal(f.status.textContent, "Upload failed. Please try again.");
});
