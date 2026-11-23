import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const { wireTimelineDialog } = require("./alpine-components.js");

function fixture(invalid = false, supported = true) {
	const handlers = {};
	let shown = 0;
	let closed = 0;
	let focused = false;
	let removed = false;
	const close = { hidden: true, addEventListener: (event, fn) => { handlers.close = fn; } };
	const composer = { querySelector: () => close, hasAttribute: () => invalid };
	const dialog = { showModal: supported ? () => { shown++; } : undefined, close: () => { closed++; }, addEventListener: (event, fn) => { handlers.dialogClose = fn; } };
	const open = { hidden: true, addEventListener: (event, fn) => { handlers.open = fn; }, focus: () => { focused = true; } };
	let moved;
	const content = { append: (element) => { moved = element; } };
	const summary = { remove: () => { removed = true; } };
	const elements = { "[data-timeline-composer]": composer, "[data-timeline-dialog]": dialog, "[data-timeline-open]": open, "[data-timeline-modal-content]": content, "[data-timeline-error-summary]": summary };
	wireTimelineDialog({ querySelector: (selector) => elements[selector] });
	return { handlers, composer, open, close, get moved() { return moved; }, get shown() { return shown; }, get closed() { return closed; }, get focused() { return focused; }, get removed() { return removed; } };
}

test("dialog enhancement opens and closes the existing form", () => {
	const f = fixture();
	assert.equal(f.moved, f.composer);
	assert.equal(f.open.hidden, false);
	assert.equal(f.close.hidden, false);
	f.handlers.open();
	assert.equal(f.shown, 1);
	f.handlers.close();
	assert.equal(f.closed, 1);
});

test("rejected submission reopens dialog and unsupported browsers keep form", () => {
	const rejected = fixture(true);
	assert.equal(rejected.shown, 1);
	assert.equal(rejected.removed, true);
	rejected.handlers.dialogClose();
	assert.equal(rejected.focused, true);
	const f = fixture(true, false);
	assert.equal(f.removed, false);
	assert.equal(f.moved, undefined);
	assert.equal(f.open.hidden, true);
	assert.equal(f.close.hidden, true);
});
