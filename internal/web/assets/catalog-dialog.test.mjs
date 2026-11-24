import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const { wireCatalogDialog } = createRequire(import.meta.url)("./alpine-components.js");

test("catalog dialog reconnects after a scope swap without duplicate handlers", () => {
	const handlers = {};
	let shown = 0, closed = 0, focused = 0;
	const dialog = { dataset: {}, open: false, showModal: () => shown++, close: () => closed++, addEventListener: (type, fn) => { handlers.dialogClose = fn; } };
	const open = { addEventListener: (type, fn) => { handlers.open = fn; }, focus: () => focused++ };
	const cancel = { addEventListener: (type, fn) => { handlers.cancel = fn; } };
	const root = { querySelector: (selector) => ({ "[data-catalog-dialog]": dialog, "[data-catalog-create]": open, "[data-catalog-cancel]": cancel })[selector] };
	wireCatalogDialog(root);
	wireCatalogDialog(root);
	const event = { preventDefault() {} };
	handlers.open(event);
	handlers.cancel(event);
	handlers.dialogClose();
	assert.deepEqual([shown, closed, focused], [1, 1, 1]);
});
