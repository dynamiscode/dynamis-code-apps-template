import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const { wireClientFieldDialog } = createRequire(import.meta.url)("./alpine-components.js");

function fixture(error = false) {
	const handlers = {}, openHandlers = {}, closeHandlers = {};
	const form = { dataset: {} };
	let shown = 0, closed = 0, focused = 0, errorFocused = 0;
	const open = { hidden: true, addEventListener: (type, fn) => { openHandlers[type] = fn; }, focus: () => focused++ };
	const close = { addEventListener: (type, fn) => { closeHandlers[type] = fn; } };
	const alert = { focus: () => errorFocused++ };
	const dialog = {
		dataset: { clientFieldDiscardMessage: "Unsaved changes" },
		hasAttribute: () => error,
		querySelector: (selector) => ({ "[data-client-field-close]": close, form, '[role="alert"], [aria-invalid="true"]': alert })[selector],
		addEventListener: (type, fn) => { handlers[type] = fn; },
		showModal: () => shown++,
		close: () => { closed++; handlers.close(); },
	};
	const root = { querySelector: (selector) => selector === "[data-client-field-dialog]" ? dialog : open };
	return { root, dialog, open, form, handlers, openHandlers, closeHandlers, counts: () => ({ shown, closed, focused, errorFocused }) };
}

test("field dialog opens, keeps draft on dismissal and returns focus", () => {
	const f = fixture();
	wireClientFieldDialog(f.root);
	wireClientFieldDialog(f.root);
	assert.equal(f.open.hidden, false);
	f.openHandlers.click({ preventDefault() {} });
	assert.equal(f.counts().shown, 1);
	f.closeHandlers.click({ preventDefault() {} });
	assert.deepEqual(f.counts(), { shown: 1, closed: 1, focused: 1, errorFocused: 0 });
	f.openHandlers.click({ preventDefault() {} });
	f.handlers.click({ target: f.dialog });
	assert.equal(f.counts().closed, 2);
});

test("field dialog protects dirty draft on cancel, backdrop and Escape", () => {
	const f = fixture();
	f.form.dataset.dirty = "true";
	const original = globalThis.window;
	let allowed = false, confirmations = 0;
	globalThis.window = { confirm(message) { assert.equal(message, "Unsaved changes"); confirmations++; return allowed; } };
	try {
		wireClientFieldDialog(f.root);
		f.closeHandlers.click({ preventDefault() {} });
		f.handlers.click({ target: f.dialog });
		let prevented = false;
		f.handlers.cancel({ preventDefault() { prevented = true; } });
		assert.equal(prevented, true);
		assert.equal(confirmations, 3);
		assert.equal(f.counts().closed, 0);
		allowed = true;
		f.handlers.cancel({ preventDefault() {} });
		assert.equal(f.counts().closed, 1);
		assert.equal(f.form.dataset.dirty, "true");
	} finally {
		if (original === undefined) delete globalThis.window;
		else globalThis.window = original;
	}
});

test("field creation error opens dialog and focuses error", () => {
	const f = fixture(true);
	wireClientFieldDialog(f.root);
	assert.deepEqual(f.counts(), { shown: 1, closed: 0, focused: 0, errorFocused: 1 });
});
