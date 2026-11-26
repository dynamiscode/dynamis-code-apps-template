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
	let focused = false;
	f.dialog.querySelector = (selector) => selector === "[autofocus]" ? {} : { focus() { focused = true; } };
	wireTransactionsDialog(f.root);
	assert.equal(focused, false);
});

test("transactions links retain their fallback when modal support is unavailable", () => {
	const f = fixture();
	delete f.dialog.showModal;
	wireTransactionsDialog(f.root);
	assert.equal(click(f.handlers, "open"), false);
	assert.equal(click(f.handlers, "cancel"), false);
});
