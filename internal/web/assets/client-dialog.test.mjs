import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const { wireClientDialog } = createRequire(import.meta.url)("./alpine-components.js");

test("client dialog opens from either link and returns focus on close", () => {
	let click, close, backdrop, shown = 0, closed = 0, fieldFocused = 0, openerFocused = 0;
	const field = { focus: () => fieldFocused++ };
	const dialog = { dataset: {}, showModal: () => shown++, close: () => closed++, querySelector: () => field, addEventListener: (type, fn) => { if (type === "close") close = fn; else if (type === "click") backdrop = fn; } };
	const root = { querySelector: () => dialog, addEventListener: (_, fn) => { click = fn; } };
	wireClientDialog(root);
	wireClientDialog(root);
	const link = { focus: () => openerFocused++, closest: (selector) => selector === "[data-client-create]" ? link : null };
	let prevented = false;
	click({ target: link, preventDefault: () => { prevented = true; } });
	assert.equal(prevented, true);
	assert.equal(shown, 1);
	assert.equal(fieldFocused, 1);
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
