import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const { wireTransactionsDialog } = createRequire(import.meta.url)("./alpine-components.js");

function fixture(initiallyOpen = false) {
	const handlers = new Map();
	let focused = false;
	const listen = (name) => (type, handler) => {
		const key = `${name}:${type}`;
		handlers.set(key, [...(handlers.get(key) || []), handler]);
	};
	const dialog = {
		dataset: {}, open: initiallyOpen,
		querySelector: () => null,
		removeAttribute: () => { dialog.open = false; },
		showModal: () => { assert.equal(dialog.open, false); dialog.open = true; },
		close: () => { dialog.open = false; handlers.get("dialog:close")?.forEach((handler) => handler()); },
		addEventListener: listen("dialog"),
	};
	const open = { addEventListener: listen("open"), focus: () => { focused = true; } };
	const cancel = { addEventListener: listen("cancel") };
	const root = { querySelector: (selector) => ({ "[data-transactions-dialog]": dialog, "[data-transactions-create]": open, "[data-transactions-cancel]": cancel })[selector] };
	return { root, dialog, handlers, get focused() { return focused; } };
}

function click(handlers, name) {
	let prevented = false;
	for (const handler of handlers.get(`${name}:click`) || []) handler({ preventDefault() { prevented = true; } });
	return prevented;
}

test("transactions dialog opens, cancels, and restores focus after repeated wiring", () => {
	const f = fixture();
	wireTransactionsDialog(f.root);
	wireTransactionsDialog(f.root);
	assert.equal(click(f.handlers, "open"), true);
	assert.equal(f.dialog.open, true);
	assert.equal(click(f.handlers, "cancel"), true);
	assert.equal(f.dialog.open, false);
	assert.equal(f.focused, true);
});

test("backdrop closes the transactions dialog, but clicks inside do not", () => {
	const f = fixture();
	wireTransactionsDialog(f.root);
	click(f.handlers, "open");
	const handler = f.handlers.get("dialog:click")[0];
	handler({ target: {} });
	assert.equal(f.dialog.open, true);
	handler({ target: f.dialog });
	assert.equal(f.dialog.open, false);
	assert.equal(f.focused, true);
});

test("transactions dialog promotes an open server-rendered draft to a modal", () => {
	const f = fixture(true);
	wireTransactionsDialog(f.root);
	assert.equal(f.dialog.open, true);
	f.dialog.close();
	assert.equal(f.focused, true);
});

test("server-rendered validation opens without a trigger and focuses its error", () => {
	const f = fixture(true);
	let focused = false;
	f.dialog.querySelector = (selector) => selector === '[role="alert"].error' ? { focus() { focused = true; } } : null;
	const query = f.root.querySelector;
	f.root.querySelector = (selector) => selector === "[data-transactions-create]" ? null : query(selector);
	wireTransactionsDialog(f.root);
	assert.equal(f.dialog.open, true);
	assert.equal(focused, true);
	assert.equal(click(f.handlers, "cancel"), false);
});

test("server-rendered picker autofocus takes precedence over error focus", () => {
	const f = fixture(true);
	let pickerFocused = false;
	let errorFocused = false;
	const section = { open: false, parentElement: null };
	f.dialog.contains = (element) => element === section;
	f.dialog.querySelector = (selector) => selector === "[autofocus]" ? { closest: () => section, focus() { assert.equal(f.dialog.open, true); assert.equal(section.open, true); pickerFocused = true; } } : { focus() { errorFocused = true; } };
	wireTransactionsDialog(f.root);
	assert.equal(pickerFocused, true);
	assert.equal(errorFocused, false);
});

test("transactions links retain their fallback when modal support is unavailable", () => {
	const f = fixture();
	delete f.dialog.showModal;
	wireTransactionsDialog(f.root);
	assert.equal(click(f.handlers, "open"), false);
	assert.equal(click(f.handlers, "cancel"), false);
});

test("creation pencil summaries follow the draft and reveal invalid controls", () => {
	const f = fixture();
	const handlers = {};
	const input = { value: "", tagName: "INPUT", addEventListener(type, handler) { handlers[type] = handler; } };
	const summary = { textContent: "", dataset: { empty: "Not set" } };
	const field = { dataset: { transactionDraftField: "description" }, open: false, addEventListener() {}, querySelector: (selector) => selector === "[data-transaction-draft-value]" ? summary : null };
	f.root.querySelectorAll = (selector) => selector === "[data-transaction-draft-field]" ? [field] : [];
	f.root.getElementById = () => input;
	wireTransactionsDialog(f.root);
	input.value = "<draft>";
	handlers.input();
	assert.equal(summary.textContent, "<draft>");
	input.value = "";
	handlers.change();
	assert.equal(summary.textContent, "Not set");
	handlers.invalid();
	assert.equal(field.open, true);
});

test("Enter in client search submits the client picker rather than another form action", () => {
	const f = fixture();
	let handler;
	let submitted;
	let prevented = false;
	const button = {};
	const search = { form: { requestSubmit(value) { submitted = value; } }, addEventListener(type, callback) { if (type === "keydown") handler = callback; } };
	const input = { value: "", tagName: "SELECT", addEventListener() {} };
	const summary = { dataset: { empty: "Not set" } };
	const field = { dataset: { transactionDraftField: "transactions-client" }, addEventListener() {}, querySelector: (selector) => ({ "[data-transaction-draft-value]": summary, "#transactions-client-search": search, 'button[name="picker_action"][value="search"]': button })[selector] };
	f.root.querySelectorAll = (selector) => selector === "[data-transaction-draft-field]" ? [field] : [];
	f.root.getElementById = () => input;
	wireTransactionsDialog(f.root);
	handler({ key: "Enter", isComposing: false, preventDefault() { prevented = true; } });
	assert.equal(prevented, true);
	assert.equal(submitted, button);
});
