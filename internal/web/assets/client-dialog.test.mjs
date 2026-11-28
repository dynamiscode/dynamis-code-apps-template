import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const { wireClientDialog } = createRequire(import.meta.url)("./alpine-components.js");

test("client dialog opens from either link and returns focus on close", () => {
	let shown = 0, closed = 0, fieldFocused = 0, openerFocused = 0;
	const handlers = {};
	const on = (name) => (type, fn) => { (handlers[`${name}:${type}`] ??= []).push(fn); };
	const fire = (name, event) => { for (const fn of handlers[name] ?? []) fn(event); };
	const field = { focus: () => fieldFocused++ };
	const dialog = { dataset: {}, showModal: () => shown++, close: () => closed++, querySelector: () => field, addEventListener: on("dialog") };
	const root = { querySelector: () => dialog, addEventListener: on("root") };
	wireClientDialog(root);
	wireClientDialog(root);
	const link = { focus: () => openerFocused++, closest: (selector) => selector === "[data-client-create]" ? link : null };
	let prevented = false;
	fire("root:click", { target: link, preventDefault: () => { prevented = true; } });
	assert.equal(prevented, true);
	assert.equal(shown, 1);
	assert.equal(fieldFocused, 1);
	fire("root:click", { target: { closest: (selector) => selector === "[data-client-close]" ? {} : null } });
	assert.equal(closed, 1);
	fire("dialog:close");
	assert.equal(openerFocused, 1);
	fire("dialog:click", { target: {} });
	assert.equal(closed, 1);
	fire("dialog:click", { target: dialog });
	assert.equal(closed, 2);
	fire("dialog:close");
	assert.equal(openerFocused, 2);
});
