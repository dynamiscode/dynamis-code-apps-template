// Exhaustive checks of the pure evaluators in forms.js. Each test enumerates
// every input in a stated bound and checks a rule from the form-field-input,
// form-drafts, form-wizard, or async-feedback skill.
import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const forms = require("./forms.js");

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
