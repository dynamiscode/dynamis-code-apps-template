import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const { wireCatalogDialog } = createRequire(import.meta.url)("./alpine-components.js");

test("catalog dialog reconnects after a scope swap without duplicate handlers", () => {
	let shown = 0, closed = 0, focused = 0;
	const dialog = Object.assign(new EventTarget(), { dataset: {}, open: false, showModal: () => shown++, close: () => closed++ });
	const open = Object.assign(new EventTarget(), { focus: () => focused++ });
	const cancel = new EventTarget();
	const root = { querySelector: (selector) => ({ "[data-catalog-dialog]": dialog, "[data-catalog-create]": open, "[data-catalog-cancel]": cancel })[selector] };
	wireCatalogDialog(root);
	wireCatalogDialog(root);
	open.dispatchEvent(new Event("click", { cancelable: true }));
	cancel.dispatchEvent(new Event("click", { cancelable: true }));
	dialog.dispatchEvent(new Event("close"));
	assert.deepEqual([shown, closed, focused], [1, 1, 1]);
});
