import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const { wireStageOrder } = createRequire(import.meta.url)("./alpine-components.js");

function fixture() {
	class Element {
		constructor(type, dataset = {}) {
			this.type = type; this.dataset = dataset; this.children = []; this.style = {}; this.listeners = {};
			this.tagName = "LI";
			const classes = new Set();
			this.classList = { add: (name) => classes.add(name), remove: (name) => classes.delete(name), contains: (name) => classes.has(name) };
		}
		get parentElement() { return this.parent; }
		get nextElementSibling() { return this.parent?.children[this.parent.children.indexOf(this) + 1] || null; }
		get isConnected() { return !!this.parent; }
		matches(selector) { return selector === "[data-stage-group]" && this.type === "group"; }
		closest(selector) {
			if (selector === "[data-stage-group]") return this.type === "group" ? this : this.parent?.closest(selector);
			if (selector === "[data-stage-card]") return this.type === "card" ? this : null;
			if (selector === '[data-stage-card][draggable="true"]') return this.type === "card" && !this.archived ? this : null;
			return null;
		}
		contains(element) { return element === this || this.children.some((child) => child.contains(element)); }
		insertBefore(element, before) {
			element.remove(); element.parent = this;
			const index = before ? this.children.indexOf(before) : -1;
			this.children.splice(index < 0 ? this.children.length : index, 0, element);
		}
		appendChild(element) { this.insertBefore(element, null); }
		remove() { if (this.parent) this.parent.children.splice(this.parent.children.indexOf(this), 1); this.parent = null; }
		setAttribute() {}
		removeAttribute() {}
		querySelectorAll() { return []; }
		getBoundingClientRect() { return { top: 0, left: 0, height: 40, width: 100 }; }
		cloneNode() { return new Element("preview"); }
		addEventListener(name, callback) { this.listeners[name] = callback; }
	}
	const board = new Element("board"), body = new Element("body");
	const open = new Element("group", { stageGroup: "open" });
	const won = new Element("group", { stageGroup: "won" });
	const a = new Element("card", { stageId: "a" }), b = new Element("card", { stageId: "b" });
	board.appendChild(open); board.appendChild(won); open.appendChild(a); open.appendChild(b);
	const fields = Object.fromEntries(["stage_id", "kind", "before_id"].map((name) => [name, { value: "" }]));
	const submissions = [];
	const form = { elements: { namedItem: (name) => fields[name] }, requestSubmit() { submissions.push(Object.fromEntries(Object.entries(fields).map(([key, value]) => [key, value.value]))); } };
	board.querySelector = () => form;
	const root = { body, listeners: {}, addEventListener(name, callback) { this.listeners[name] = callback; }, querySelectorAll: () => [board], createElement: () => new Element("gap") };
	wireStageOrder(root);
	const dataTransfer = { setData() {}, setDragImage() {} };
	const fire = (name, target, clientY = 0) => board.listeners[name]({ target, clientX: 0, clientY, dataTransfer, preventDefault() {} });
	return { root, board, open, won, a, b, submissions, fire };
}

test("drop submits the target position and kind; dragend alone cancels", () => {
	const old = globalThis.requestAnimationFrame;
	globalThis.requestAnimationFrame = (callback) => callback();
	try {
		const f = fixture();
		f.fire("dragstart", f.a); f.fire("dragover", f.b, 40); f.fire("dragend", f.a);
		assert.deepEqual(f.open.children, [f.a, f.b]);
		assert.deepEqual(f.submissions, []);
		f.fire("dragstart", f.a); f.fire("dragover", f.b, 40); f.fire("drop", f.b, 40);
		assert.deepEqual(f.submissions, [{ stage_id: "a", kind: "open", before_id: "" }]);
		assert.deepEqual(f.open.children, [f.a, f.b]);
		f.fire("dragstart", f.b); f.fire("dragover", f.won); f.fire("drop", f.won);
		assert.deepEqual(f.submissions[1], { stage_id: "b", kind: "won", before_id: "" });
		assert.deepEqual(f.open.children, [f.a, f.b]);
	} finally { globalThis.requestAnimationFrame = old; }
});

test("unchanged and archived cards never submit", () => {
	const old = globalThis.requestAnimationFrame;
	globalThis.requestAnimationFrame = (callback) => callback();
	try {
		const f = fixture();
		f.fire("dragstart", f.a); f.fire("dragover", f.b); f.fire("drop", f.b);
		f.b.archived = true; f.fire("dragstart", f.b); f.fire("dragover", f.won); f.fire("drop", f.won);
		assert.deepEqual(f.submissions, []);
	} finally { globalThis.requestAnimationFrame = old; }
});

test("escape cancels and dropping before a card identifies that card", () => {
	const old = globalThis.requestAnimationFrame;
	globalThis.requestAnimationFrame = (callback) => callback();
	try {
		const f = fixture();
		f.fire("dragstart", f.b); f.fire("dragover", f.a); f.root.listeners.keydown({ key: "Escape", target: f.root.body });
		assert.deepEqual(f.open.children, [f.a, f.b]);
		f.fire("drop", f.a);
		assert.deepEqual(f.submissions, []);
		f.fire("dragstart", f.b); f.fire("dragover", f.a); f.fire("drop", f.a);
		assert.deepEqual(f.submissions, [{ stage_id: "b", kind: "open", before_id: "a" }]);
	} finally { globalThis.requestAnimationFrame = old; }
});

test("drag guidance follows the current group and clears on drop, cancel, and escape", () => {
	const old = globalThis.requestAnimationFrame;
	globalThis.requestAnimationFrame = (callback) => callback();
	try {
		const f = fixture();
		const active = () => f.board.classList.contains("is-stage-dragging");
		const target = (group) => group.classList.contains("is-stage-drop-target");
		f.fire("dragstart", f.a);
		assert.equal(active(), true);
		f.fire("dragover", f.open);
		assert.equal(target(f.open), true);
		f.fire("dragover", f.won);
		assert.equal(target(f.open), false);
		assert.equal(target(f.won), true);
		f.fire("dragover", f.board);
		assert.equal(target(f.won), false);
		f.fire("dragover", f.won);
		f.board.listeners.dragleave({ relatedTarget: null });
		assert.equal(target(f.won), false);
		f.fire("dragover", f.won);
		f.fire("drop", f.won);
		assert.equal(active(), false);
		assert.equal(target(f.won), false);
		assert.deepEqual(f.submissions, [{ stage_id: "a", kind: "won", before_id: "" }]);
		f.fire("dragstart", f.a); f.fire("dragover", f.won); f.fire("dragend", f.a);
		assert.equal(active(), false);
		assert.equal(target(f.won), false);
		f.fire("dragstart", f.a); f.fire("dragover", f.open); f.root.listeners.keydown({ key: "Escape", target: f.root.body });
		assert.equal(active(), false);
		assert.equal(target(f.open), false);
		f.fire("drop", f.open);
		assert.equal(f.submissions.length, 1);
	} finally { globalThis.requestAnimationFrame = old; }
});
