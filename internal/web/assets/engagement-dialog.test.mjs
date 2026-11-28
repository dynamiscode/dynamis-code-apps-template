import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const { wireEngagementDialog } = createRequire(import.meta.url)("./alpine-components.js");

test("engagement dialog opens once and restores focus after closing", () => {
	let shown = 0, closed = 0, focused = 0;
	const cancel = new EventTarget();
	const dialog = Object.assign(new EventTarget(), { dataset: {}, querySelector: () => cancel, showModal: () => shown++, close: () => closed++ });
	const trigger = { focus: () => focused++ };
	const root = { querySelector: (selector) => ({ "[data-engagement-dialog]": dialog, "[data-engagement-create]": trigger })[selector] };
	wireEngagementDialog(root);
	wireEngagementDialog(root);
	cancel.dispatchEvent(new Event("click", { cancelable: true }));
	dialog.dispatchEvent(new Event("close"));
	assert.deepEqual([shown, closed, focused], [1, 1, 1]);
	const childClick = new Event("click");
	Object.defineProperty(childClick, "target", { value: {} });
	dialog.dispatchEvent(childClick);
	assert.equal(closed, 1);
	dialog.dispatchEvent(new Event("click"));
	dialog.dispatchEvent(new Event("close"));
	assert.deepEqual([closed, focused], [2, 2]);
});
