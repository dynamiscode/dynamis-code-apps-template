import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const { wireClientDialog } = createRequire(import.meta.url)("./alpine-components.js");

test("client dialog opens from either link and returns focus on close", () => {
	let click, close, backdrop, shown = 0, closed = 0, openerFocused = 0;
	const dialog = { dataset: {}, querySelector: () => null, showModal: () => shown++, close: () => closed++, addEventListener: (type, fn) => { if (type === "close") close = fn; else if (type === "click") backdrop = fn; } };
	const root = { querySelector: () => dialog, addEventListener: (type, fn) => { if (type === "click") click = fn; } };
	wireClientDialog(root);
	wireClientDialog(root);
	const link = { focus: () => openerFocused++, closest: (selector) => selector === "[data-client-create]" ? link : null };
	let prevented = false;
	click({ target: link, preventDefault: () => { prevented = true; } });
	assert.equal(prevented, true);
	assert.equal(shown, 1);
	click({ target: { closest: (selector) => selector === "[data-client-close]" ? {} : null } });
	assert.equal(closed, 1);
	close();
	assert.equal(openerFocused, 1);
	backdrop({ target: {} });
	assert.equal(closed, 1);
	backdrop({ target: dialog });
	assert.equal(closed, 2);
	close();
	assert.equal(openerFocused, 2);
});

test("client dialog protects dirty drafts on close, backdrop and Escape", () => {
	const handlers = {}, rootHandlers = {};
	const form = { dataset: { dirty: "true" } };
	let closed = 0, confirmations = 0, allowed = false;
	const dialog = {
		dataset: { clientDiscardMessage: "Unsaved changes" },
		querySelector: () => form,
		showModal() {},
		close() { closed++; },
		addEventListener(type, fn) { handlers[type] = fn; },
	};
	const root = { querySelector: () => dialog, addEventListener(type, fn) { rootHandlers[type] = fn; } };
	const originalWindow = globalThis.window;
	globalThis.window = { confirm(message) { assert.equal(message, "Unsaved changes"); confirmations++; return allowed; } };
	try {
		wireClientDialog(root);
		rootHandlers.click({ target: { closest: (selector) => selector === "[data-client-close]" ? {} : null } });
		handlers.click({ target: dialog });
		let prevented = false;
		handlers.cancel({ preventDefault() { prevented = true; } });
		assert.equal(prevented, true);
		assert.equal(confirmations, 3);
		assert.equal(closed, 0);
		allowed = true;
		handlers.cancel({ preventDefault() {} });
		assert.equal(closed, 1);
		form.dataset.dirty = "";
		allowed = false;
		handlers.click({ target: dialog });
		assert.equal(closed, 2);
		assert.equal(confirmations, 4);
	} finally {
		if (originalWindow === undefined) delete globalThis.window;
		else globalThis.window = originalWindow;
	}
});

test("reopening a client draft reveals populated advanced fields", () => {
	let click;
	const advanced = { open: false, querySelectorAll: () => [{ value: "draft@example.com" }] };
	const dialog = {
		dataset: {},
		querySelector: (selector) => selector === "details[data-client-advanced]" ? advanced : null,
		showModal() {},
		addEventListener() {},
	};
	const root = { querySelector: () => dialog, addEventListener(type, fn) { if (type === "click") click = fn; } };
	wireClientDialog(root);
	const link = { closest: (selector) => selector === "[data-client-create]" ? link : null };
	click({ target: link, preventDefault() {} });
	assert.equal(advanced.open, true);
});

test("client creation reveals invalid advanced fields before validation", () => {
	let submit;
	const root = { querySelector: () => null, addEventListener(type, fn) { if (type === "submit") submit = fn; } };
	wireClientDialog(root);
	const details = { open: false, querySelector: () => ({}) };
	submit({ target: { closest: () => ({}), querySelector: () => details } });
	assert.equal(details.open, true);
	details.open = false;
	details.querySelector = () => null;
	submit({ target: { closest: () => ({}), querySelector: () => details } });
	assert.equal(details.open, false);
});
