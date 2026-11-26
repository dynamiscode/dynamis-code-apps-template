import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const { wireEntryValidation } = createRequire(import.meta.url)("./alpine-components.js");

function fixture() {
  const listeners = {};
  const doc = { addEventListener(name, fn) { (listeners[name] ||= []).push(fn); }, querySelectorAll() { return [form]; } };
  const submit = { disabled: false, formNoValidate: false, hasAttribute: () => false };
  const search = { disabled: false, formNoValidate: true, hasAttribute: () => false };
  const form = { dataset: {}, elements: [], hasAttribute: () => false, querySelector: () => ({}), closest: () => null, querySelectorAll: () => [submit, search] };
  form.elements.namedItem = (name) => form.elements.find((el) => el.name === name);
  const field = (options = {}) => {
    const el = {
      dataset: {}, form, value: "", tagName: "INPUT", type: "text", willValidate: true,
      required: false, disabled: false, selectionStart: 0, selectionEnd: 0,
      attrs: new Set(), hidden: false, parent: null,
      get validity() { return { valid: !this.required || !!this.value }; },
      hasAttribute(name) { return this.attrs.has(name); },
      closest(selector) { if (selector === "[hidden]") return this.hidden ? {} : null; if (selector === "[data-entry-number]") return this.parent; return null; },
      setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; },
      ...options,
    };
    form.elements.push(el);
    return el;
  };
  const fire = (name, target, extra = {}) => { for (const listener of listeners[name] || []) listener({ target, ...extra }); };
  const change = (el, value) => { fire("beforeinput", el); el.value = value; fire("input", el); };
  return { doc, form, submit, search, field, fire, change };
}

test("entry submit tracks required fields, conditional visibility, selection and picker actions", () => {
  const f = fixture();
  const name = f.field({ required: true });
  const choice = f.field({ required: true, tagName: "SELECT", selectedOptions: [{ disabled: false }] });
  const conditional = f.field({ required: true, hidden: true });
  const selected = f.field({ willValidate: false, value: "0", attrs: new Set(["data-entry-required"]) });
  wireEntryValidation(f.doc);
  assert.equal(f.submit.disabled, true);
  assert.equal(f.search.disabled, false);
  f.change(name, "Ada");
  f.change(choice, "card");
  selected.value = "42";
  f.fire("change", selected);
  assert.equal(f.submit.disabled, false);
  conditional.hidden = false;
  f.fire("change", choice);
  assert.equal(f.submit.disabled, true);
  f.change(conditional, "provided");
  assert.equal(f.submit.disabled, false);
  choice.selectedOptions[0].disabled = true;
  f.fire("change", choice);
  assert.equal(f.submit.disabled, true);
  choice.selectedOptions[0].disabled = false;
  name.value = "   ";
  f.fire("input", name);
  assert.equal(f.submit.disabled, true);
});

test("positive amounts reject localized zero without changing the entered value", () => {
  const f = fixture();
  const amount = f.field({ required: true, dataset: { entryNumber: "amount" }, attrs: new Set(["data-entry-number-positive"]) });
  wireEntryValidation(f.doc);
  for (const zero of ["0.00", "٠٫٠٠", "۰٫۰۰", "०.००", "০.০০", "𝟘.𝟘𝟘"]) {
    f.change(amount, zero);
    assert.equal(amount.value, zero);
    assert.equal(f.submit.disabled, true, zero);
  }
  for (const positive of ["0.01", "٠٫٠١", "۰٫۰۱", "०.०१", "০.০১", "𝟘.𝟘𝟙"]) {
    f.change(amount, positive);
    assert.equal(amount.value, positive);
    assert.equal(f.submit.disabled, false, positive);
  }
});

test("transaction amounts follow server locale, currency precision, grouping and int64 limits", () => {
  const f = fixture();
  const currency = f.field({ name: "currency", tagName: "SELECT", selectedOptions: [{ dataset: { entryAmountScale: "2" } }] });
  const amount = f.field({ required: true, dataset: {
    entryNumber: "amount", entryAmountCurrency: "currency",
    entryAmountDigits: JSON.stringify(Array.from("0123456789")), entryAmountDecimal: ".",
    entryAmountGroup: ",", entryAmountPrimary: "3", entryAmountSecondary: "3"
  }, attrs: new Set(["data-entry-number-positive"]) });
  wireEntryValidation(f.doc);
  for (const value of ["1,234.50", "92233720368547758.07", "0.01", "\u00851.00\u0085"]) {
    f.change(amount, value);
    assert.equal(amount.value, value);
    assert.equal(f.submit.disabled, false, value);
  }
  f.change(amount, "\ufeff1.00\ufeff");
  assert.equal(amount.value, "\u00851.00\u0085");
  amount.value = "\ufeff1.00\ufeff";
  f.fire("change", amount);
  assert.equal(f.submit.disabled, true);
  for (const value of ["12,34.00", "1.001", "1,25", "1..00", "١.٠٠", "92233720368547758.08"]) {
    f.change(amount, value);
    assert.equal(amount.value, value);
    assert.equal(f.submit.disabled, true, value);
  }
  currency.selectedOptions[0].dataset.entryAmountScale = "0";
  f.change(amount, "1.0");
  assert.equal(f.submit.disabled, true);
  f.change(amount, "123");
  assert.equal(f.submit.disabled, false);
  currency.selectedOptions[0].dataset.entryAmountScale = "2";
  Object.assign(amount.dataset, { entryAmountDigits: JSON.stringify(Array.from("٠١٢٣٤٥٦٧٨٩")), entryAmountDecimal: "٫", entryAmountGroup: "٬" });
  f.fire("change", currency);
  f.change(amount, "١٬٢٣٤٫٥٦");
  assert.equal(amount.value, "١٬٢٣٤٫٥٦");
  assert.equal(f.submit.disabled, false);
  f.change(amount, "١٬٢٣٫٤٥");
  assert.equal(f.submit.disabled, true);
  f.change(amount, "١٫٢٥");
  assert.equal(f.submit.disabled, false);
  currency.selectedOptions[0].dataset.entryAmountScale = "0";
  f.fire("change", currency);
  assert.equal(f.submit.disabled, true);
});

test("amount input preserves localized paste, caret, IME and signed values", () => {
  const f = fixture();
  const amount = f.field({ required: true, dataset: { entryNumber: "amount" } });
  wireEntryValidation(f.doc);
  f.change(amount, "1,25");
  assert.equal(amount.value, "1,25");
  assert.equal(f.submit.disabled, false);
  f.change(amount, "١٬٢٣٤٫٥٦");
  assert.equal(amount.value, "١٬٢٣٤٫٥٦");
  amount.selectionStart = amount.selectionEnd = 2;
  f.change(amount, "١٬x٢٣٤٫٥٦");
  assert.equal(amount.value, "١٬٢٣٤٫٥٦");
  assert.equal(amount.selectionStart, 2);
  f.fire("beforeinput", amount);
  amount.value = "bad";
  f.fire("input", amount, { isComposing: true });
  assert.equal(amount.value, "bad");
  f.fire("compositionend", amount);
  assert.equal(amount.value, "١٬٢٣٤٫٥٦");
  f.change(amount, "-2");
  assert.equal(amount.value, "١٬٢٣٤٫٥٦");
  amount.dataset.entryNumber = "signed-amount";
  f.change(amount, "-2,5");
  assert.equal(amount.value, "-2,5");
});

test("numeric modes, optional groups and swap initialization update the submit", () => {
  const f = fixture();
  const mode = f.field({ name: "amount_mode", value: "decimal" });
  const amount = f.field({ dataset: { entryNumber: "amount", entryNumberMode: "amount_mode" }, attrs: new Set(["data-entry-number-positive"]), value: "1,25" });
  const phone = f.field({ dataset: { entryRequiredGroup: "contact" } });
  const email = f.field({ dataset: { entryRequiredGroup: "contact" } });
  wireEntryValidation(f.doc);
  assert.equal(f.submit.disabled, true);
  f.change(email, "a@example.test");
  assert.equal(f.submit.disabled, false);
  mode.value = "minor";
  f.fire("change", mode);
  assert.equal(f.submit.disabled, true);
  f.change(amount, "125");
  assert.equal(f.submit.disabled, false);
  f.change(amount, "12.5");
  assert.equal(amount.value, "125");
  f.change(amount, "+5");
  assert.equal(amount.value, "+5");
  assert.equal(f.submit.disabled, false);
  f.change(amount, "  +5  ");
  assert.equal(amount.value, "  +5  ");
  assert.equal(f.submit.disabled, false);
  f.change(amount, "\u0085+5\u0085");
  assert.equal(amount.value, "\u0085+5\u0085");
  assert.equal(f.submit.disabled, false);
  f.change(amount, "\ufeff+5\ufeff");
  assert.equal(amount.value, "\u0085+5\u0085");
  amount.value = "\ufeff+5\ufeff";
  f.fire("change", amount);
  assert.equal(f.submit.disabled, true);
  f.change(amount, "9223372036854775807");
  assert.equal(f.submit.disabled, false);
  f.change(amount, "9223372036854775808");
  assert.equal(f.submit.disabled, true);
  f.change(amount, "+0");
  assert.equal(amount.value, "+0");
  assert.equal(f.submit.disabled, true);
  f.change(amount, "+5");
  const integer = f.field({ dataset: { entryNumber: "integer" }, value: "2" });
  wireEntryValidation(f.doc);
  f.change(integer, "+5");
  assert.equal(integer.value, "2");
  const inline = f.field({ value: "2", attrs: new Set(["data-inline-input"]), parent: { dataset: { entryNumber: "minor" } } });
  wireEntryValidation(f.doc);
  f.change(inline, "+5");
  assert.equal(inline.value, "+5");
  const custom = f.field({ dataset: { entryNumber: "float" }, value: "-1.2e+3" });
  wireEntryValidation(f.doc);
  assert.equal(f.submit.disabled, false);
  f.change(custom, "-1.2e+");
  assert.equal(f.submit.disabled, true);
  f.change(custom, "+4e2");
  assert.equal(f.submit.disabled, false);
  phone.hidden = true;
  f.change(email, "");
  assert.equal(f.submit.disabled, true);
});
