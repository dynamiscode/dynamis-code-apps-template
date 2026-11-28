import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const { wireClientDeleteDialog } = createRequire(import.meta.url)("./alpine-components.js");

test("client deletion confirms the selected form and cancel never submits", () => {
	const handlers = {};
	const on = (name) => (_, fn) => { (handlers[name] ??= []).push(fn); };
	const fire = (name, event) => { for (const fn of handlers[name] ?? []) fn(event); };
	const warning = {};
	const names = { children: [], replaceChildren() { this.children = []; }, append(item) { this.children.push(item); } };
	const cancel = { addEventListener: on("cancel"), focus() {} };
	const confirm = { addEventListener: on("confirm") };
	let shown = 0, closed = 0, focused = 0, submitted = 0;
	const dialog = { dataset: {}, showModal: () => shown++, close: () => { closed++; fire("close"); }, addEventListener: on("close"), querySelector: (selector) => ({ "[data-client-delete-warning]": warning, "[data-client-delete-names]": names, "[data-client-delete-cancel]": cancel, "[data-client-delete-confirm]": confirm })[selector] };
	const root = { querySelector: () => dialog, addEventListener: on("click"), createElement: () => ({}) };
	wireClientDeleteDialog(root);
	wireClientDeleteDialog(root);
	const form = { dataset: {}, elements: [], append(input) { this.confirmation = input; }, requestSubmit(button) { submitted++; assert.equal(button, trigger); } };
	const trigger = { form, dataset: { confirm: "Cannot be undone", clientName: "Ada" }, hasAttribute: () => false, focus: () => focused++ };
	const event = { target: { closest: () => trigger }, preventDefault() {}, stopImmediatePropagation() {} };
	fire("click", event);
	assert.equal(shown, 1);
	assert.equal(warning.textContent, "Cannot be undone");
	assert.equal(names.children[0].textContent, "Ada");
	fire("cancel");
	assert.deepEqual([closed, focused, submitted], [1, 1, 0]);
	fire("click", event);
	fire("confirm");
	assert.deepEqual([closed, focused, submitted], [2, 2, 1]);
	assert.equal(form.confirmation.name, "confirm_delete");
	assert.equal(form.confirmation.value, "yes");
});

test("bulk confirmation names only the selected clients", () => {
	const handlers = {};
	const names = { children: [], replaceChildren() { this.children = []; }, append(item) { this.children.push(item); } };
	const cancel = { addEventListener() {}, focus() {} };
	const confirm = { addEventListener() {} };
	const dialog = { dataset: {}, showModal() {}, addEventListener() {}, querySelector: (selector) => ({ "[data-client-delete-warning]": {}, "[data-client-delete-names]": names, "[data-client-delete-cancel]": cancel, "[data-client-delete-confirm]": confirm })[selector] };
	const root = { querySelector: () => dialog, addEventListener: (_, fn) => { handlers.click = fn; }, createElement: () => ({}) };
	wireClientDeleteDialog(root);
	const row = (value, label, checked) => ({ name: "client_id", value, checked, getAttribute: () => label });
	const form = { elements: [row("1", "Select Ada", true), row("1", "Select Ada", true), row("2", "Select Bea", false), row("3", "Select Cy", true)] };
	const trigger = { form, dataset: { confirm: "Cannot be undone" }, hasAttribute: () => true };
	handlers.click({ target: { closest: () => trigger }, preventDefault() {}, stopImmediatePropagation() {} });
	assert.deepEqual(names.children.map((item) => item.textContent), ["Select Ada", "Select Cy"]);
});
