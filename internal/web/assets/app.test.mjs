import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const appJsPath = path.join(__dirname, "app.js");
const appJsCode = fs.readFileSync(appJsPath, "utf8");

function loadUploader() {
  const window = {
    location: {
      assignCalls: [],
      assign(url) { this.assignCalls.push(url); },
      reloadCalls: 0,
      reload() { this.reloadCalls++; }
    }
  };
  const document = {
    querySelector: () => null,
    addEventListener: () => {}
  };

  const fn = new Function("window", "document", `${appJsCode}; return { window, FileUploader: window.FileUploader };`);
  const result = fn(window, document);
  return { uploaderClass: result.FileUploader, window: result.window };
}

function createMockForm(options = {}) {
  const statusElement = {
    textContent: "",
    classes: ["status"],
    classList: {
      add(cls) { if (!statusElement.classes.includes(cls)) statusElement.classes.push(cls); },
      remove(cls) { statusElement.classes = statusElement.classes.filter((c) => c !== cls); },
      contains(cls) { return statusElement.classes.includes(cls); }
    },
    attributes: { hidden: "" },
    setAttribute(k, v) { this.attributes[k] = v; },
    removeAttribute(k) { delete this.attributes[k]; },
    hasAttribute(k) { return k in this.attributes; }
  };
  const submitButton = { disabled: false };
  const fileInput = {
    files: options.files || [new File(["%PDF-1.4"], "test.pdf", { type: "application/pdf" })],
    disabled: false
  };
  const csrfInput = { value: "mock-csrf-token" };

  const listeners = {};
  const form = {
    dataset: {},
    action: options.action || "/workspaces/ws1/files",
    querySelector(sel) {
      if (sel === "button" || sel === 'button[type="submit"]') return submitButton;
      if (sel === 'input[type="file"]') return fileInput;
      if (sel === 'input[name="csrf"]') return csrfInput;
      if (sel === "[data-upload-status]" || sel === '[role="status"]' || sel === '[aria-live="polite"]') return statusElement;
      return null;
    },
    appendChild(el) { return el; },
    addEventListener(evt, fn) { listeners[evt] = fn; },
    trigger(evt, data) { if (listeners[evt]) return listeners[evt](data); }
  };

  return { form, submitButton, fileInput, statusElement, listeners };
}

// A minimal stand-in for DOMParser: finds the first role="alert" element in the HTML.
function withAlertParser(run) {
  const original = globalThis.DOMParser;
  globalThis.DOMParser = class {
    parseFromString(text) {
      const match = /role="alert"[^>]*>([^<]*)</.exec(text);
      return { querySelector: () => (match ? { textContent: match[1] } : null) };
    }
  };
  return Promise.resolve().then(run).finally(() => { globalThis.DOMParser = original; });
}

test("FileUploader defines single-request phases and messages", () => {
  const { uploaderClass } = loadUploader();
  assert.ok(uploaderClass, "FileUploader class should be exported to window");

  const expectedPhases = {
    IDLE: "IDLE",
    UPLOADING: "UPLOADING",
    SUCCESS: "SUCCESS",
    ERROR: "ERROR"
  };
  assert.deepEqual(uploaderClass.PHASES, expectedPhases);
  assert.deepEqual(uploaderClass.Phases, expectedPhases);

  assert.equal(uploaderClass.PHASE_MESSAGES.UPLOADING, "Uploading file...");
  assert.equal(uploaderClass.PHASE_MESSAGES.SUCCESS, "Upload complete.");
  assert.equal(uploaderClass.PHASE_MESSAGES.ERROR, "Upload failed. Please try again.");
});

test("FileUploader initial state is IDLE with enabled controls", () => {
  const { uploaderClass } = loadUploader();
  const { form, submitButton, fileInput } = createMockForm();
  const uploader = new uploaderClass(form);

  assert.equal(uploader.phase, "IDLE");
  assert.equal(submitButton.disabled, false);
  assert.equal(fileInput.disabled, false);
});

test("FileUploader locks controls while UPLOADING and shows status", () => {
  const { uploaderClass } = loadUploader();
  const { form, submitButton, fileInput, statusElement } = createMockForm();
  const uploader = new uploaderClass(form);

  uploader.transition(uploaderClass.PHASES.UPLOADING);
  assert.equal(uploader.phase, "UPLOADING");
  assert.equal(submitButton.disabled, true);
  assert.equal(fileInput.disabled, true);
  assert.equal(statusElement.textContent, "Uploading file...");
  assert.equal(statusElement.hasAttribute("hidden"), false);
  assert.ok(statusElement.classList.contains("status"));
  assert.ok(!statusElement.classList.contains("error"));
});

test("FileUploader transitions to ERROR, re-enables controls, and displays error without reload", () => {
  const { uploaderClass, window } = loadUploader();
  const { form, submitButton, fileInput, statusElement } = createMockForm();
  const uploader = new uploaderClass(form);

  uploader.transition(uploaderClass.PHASES.UPLOADING);
  assert.equal(submitButton.disabled, true);

  uploader.transition(uploaderClass.PHASES.ERROR, "File size exceeds limit.");
  assert.equal(uploader.phase, "ERROR");
  assert.equal(submitButton.disabled, false, "Submit button must be re-enabled on error");
  assert.equal(fileInput.disabled, false, "File input must be re-enabled on error");
  assert.equal(statusElement.textContent, "File size exceeds limit.");
  assert.ok(statusElement.classList.contains("error"));
  assert.ok(!statusElement.classList.contains("status"));
  assert.equal(window.location.reloadCalls, 0, "window.location.reload must NOT be called");
});

test("FileUploader handleSubmit validates presence of file without sending a request", async () => {
  const { uploaderClass } = loadUploader();
  const { form, submitButton, statusElement } = createMockForm({ files: [] });
  const uploader = new uploaderClass(form);
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () => { throw new Error("fetch must not be called"); };

  try {
    await uploader.handleSubmit();
  } finally {
    globalThis.fetch = originalFetch;
  }
  assert.equal(uploader.phase, "ERROR");
  assert.equal(submitButton.disabled, false);
  assert.equal(statusElement.textContent, "Choose a file to upload.");
});

test("FileUploader sends the file as one multipart request, locks while in flight, and reports success", async () => {
  const { uploaderClass, window } = loadUploader();
  const { form, submitButton, statusElement } = createMockForm();
  const originalFetch = globalThis.fetch;

  const fetchCalls = [];
  globalThis.fetch = async (url, opts) => {
    fetchCalls.push({ url, method: opts.method, body: opts.body, lockedDuringRequest: submitButton.disabled });
    return { ok: true, status: 200, text: async () => "" };
  };

  try {
    const uploader = new uploaderClass(form);
    await uploader.handleSubmit();

    assert.equal(fetchCalls.length, 1, "exactly one request");
    const [call] = fetchCalls;
    assert.equal(call.url, "/workspaces/ws1/files");
    assert.equal(call.method, "POST");
    assert.ok(call.body instanceof FormData, "body is multipart form data");
    assert.equal(call.body.get("csrf"), "mock-csrf-token");
    assert.equal(call.body.get("file").name, "test.pdf");
    assert.equal(call.lockedDuringRequest, true, "button is locked while the request is in flight");

    assert.equal(uploader.phase, "SUCCESS");
    assert.equal(statusElement.textContent, "Upload complete.");
    assert.equal(submitButton.disabled, true, "button stays locked until the page navigates");
    assert.deepEqual(window.location.assignCalls, ["/workspaces/ws1/files"]);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("FileUploader shows oversize, storage limit, and invalid file messages and unlocks after failure", async () => {
  const cases = [
    "The file exceeds the per-file size limit.",
    "The workspace file storage limit was reached.",
    "The selected file is invalid."
  ];
  for (const message of cases) {
    const { uploaderClass, window } = loadUploader();
    const { form, submitButton, fileInput, statusElement } = createMockForm();
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => ({
      ok: false,
      status: 422,
      text: async () => `<p class="error" role="alert" tabindex="-1">${message}</p>`
    });

    try {
      await withAlertParser(async () => {
        const uploader = new uploaderClass(form);
        await uploader.handleSubmit();

        assert.equal(uploader.phase, "ERROR", message);
        assert.equal(statusElement.textContent, message);
        assert.ok(statusElement.classList.contains("error"));
        assert.equal(submitButton.disabled, false, `button unlocked after: ${message}`);
        assert.equal(fileInput.disabled, false, `file input unlocked after: ${message}`);
        assert.deepEqual(window.location.assignCalls, [], "no navigation on failure");
      });
    } finally {
      globalThis.fetch = originalFetch;
    }
  }
});

test("FileUploader reports network failure and unlocks controls", async () => {
  const { uploaderClass, window } = loadUploader();
  const { form, submitButton, statusElement } = createMockForm();
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () => { throw new TypeError("Failed to fetch"); };

  try {
    const uploader = new uploaderClass(form);
    await uploader.handleSubmit();

    assert.equal(uploader.phase, "ERROR");
    assert.equal(statusElement.textContent, "Upload failed. Please try again.");
    assert.equal(submitButton.disabled, false);
    assert.deepEqual(window.location.assignCalls, []);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("FileUploader falls back to the default failure message when the server gives no alert", async () => {
  const { uploaderClass } = loadUploader();
  const { form, statusElement } = createMockForm();
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () => ({ ok: false, status: 500, text: async () => "" });

  try {
    const uploader = new uploaderClass(form);
    await uploader.handleSubmit();

    assert.equal(uploader.phase, "ERROR");
    assert.equal(statusElement.textContent, "Upload failed. Please try again.");
  } finally {
    globalThis.fetch = originalFetch;
  }
});
