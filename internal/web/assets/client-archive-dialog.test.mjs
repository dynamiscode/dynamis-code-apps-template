import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const { wireClientArchiveDialog } = createRequire(import.meta.url)("./alpine-components.js");

test("archive cancellation restores focus to its trigger in a closed menu", () => {
	const handlers = {}, controls = {}, rootHandlers = {};
	const menu = { open: false };
	let focused = false;
	const trigger = {
		form: {}, dataset: { clientName: "Ada" },
		closest: (selector) => selector === "details.list-menu" ? menu : null,
		focus() { focused = menu.open; },
	};
	const cancel = { addEventListener: (_, fn) => { controls.cancel = fn; }, focus() {} };
	const confirm = { addEventListener() {} };
	const dialog = {
		dataset: {}, showModal() {}, close() { handlers.close(); },
		addEventListener: (type, fn) => { handlers[type] = fn; },
		querySelector: (selector) => ({ "[data-client-archive-name]": {}, "[data-client-archive-cancel]": cancel, "[data-client-archive-confirm]": confirm })[selector],
	};
	const root = { querySelector: () => dialog, addEventListener: (type, fn) => { rootHandlers[type] = fn; } };
	wireClientArchiveDialog(root);
	rootHandlers.click({ target: { closest: () => trigger }, preventDefault() {}, stopImmediatePropagation() {} });
	controls.cancel();
	assert.equal(menu.open, true);
	assert.equal(focused, true);
});
