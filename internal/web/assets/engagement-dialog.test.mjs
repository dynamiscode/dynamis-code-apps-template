import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const { wireEngagementDialog } = createRequire(import.meta.url)("./alpine-components.js");

test("engagement dialog opens once and restores focus after closing", () => {
	const handlers = {};
	let shown = 0, closed = 0, focused = 0;
	const cancel = { addEventListener: (type, fn) => { handlers.cancel = fn; } };
	const dialog = { dataset: {}, querySelector: () => cancel, showModal: () => shown++, close: () => closed++, addEventListener: (type, fn) => { handlers[type] = fn; } };
	const trigger = { focus: () => focused++ };
	const root = { querySelector: (selector) => ({ "[data-engagement-dialog]": dialog, "[data-engagement-create]": trigger })[selector] };
	wireEngagementDialog(root);
	wireEngagementDialog(root);
	handlers.cancel({ preventDefault() {} });
	handlers.close();
	assert.deepEqual([shown, closed, focused], [1, 1, 1]);
	handlers.click({ target: {} });
	assert.equal(closed, 1);
	handlers.click({ target: dialog });
	handlers.close();
	assert.deepEqual([closed, focused], [2, 2]);
});
