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

  const fn = new Function("window", "document", `${appJsCode}; return { window, PresignedUploader: window.PresignedUploader };`);
  const result = fn(window, document);
  return { uploaderClass: result.PresignedUploader, window: result.window };
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
    files: options.files || [{ name: "test.pdf", size: 512, type: "application/pdf" }],
    disabled: false
  };
  const csrfInput = { value: "mock-csrf-token" };

  const listeners = {};
  const form = {
    dataset: options.dataset || { initiate: "/workspaces/ws1/files/initiate" },
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

test("PresignedUploader defines explicit phases and phase messages", () => {
  const { uploaderClass } = loadUploader();
  assert.ok(uploaderClass, "PresignedUploader class should be exported to window");

  const expectedPhases = {
    IDLE: "IDLE",
    INITIATING: "INITIATING",
    UPLOADING: "UPLOADING",
    COMPLETING: "COMPLETING",
    SUCCESS: "SUCCESS",
    ERROR: "ERROR"
  };
  assert.deepEqual(uploaderClass.PHASES, expectedPhases);
  assert.deepEqual(uploaderClass.Phases, expectedPhases);

  assert.equal(uploaderClass.PHASE_MESSAGES.INITIATING, "Initiating upload...");
  assert.equal(uploaderClass.PHASE_MESSAGES.UPLOADING, "Uploading file...");
  assert.equal(uploaderClass.PHASE_MESSAGES.COMPLETING, "Completing upload...");
  assert.equal(uploaderClass.PHASE_MESSAGES.SUCCESS, "Upload complete.");
  assert.equal(uploaderClass.PHASE_MESSAGES.ERROR, "Upload failed. Please try again.");
});

test("PresignedUploader initial state is IDLE with enabled controls", () => {
  const { uploaderClass } = loadUploader();
  const { form, submitButton, fileInput } = createMockForm();
  const uploader = new uploaderClass(form);

  assert.equal(uploader.phase, "IDLE");
  assert.equal(submitButton.disabled, false);
  assert.equal(fileInput.disabled, false);
});

test("PresignedUploader disables controls during in-flight states and updates status", () => {
  const { uploaderClass } = loadUploader();
  const { form, submitButton, fileInput, statusElement } = createMockForm();
  const uploader = new uploaderClass(form);

  uploader.transition(uploaderClass.PHASES.INITIATING);
  assert.equal(uploader.phase, "INITIATING");
  assert.equal(submitButton.disabled, true);
  assert.equal(fileInput.disabled, true);
  assert.equal(statusElement.textContent, "Initiating upload...");
  assert.equal(statusElement.hasAttribute("hidden"), false);
  assert.ok(statusElement.classList.contains("status"));
  assert.ok(!statusElement.classList.contains("error"));

  uploader.transition(uploaderClass.PHASES.UPLOADING);
  assert.equal(uploader.phase, "UPLOADING");
  assert.equal(submitButton.disabled, true);
  assert.equal(fileInput.disabled, true);
  assert.equal(statusElement.textContent, "Uploading file...");

  uploader.transition(uploaderClass.PHASES.COMPLETING);
  assert.equal(uploader.phase, "COMPLETING");
  assert.equal(submitButton.disabled, true);
  assert.equal(fileInput.disabled, true);
  assert.equal(statusElement.textContent, "Completing upload...");
});

test("PresignedUploader transitions to ERROR, re-enables controls, and displays friendly error without reload", () => {
  const { uploaderClass, window } = loadUploader();
  const { form, submitButton, fileInput, statusElement } = createMockForm();
  const uploader = new uploaderClass(form);

  uploader.transition(uploaderClass.PHASES.INITIATING);
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

test("PresignedUploader handleSubmit validates presence of file", async () => {
  const { uploaderClass } = loadUploader();
  const { form, submitButton, statusElement } = createMockForm({ files: [] });
  const uploader = new uploaderClass(form);

  await uploader.handleSubmit();
  assert.equal(uploader.phase, "ERROR");
  assert.equal(submitButton.disabled, false);
  assert.equal(statusElement.textContent, "Choose a file to upload.");
});

test("PresignedUploader handles successful upload lifecycle", async () => {
  const { uploaderClass, window } = loadUploader();
  const { form, submitButton, statusElement } = createMockForm();
  const originalFetch = globalThis.fetch;

  const fetchCalls = [];
  globalThis.fetch = async (url, opts) => {
    fetchCalls.push({ url, method: opts.method });
    if (url.endsWith("/initiate")) {
      return {
        ok: true,
        json: async () => ({
          uploadUrl: "https://storage.example.com/object-key",
          completeUrl: "/workspaces/ws1/files/file-1/complete",
          uploadHeaders: { "x-custom-header": "test" }
        })
      };
    }
    if (url.includes("storage.example.com")) {
      return { ok: true };
    }
    if (url.endsWith("/complete")) {
      return { ok: true };
    }
    throw new Error(`Unexpected URL: ${url}`);
  };

  try {
    const uploader = new uploaderClass(form);
    await uploader.handleSubmit();

    assert.equal(uploader.phase, "SUCCESS");
    assert.equal(statusElement.textContent, "Upload complete.");
    assert.equal(window.location.assignCalls.length, 1);
    assert.equal(window.location.assignCalls[0], "/workspaces/ws1/files");
    assert.equal(fetchCalls.length, 3);
  } finally {
    globalThis.fetch = originalFetch;
  }
});
