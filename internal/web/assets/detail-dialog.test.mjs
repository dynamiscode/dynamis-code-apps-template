import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const { wireDetailDialogs } = createRequire(import.meta.url)("./alpine-components.js");

function fixture({ initiallyOpen = false, dirtyOnLoad = false, supported = true } = {}) {
	const handlers = {};
	const on = (name) => (type, handler) => { (handlers[`${name}:${type}`] ??= []).push(handler); };
	const fire = (name, event) => { for (const handler of handlers[name] ?? []) handler(event); };
	const field = { name: "text", type: "text", value: "saved" };
	const form = { elements: [field] };
	const cancel = { hidden: true, addEventListener: on("cancel") };
	const fallback = { hidden: false };
	let focused = 0, shown = 0, closed = 0, alertFocused = 0;
	const summary = { addEventListener: on("summary"), focus: () => focused++ };
	const dialog = {
		dataset: {}, classList: { add: (name) => assert.equal(name, "modal"), remove: (name) => assert.equal(name, "relative") },
		removeAttribute: (name) => assert.equal(name, "open"),
		querySelector: (selector) => ({ form, "[data-detail-cancel]": cancel, "[data-detail-fallback-cancel]": fallback, '[role="alert"], [aria-invalid="true"]': initiallyOpen ? { focus: () => alertFocused++ } : null })[selector],
		addEventListener: on("dialog"),
		close: () => { closed++; fire("dialog:close"); },
	};
	if (supported) dialog.showModal = () => shown++;
	const details = {
		open: initiallyOpen, dataset: { detailDiscardMessage: "Discard?" },
		hasAttribute: (name) => name === "data-detail-dirty-on-load" && dirtyOnLoad,
		querySelector: (selector) => ({ "[data-detail-dialog]": dialog, summary })[selector],
	};
	return { root: { querySelectorAll: () => [details] }, fire, details, dialog, field, cancel, fallback, counts: () => ({ focused, shown, closed, alertFocused }) };
}

const event = () => ({ prevented: false, preventDefault() { this.prevented = true; } });

test("dialog enhancement preserves fallback when unsupported", () => {
	const f = fixture({ initiallyOpen: true, supported: false });
	wireDetailDialogs(f.root);
	assert.equal(f.details.open, true);
	assert.equal(f.cancel.hidden, true);
	assert.equal(f.fallback.hidden, false);
});

test("dialog opens once, focuses errors, and restores focus on cancel", () => {
	const f = fixture({ initiallyOpen: true });
	wireDetailDialogs(f.root);
	wireDetailDialogs(f.root);
	assert.equal(f.details.open, true);
	assert.equal(f.cancel.hidden, false);
	assert.equal(f.fallback.hidden, true);
	assert.deepEqual(f.counts(), { focused: 0, shown: 1, closed: 0, alertFocused: 1 });
	f.fire("cancel:click", event());
	assert.equal(f.details.open, false);
	assert.deepEqual(f.counts(), { focused: 1, shown: 1, closed: 1, alertFocused: 1 });
	const click = event();
	f.fire("summary:click", click);
	assert.equal(click.prevented, true);
	assert.equal(f.details.open, true);
	assert.equal(f.counts().shown, 2);
});

test("dirty Escape and cancellation require confirmation and keep draft", () => {
	const f = fixture();
	const previous = globalThis.window;
	let confirm = false, prompts = 0;
	globalThis.window = { confirm: (message) => { assert.equal(message, "Discard?"); prompts++; return confirm; } };
	try {
		wireDetailDialogs(f.root);
		f.fire("summary:click", event());
		f.field.value = "draft";
		const escape = event();
		f.fire("dialog:cancel", escape);
		assert.equal(escape.prevented, true);
		assert.equal(f.counts().closed, 0);
		confirm = true;
		f.fire("cancel:click", event());
		assert.equal(f.counts().closed, 1);
		assert.equal(f.field.value, "draft");
		assert.equal(prompts, 2);
	} finally { globalThis.window = previous; }
});

test("server-echoed drafts require confirmation even without further edits", () => {
	const f = fixture({ initiallyOpen: true, dirtyOnLoad: true });
	const previous = globalThis.window;
	let prompts = 0;
	globalThis.window = { confirm: () => { prompts++; return false; } };
	try {
		wireDetailDialogs(f.root);
		f.fire("cancel:click", event());
		assert.equal(prompts, 1);
		assert.equal(f.counts().closed, 0);
	} finally { globalThis.window = previous; }
});
