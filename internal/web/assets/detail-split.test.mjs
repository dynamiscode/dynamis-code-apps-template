import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const { wireDetailSplit } = createRequire(import.meta.url)("./alpine-components.js");

function fixture(width = 1000, desktop = true) {
	const listeners = {};
	const control = () => ({
		hidden: true, tabIndex: -1, attributes: {}, handlers: {},
		addEventListener(type, handler) { (this.handlers[type] ??= []).push(handler); },
		setAttribute(name, value) { this.attributes[name] = value; },
		focus() { this.focused = true; },
		dispatch(type, values = {}) {
			const event = { preventDefault() { this.prevented = true; }, ...values };
			for (const handler of this.handlers[type] ?? []) handler(event);
			return event;
		},
	});
	const divider = control(), range = control();
	let captured = null;
	divider.setPointerCapture = (id) => { captured = id; };
	divider.hasPointerCapture = (id) => captured === id;
	divider.releasePointerCapture = () => { captured = null; };
	const media = { matches: desktop };
	const split = {
		clientWidth: width, dataset: {}, isConnected: true,
		style: { setProperty(name, value) { this[name] = value; } },
		querySelector: (selector) => ({ "[data-detail-divider]": divider, "[data-detail-range]": range })[selector],
	};
	const root = { querySelectorAll: () => [split] };
	const previous = globalThis.window;
	globalThis.window = {
		matchMedia: () => media,
		addEventListener: (type, handler) => { listeners[type] = handler; },
		removeEventListener: (type) => { delete listeners[type]; },
	};
	return { split, divider, range, media, root, listeners, captured: () => captured, restore: () => { globalThis.window = previous; } };
}

function pointer(type, x, id = 1) {
	return { pointerId: id, clientX: x, isPrimary: true, button: 0 };
}

test("keyboard resizes and clamps both columns", () => {
	const f = fixture();
	try {
		wireDetailSplit(f.root);
		wireDetailSplit(f.root);
		assert.equal(f.split.style["--detail-sidebar-width"], "384px");
		assert.deepEqual(f.divider.attributes, { "aria-valuemin": "320", "aria-valuemax": "704", "aria-valuenow": "600" });
		assert.equal(f.divider.dispatch("keydown", { key: "ArrowLeft" }).prevented, true);
		assert.equal(f.divider.attributes["aria-valuenow"], "590");
		f.divider.dispatch("keydown", { key: "Home" });
		assert.equal(f.split.style["--detail-sidebar-width"], "664px");
		assert.equal(f.divider.attributes["aria-valuenow"], "320");
		f.divider.dispatch("keydown", { key: "End" });
		assert.equal(f.divider.attributes["aria-valuenow"], "704");
		f.divider.dispatch("keydown", { key: "ArrowLeft" });
		assert.equal(f.divider.attributes["aria-valuenow"], "694");
		assert.equal(f.divider.dispatch("keydown", { key: "Tab" }).prevented, undefined);
	} finally { f.restore(); }
});

test("clicking the divider opens a single-pointer resize slider", () => {
	const f = fixture();
	try {
		wireDetailSplit(f.root);
		f.divider.dispatch("pointerdown", pointer("pointerdown", 600));
		f.divider.dispatch("pointerup", pointer("pointerup", 600));
		assert.equal(f.range.hidden, false);
		assert.equal(f.range.focused, true);
		assert.equal(f.range.min, "320");
		assert.equal(f.range.max, "704");
		f.range.value = "400";
		f.range.dispatch("input");
		assert.equal(f.divider.attributes["aria-valuenow"], "400");
		f.range.dispatch("blur");
		assert.equal(f.range.hidden, true);
	} finally { f.restore(); }
});

test("pointer capture supports dragging and cancellation", () => {
	const f = fixture();
	try {
		wireDetailSplit(f.root);
		f.divider.dispatch("pointerdown", pointer("pointerdown", 600));
		assert.equal(f.captured(), 1);
		f.divider.dispatch("pointermove", pointer("pointermove", 1200));
		assert.equal(f.divider.attributes["aria-valuenow"], "704");
		f.divider.dispatch("pointermove", pointer("pointermove", 0, 2));
		assert.equal(f.divider.attributes["aria-valuenow"], "704");
		f.divider.dispatch("pointercancel", pointer("pointercancel", 1200));
		assert.equal(f.captured(), null);
		f.divider.dispatch("pointermove", pointer("pointermove", 0));
		assert.equal(f.divider.attributes["aria-valuenow"], "704");
		f.divider.dispatch("pointerdown", pointer("pointerdown", 700));
		f.divider.dispatch("pointermove", pointer("pointermove", 0));
		assert.equal(f.divider.attributes["aria-valuenow"], "320");
		f.divider.dispatch("pointerup", pointer("pointerup", 0));
		assert.equal(f.captured(), null);
	} finally { f.restore(); }
});

test("mobile hides controls and resize reclamps the preserved sidebar", () => {
	const f = fixture(1000, false);
	try {
		wireDetailSplit(f.root);
		assert.equal(f.divider.hidden, true);
		assert.equal(f.divider.tabIndex, -1);
		f.media.matches = true;
		f.listeners.resize();
		assert.equal(f.divider.hidden, false);
		assert.equal(f.divider.tabIndex, 0);
		f.divider.dispatch("keydown", { key: "Home" });
		f.split.clientWidth = 800;
		f.listeners.resize();
		assert.equal(f.split.style["--detail-sidebar-width"], "464px");
		assert.equal(f.divider.attributes["aria-valuenow"], "320");
		f.divider.dispatch("pointerdown", pointer("pointerdown", 320));
		f.media.matches = false;
		f.listeners.resize();
		assert.equal(f.captured(), null);
		assert.equal(f.divider.hidden, true);
		assert.equal(f.divider.tabIndex, -1);
		f.media.matches = true;
		f.listeners.resize();
		assert.equal(f.divider.attributes["aria-valuenow"], "320");
	} finally { f.restore(); }
});
