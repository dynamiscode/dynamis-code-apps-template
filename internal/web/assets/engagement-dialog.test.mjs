import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const { wireEngagementDialog } = createRequire(import.meta.url)("./alpine-components.js");

test("engagement dialog opens once and restores focus after closing", () => {
	const handlers = {};
	let shown = 0, closed = 0, focused = 0;
	const form = { dataset: {} };
	const cancel = { addEventListener: (type, fn) => { handlers.cancelClick = fn; } };
	const dialog = { dataset: { engagementDiscardMessage: "Unsaved changes" }, querySelector: (selector) => selector === "form" ? form : cancel, showModal: () => shown++, close: () => closed++, addEventListener: (type, fn) => { handlers[type] = fn; } };
	const trigger = { focus: () => focused++ };
	const root = { querySelector: (selector) => ({ "[data-engagement-dialog]": dialog, "[data-engagement-create]": trigger })[selector] };
	wireEngagementDialog(root);
	wireEngagementDialog(root);
	handlers.cancelClick({ preventDefault() {} });
	handlers.close();
	assert.deepEqual([shown, closed, focused], [1, 1, 1]);
	handlers.click({ target: {} });
	assert.equal(closed, 1);
	handlers.click({ target: dialog });
	handlers.close();
	assert.deepEqual([closed, focused], [2, 2]);
});

test("engagement dialog protects dirty draft on cancel, backdrop and Escape", () => {
	const handlers = {};
	const form = { dataset: { dirty: "true" } };
	const cancel = { addEventListener: (_, fn) => { handlers.cancelClick = fn; } };
	let closed = 0, focused = 0, confirmations = 0, allowed = false;
	const dialog = {
		dataset: { engagementDiscardMessage: "Unsaved changes" },
		querySelector: (selector) => selector === "form" ? form : cancel,
		showModal() {}, close() { closed++; handlers.close(); },
		addEventListener: (type, fn) => { handlers[type] = fn; },
	};
	const root = { querySelector: (selector) => selector === "[data-engagement-dialog]" ? dialog : { focus() { focused++; } } };
	const original = globalThis.window;
	globalThis.window = { confirm(message) { assert.equal(message, "Unsaved changes"); confirmations++; return allowed; } };
	try {
		wireEngagementDialog(root);
		handlers.cancelClick({ preventDefault() {} });
		handlers.click({ target: dialog });
		let prevented = false;
		handlers.cancel({ preventDefault() { prevented = true; } });
		assert.equal(prevented, true);
		assert.equal(confirmations, 3);
		assert.equal(closed, 0);
		assert.equal(focused, 0);
		allowed = true;
		handlers.cancel({ preventDefault() {} });
		assert.equal(closed, 1);
		assert.equal(focused, 1);
		assert.equal(form.dataset.dirty, "true");
	} finally {
		if (original === undefined) delete globalThis.window;
		else globalThis.window = original;
	}
});
