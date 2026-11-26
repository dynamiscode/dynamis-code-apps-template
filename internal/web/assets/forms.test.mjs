// Exhaustive checks of the pure evaluators in forms.js. Each test enumerates
// every input in a stated bound and checks a rule from the form-field-input,
// form-drafts, form-wizard, or async-feedback skill.
import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const forms = require("./alpine-components.js").core;

// Every string over `alphabet` up to `max` characters.
function* strings(alphabet, max) {
  let level = [""];
  yield "";
  for (let n = 1; n <= max; n++) {
    const next = [];
    for (const prefix of level) for (const c of alphabet) { next.push(prefix + c); yield prefix + c; }
    level = next;
  }
}

// Go's normalizeTOTPCode (internal/identity/mfa.go): drop spaces, dashes, tabs.
const goTOTP = (s) => s.replace(/[ \t-]/g, "");
// Go's normalizeRecoveryCode: strings.ToUpper(strings.Join(strings.Fields(s), "")).
const goRecovery = (s) => s.split(/\s+/).join("").toUpperCase();

test("password meter: a score above 0 only for passwords the server accepts", () => {
  const classes = ["a", "B", "7", "!"];
  for (let mask = 1; mask < 16; mask++) {
    const pool = classes.filter((_, i) => mask & (1 << i));
    let previous = 0;
    for (let length = 0; length <= 40; length++) {
      const value = Array.from({ length }, (_, i) => pool[i % pool.length]).join("");
      const score = forms.passwordStrength(value);
      assert.ok(score >= 0 && score <= 4, `${value}: score ${score}`);
      assert.equal(score > 0, length >= forms.MIN_PASSWORD, `${value}: score ${score} at length ${length}`);
      if (pool.length > 1) assert.ok(score >= previous, `${value}: score fell from ${previous} to ${score}`);
      previous = score;
    }
  }
  for (const c of ["a", "Z", "1", "!"]) assert.ok(forms.passwordStrength(c.repeat(40)) <= 1, `repeated ${c}`);
});

test("authenticator mask agrees with the server for every short input", () => {
  let count = 0;
  for (const s of strings(["1", "2", " ", "-", "a"], 8)) {
    count++;
    const masked = forms.maskDigits(s, 6);
    assert.match(masked, /^\d{0,6}$/, `${JSON.stringify(s)} -> ${masked}`);
    const server = goTOTP(s);
    // Whatever the server accepts as a 6-digit code, the mask sends unchanged.
    if (/^\d{6}$/.test(server)) assert.equal(masked, server, JSON.stringify(s));
  }
  assert.ok(count > 400000);
});

test("recovery mask equals the server normalizer for every short input", () => {
  for (const s of strings(["a", "B", "-", "_", " ", "\t"], 6)) {
    assert.equal(forms.maskUpper(s), goRecovery(s), JSON.stringify(s));
  }
});

test("draft decision: restore unless the server accepted the submit", () => {
  for (const stored of [null, { submitted: false }, { submitted: true }]) {
    for (const pageHasError of [false, true]) {
      const action = forms.draftAction(stored, pageHasError);
      if (!stored) assert.equal(action, "none");
      else if (stored.submitted && !pageHasError) assert.equal(action, "discard");
      else assert.equal(action, "restore", JSON.stringify({ stored, pageHasError }));
    }
  }
});

test("dirty check: dirty exactly when any field differs from its first value", () => {
  const values = ["", "a", true, false];
  for (const a of values) for (const b of values) for (const c of values) for (const d of values) {
    const dirty = forms.isDirty([["x", a], ["y", b]], [["x", c], ["y", d]]);
    assert.equal(dirty, a !== c || b !== d);
  }
});

test("notes show unsaved feedback and warn only when leaving an unsaved form", () => {
  const documentEvents = {}, windowEvents = {};
  const status = { hidden: true };
  class Element {}
  class HTMLSelectElement extends Element {}
  class HTMLFormElement extends Element {
    constructor(rejected = false) {
      super();
      this.dataset = {};
      this.rejected = rejected;
      this.elements = [];
    }
    matches(selector) { return selector === "[data-dirty-track], [data-unsaved-warning]"; }
    hasAttribute(name) { return name === "data-dirty-on-load" && this.rejected; }
    querySelector(selector) { return selector === "[data-dirty-status]" ? status : null; }
    querySelectorAll() { return []; }
  }
  const form = new HTMLFormElement();
  const field = Object.assign(new Element(), {
    form, tagName: "TEXTAREA", name: "notes", type: "textarea", value: "Saved notes",
    dataset: {}, willValidate: false, getAttribute: () => null, matches: () => false
  });
  form.elements.push(field);
  const document = {
    querySelector: () => null,
    querySelectorAll: (selector) => selector === "form" || selector === "form[data-unsaved-warning]" ? [form] : [],
    addEventListener: (name, listener) => (documentEvents[name] ||= []).push(listener)
  };
  const window = {
    sessionStorage: { setItem() {}, removeItem() {} },
    addEventListener: (name, listener) => (windowEvents[name] ||= []).push(listener)
  };
  const module = { exports: {} };
  const source = readFileSync(new URL("./alpine-components.js", import.meta.url), "utf8");
  new Function("module", "window", "document", "Element", "HTMLFormElement", "HTMLSelectElement", "setTimeout", source)(
    module, window, document, Element, HTMLFormElement, HTMLSelectElement, () => {}
  );
  module.exports.wireForms();
  const fire = (listeners, name, event) => (listeners[name] || []).forEach((listener) => listener(event));
  const leavesWithWarning = () => {
    const event = { prevented: false, preventDefault() { this.prevented = true; } };
    fire(windowEvents, "beforeunload", event);
    return event.prevented;
  };
  assert.equal(status.hidden, true);
  assert.equal(leavesWithWarning(), false);
  field.value = "Changed notes";
  fire(documentEvents, "input", { target: field });
  assert.equal(status.hidden, false);
  assert.equal(leavesWithWarning(), true);
  field.value = "Saved notes";
  fire(documentEvents, "input", { target: field });
  assert.equal(status.hidden, true);
  assert.equal(leavesWithWarning(), false);
  field.value = "Changed notes";
  fire(documentEvents, "input", { target: field });
  fire(documentEvents, "submit", { target: form });
  assert.equal(leavesWithWarning(), false);
});

test("wizard: never skips a step and moves forward only from a valid step", () => {
  for (let total = 1; total <= 6; total++) {
    for (let step = 0; step < total; step++) {
      for (const direction of ["next", "back", "other"]) {
        for (const valid of [false, true]) {
          const to = forms.wizardMove(step, total, direction, valid);
          assert.ok(to >= 0 && to < total);
          assert.ok(Math.abs(to - step) <= 1);
          if (to > step) assert.ok(direction === "next" && valid);
          if (to < step) assert.equal(direction, "back");
        }
      }
    }
  }
});

test("realtime refresh waits while you edit", () => {
  assert.equal(forms.refreshDecision(true), "defer");
  assert.equal(forms.refreshDecision(false), "refresh");
});

test("optimistic removal: the row comes back on every failure", () => {
  assert.equal(forms.optimisticOutcome("pending"), "hidden");
  for (let status = 100; status < 600; status++) {
    const outcome = forms.optimisticOutcome(status);
    assert.equal(outcome, status >= 200 && status < 400 ? "removed" : "restored", String(status));
  }
  for (const missing of [undefined, null, 0, "timeout"]) assert.equal(forms.optimisticOutcome(missing), "restored");
});

test("lazy audit response must stay on the audit route", () => {
	const base = "https://example.test/workspaces/abc/settings/audit";
	assert.equal(forms.isAuditFragment({ ok: true, redirected: true, url: "https://example.test/login" }, base, ""), false);
	assert.equal(forms.isAuditFragment({ ok: false, redirected: false, url: `${base}?part=table` }, base, "audit-table"), false);
	assert.equal(forms.isAuditFragment({ ok: true, redirected: false, url: "https://example.test/workspaces/abc/settings/general?part=table" }, base, "audit-table"), false);
	assert.equal(forms.isAuditFragment({ ok: true, redirected: false, url: "https://example.test/other/workspaces/abc/settings/audit?part=table" }, base, "audit-table"), false);
	assert.equal(forms.isAuditFragment({ ok: true, redirected: false, url: `${base}?part=table` }, base, "HTML"), false);
	assert.equal(forms.isAuditFragment({ ok: true, redirected: false, url: `${base}?part=table` }, base, "audit-table"), true);
});

test("password reveal glyph: crossed eye only when shown", () => {
	const closed = forms.revealIconPaths(false);
	const shown = forms.revealIconPaths(true);
	assert.ok(closed.includes('<circle cx="12" cy="12" r="3"/>'));
	assert.ok(!closed.includes('<line x1="1" y1="1" x2="23" y2="23"/>'));
	assert.ok(shown.includes('<line x1="1" y1="1" x2="23" y2="23"/>'));
	assert.ok(!shown.includes("<circle"));
	assert.equal(forms.revealIconPaths(false), closed);
	assert.equal(forms.revealIconPaths(true), shown);
});

test("confirmed selectors submit only after confirmation and restore canceled choices", () => {
  const { wireConfirmSelects } = require("./alpine-components.js");
  const control = () => ({
    listeners: {}, addEventListener(type, fn) { this.listeners[type] = fn; },
    fire(type, event = {}) { this.listeners[type]?.(event); },
    focus() { this.focused = true; }
  });
  const select = Object.assign(control(), {
    selectedIndex: 0, options: [{ value: "old", textContent: "Old" }, { value: "new", textContent: "New" }],
    dispatchEvent(event) { this.lastEvent = event.type; }
  });
  Object.defineProperties(select, {
    selectedOptions: { get() { return [this.options[this.selectedIndex]]; } },
    value: { get() { return this.options[this.selectedIndex].value; } }
  });
  const cancel = control(), accept = control(), value = {};
  const dialog = Object.assign(control(), {
    querySelector(selector) {
      return { "[data-confirm-select-value]": value, "[data-confirm-select-cancel]": cancel, "[data-confirm-select-accept]": accept }[selector];
    },
    showModal() { this.open = true; },
    close() { this.open = false; this.fire("close"); }
  });
  const fallback = { hidden: false };
  const form = {
    dataset: {}, submits: 0,
    querySelector(selector) {
      return { select, "[data-confirm-select-dialog]": dialog, "[data-confirm-select-fallback]": fallback }[selector];
    },
    requestSubmit() { this.submits++; }
  };
  const root = { querySelectorAll: () => [form] };
  wireConfirmSelects(root);
  wireConfirmSelects(root);
  assert.equal(fallback.hidden, true);
  select.fire("change");
  assert.equal(dialog.open, undefined);
  select.selectedIndex = 1;
  select.fire("change");
  assert.equal(dialog.open, true);
  assert.equal(value.textContent, "New");
  assert.equal(cancel.focused, true);
  cancel.fire("click");
  assert.equal(select.selectedIndex, 0);
  assert.equal(select.lastEvent, "input");
  assert.equal(form.submits, 0);
  select.selectedIndex = 1;
  select.fire("change");
  dialog.close(); // Escape closes the native dialog.
  assert.equal(select.selectedIndex, 0);
  select.selectedIndex = 1;
  select.fire("change");
  dialog.fire("click", { target: dialog });
  assert.equal(select.selectedIndex, 0);
  select.selectedIndex = 1;
  select.fire("change");
  accept.fire("click");
  assert.equal(select.selectedIndex, 1);
  assert.equal(form.submits, 1);
  assert.equal(select.focused, true);

  const unsupportedFallback = { hidden: false };
  const unsupported = {
    dataset: {},
    querySelector(selector) {
      return { select, "[data-confirm-select-dialog]": { querySelector: dialog.querySelector }, "[data-confirm-select-fallback]": unsupportedFallback }[selector];
    }
  };
  wireConfirmSelects({ querySelectorAll: () => [unsupported] });
  assert.equal(unsupportedFallback.hidden, false);
});
