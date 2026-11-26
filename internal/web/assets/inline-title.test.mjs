import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const { wireInlineTitles } = createRequire(import.meta.url)("./alpine-components.js");

function fixture({ initiallyOpen = false, rejected = false, explicitRetry = false, multiline = false } = {}) {
	const handlers = {};
	const on = (name) => (type, handler) => { (handlers[`${name}:${type}`] ??= []).push(handler); };
	const counts = { focused: 0, selected: 0, summaryFocused: 0, submitted: 0, inputEvents: 0 };
	const input = {
		value: rejected ? "invalid draft" : "saved",
		hasAttribute: (name) => name === "data-inline-multiline" && multiline,
		focus: () => counts.focused++, select: () => counts.selected++,
		dispatchEvent: (event) => { assert.equal(event.type, "input"); assert.equal(event.bubbles, true); counts.inputEvents++; },
	};
	const pending = { hidden: true, textContent: "" };
	const summary = { addEventListener: on("summary"), focus: () => counts.summaryFocused++, querySelector: () => pending };
	const cancel = { href: "/engagement", addEventListener: on("cancel") };
	const form = { dataset: {}, querySelector: () => null, requestSubmit: () => counts.submitted++ };
	const details = {
		open: initiallyOpen, dataset: { inlineSaved: input.value, inlineDiscardMessage: "Discard?" },
		ownerDocument: { addEventListener: on("document"), querySelector: () => ({ getAttribute: () => "Unsaved changes" }) },
		contains: (target) => target === input || target === summary || target === cancel,
		hasAttribute: (name) => (name === "data-inline-dirty-on-load" && rejected) || (name === "data-inline-explicit-retry" && explicitRetry),
		removeAttribute: (name) => { if (name === "data-inline-dirty-on-load") rejected = false; },
		querySelector: (selector) => ({ summary, "[data-inline-input]": input, form, "[data-inline-cancel]": cancel })[selector],
		addEventListener: on("details"),
	};
	const root = { querySelectorAll: (selector) => selector === "[data-inline-title]" ? [details] : [] };
	const emit = (name, event = {}) => { for (const handler of handlers[name] ?? []) handler(event); };
	return { root, details, input, form, counts, emit, handlers, pending };
}

function event(properties = {}) {
	return { prevented: false, stopped: false, preventDefault() { this.prevented = true; }, stopPropagation() { this.stopped = true; }, ...properties };
}

test("user opening selects title; initial rejected opening preserves focus and draft", () => {
	const initial = fixture({ initiallyOpen: true, rejected: true });
	wireInlineTitles(initial.root);
	initial.emit("details:toggle");
	assert.equal(initial.input.value, "invalid draft");
	assert.equal(initial.counts.focused, 0);
	const f = fixture();
	wireInlineTitles(f.root);
	wireInlineTitles(f.root);
	assert.equal(f.handlers["summary:click"].length, 1);
	const click = event();
	f.emit("summary:click", click);
	assert.equal(click.prevented, false);
	f.details.open = true;
	f.emit("details:toggle");
	assert.equal(f.counts.focused, 1);
	assert.equal(f.counts.selected, 1);
});

test("Escape and cancel restore the original value without a prompt", () => {
	const f = fixture({ initiallyOpen: true });
	wireInlineTitles(f.root);
	f.input.value = "edited";
	const escape = event({ key: "Escape" });
	f.emit("details:keydown", escape);
	assert.equal(escape.prevented, true);
	assert.equal(escape.stopped, true);
	assert.equal(f.details.open, false);
	assert.equal(f.input.value, "saved");
	assert.equal(f.counts.summaryFocused, 1);
	assert.equal(f.counts.inputEvents, 1);
	assert.equal(f.form.dataset.dirty, "");
	f.details.open = true;
	f.input.value = "another edit";
	const cancel = event();
	f.emit("cancel:click", cancel);
	assert.equal(cancel.prevented, true);
	assert.equal(f.details.open, false);
	assert.equal(f.input.value, "saved");
});

test("clicking outside closes without losing the draft or stealing focus", () => {
	const f = fixture({ initiallyOpen: true });
	wireInlineTitles(f.root);
	f.input.value = "edited";
	f.emit("document:click", event({ target: f.input }));
	assert.equal(f.details.open, true);
	assert.equal(f.input.value, "edited");
	f.emit("document:click", event({ target: {} }));
	assert.equal(f.details.open, false);
	assert.equal(f.input.value, "edited");
	assert.equal(f.pending.hidden, false);
	assert.equal(f.pending.textContent, "Unsaved changes");
	assert.equal(f.counts.summaryFocused, 0);
});

test("outside click retains a rejected draft without navigating", () => {
	const f = fixture({ initiallyOpen: true, rejected: true });
	wireInlineTitles(f.root);
	f.input.value = "rejected draft";
	f.emit("document:click", event({ target: {} }));
	assert.equal(f.details.open, false);
	assert.equal(f.input.value, "rejected draft");
	assert.equal(f.pending.hidden, false);
	assert.notEqual(f.form.dataset.submitting, "true");
});

test("multiple independent editors restore only the cancelled field", () => {
	const first = fixture();
	const second = fixture();
	const root = { querySelectorAll: () => [first.details, second.details] };
	wireInlineTitles(root);
	for (const f of [first, second]) {
		f.emit("summary:click", event());
		f.details.open = true;
		f.emit("details:toggle");
		f.input.value = "edited";
	}
	assert.equal(first.counts.selected, 1);
	assert.equal(second.counts.selected, 1);
	second.form.dataset.dirty = "true";
	first.emit("cancel:click", event());
	assert.equal(first.input.value, "saved");
	assert.equal(first.details.open, false);
	assert.equal(second.input.value, "edited");
	assert.equal(second.details.open, true);
	assert.equal(second.form.dataset.dirty, "true");
	second.emit("details:keydown", event({ key: "Enter", target: second.input }));
	assert.equal(second.counts.submitted, 1);
	assert.equal(first.counts.submitted, 0);
});

test("cancelling a rejected draft returns to the saved title without a prompt", () => {
	const previous = globalThis.window;
	let destination;
	globalThis.window = { location: { assign: (url) => { destination = url; } } };
	try {
		const f = fixture({ initiallyOpen: true, rejected: true });
		wireInlineTitles(f.root);
		f.emit("cancel:click", event());
		assert.equal(destination, "/engagement");
		assert.equal(f.form.dataset.submitting, "true");
	} finally { globalThis.window = previous; }
});

test("Enter submits natively; composition and Shift+Enter remain untouched", () => {
	const f = fixture({ initiallyOpen: true });
	wireInlineTitles(f.root);
	const enter = event({ key: "Enter", target: f.input });
	f.emit("details:keydown", enter);
	assert.equal(enter.prevented, true);
	assert.equal(f.counts.submitted, 1);
	for (const properties of [{ shiftKey: true }, { isComposing: true }, { target: {} }]) {
		const other = event({ key: "Enter", target: f.input, ...properties });
		f.emit("details:keydown", other);
		assert.equal(other.prevented, false);
	}
	assert.equal(f.counts.submitted, 1);
});

test("multiline Enter inserts a newline; Ctrl or Meta Enter submits", () => {
	const f = fixture({ initiallyOpen: true, multiline: true });
	wireInlineTitles(f.root);
	for (const properties of [{}, { shiftKey: true }, { ctrlKey: true, isComposing: true }]) {
		const key = event({ key: "Enter", target: f.input, ...properties });
		f.emit("details:keydown", key);
		assert.equal(key.prevented, false);
	}
	assert.equal(f.counts.submitted, 0);
	for (const properties of [{ ctrlKey: true }, { metaKey: true }]) {
		const key = event({ key: "Enter", target: f.input, ...properties });
		f.emit("details:keydown", key);
		assert.equal(key.prevented, true);
	}
	assert.equal(f.counts.submitted, 2);
});

test("multiline conflict requires explicit retry", () => {
	const f = fixture({ initiallyOpen: true, explicitRetry: true, multiline: true });
	wireInlineTitles(f.root);
	const key = event({ key: "Enter", target: f.input, ctrlKey: true });
	f.emit("details:keydown", key);
	assert.equal(key.prevented, true);
	assert.equal(f.counts.submitted, 0);
});

test("multiline Escape discards edits and outside click retains them", () => {
	for (const action of ["escape", "outside"]) {
		const f = fixture({ initiallyOpen: true, multiline: true });
		wireInlineTitles(f.root);
		f.input.value = "unsaved\nnotes";
		if (action === "escape") {
			f.emit("details:keydown", event({ key: "Escape", target: f.input }));
		} else {
			f.emit("document:click", event({ target: {} }));
		}
		assert.equal(f.input.value, action === "escape" ? "saved" : "unsaved\nnotes");
		assert.equal(f.details.open, false);
	}
});

test("conflict retry requires explicit button activation", () => {
	const f = fixture({ initiallyOpen: true, explicitRetry: true });
	wireInlineTitles(f.root);
	const enter = event({ key: "Enter", target: f.input });
	f.emit("details:keydown", enter);
	assert.equal(enter.prevented, true);
	assert.equal(f.counts.submitted, 0);
});

test("Enter requests confirmation before moving a booking", () => {
	const f = fixture();
	let confirmations = 0;
	f.form.querySelector = () => ({ click: () => confirmations++ });
	wireInlineTitles(f.root);
	f.emit("details:keydown", event({ key: "Enter", target: f.input }));
	assert.equal(confirmations, 1);
	assert.equal(f.counts.submitted, 0);
});
