import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const { entryPickerSearchable, wireEntryFields } = createRequire(import.meta.url)("./alpine-components.js");

test("search appears only when the full choice list exceeds five", () => {
	for (let count = 0; count <= 10; count++) {
		assert.equal(entryPickerSearchable(0, count), count > 5);
		assert.equal(entryPickerSearchable(String(count), 0), count > 5);
	}
	assert.equal(entryPickerSearchable("100", 5), true);
});

test("remote picker blocks stale choices, preserves reset value, and mirrors accessibility state", async () => {
	const oldFetch = globalThis.fetch;
	const oldObserver = globalThis.MutationObserver;
	let observer, focused;
	globalThis.MutationObserver = class {
		constructor(callback) { observer = callback; }
		observe() {}
	};
	const requests = [];
	globalThis.fetch = (url) => new Promise((resolve) => requests.push({ url, resolve }));
	class Element {
		constructor(tag) { this.tag = tag; this.children = []; this.dataset = {}; this.attrs = {}; this.listeners = {}; this.hidden = false; this.value = ""; }
		append(...nodes) { for (const node of nodes) { this.children.push(node); node.parentElement = this; } }
		replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
		setAttribute(name, value) { this.attrs[name] = String(value); }
		getAttribute(name) { return this.attrs[name] ?? null; }
		removeAttribute(name) { delete this.attrs[name]; }
		hasAttribute(name) { return Object.hasOwn(this.attrs, name); }
		toggleAttribute(name, value) { if (value) this.setAttribute(name, ""); else this.removeAttribute(name); }
		addEventListener(name, fn) { (this.listeners[name] ??= []).push(fn); }
		dispatchEvent(event) { for (const fn of this.listeners[event.type] ?? []) fn(event); }
		click() { this.dispatchEvent(new Event("click")); }
		focus() { focused = this; }
		querySelector(selector) { return selector === "button:not(:disabled)" ? this.children.find((child) => child.tag === "button" && !child.disabled) : null; }
	}
	const doc = {
		location: { href: "https://example.test/form" },
		createElement: (tag) => new Element(tag),
		addEventListener() {}, removeEventListener() {},
	};
	const control = new Element("select");
	control.id = "client";
	control.dataset = { entryRemoteUrl: "/clients", entryRemoteTotal: "30", entryPreviousLabel: "Previous", entryNextLabel: "Next" };
	control.required = true;
	control.attrs = { "aria-invalid": "true", "aria-describedby": "client-error" };
	const form = new Element("form");
	control.form = form;
	control.matches = () => true;
	Object.defineProperty(control, "options", { get() { return this.children; } });
	Object.defineProperty(control, "selectedOptions", { get() { return this.children.filter((option) => option.value === this.value); } });
	const option = (value, text) => { const node = new Element("option"); node.value = value; node.textContent = text; return node; };
	const empty = option("", "No client"), original = option("original", "Original client");
	control.append(empty, original);
	control.value = "original";
	original.defaultSelected = true;
	form.reset = () => {
		control.value = control.options.find((item) => item.defaultSelected)?.value || "";
		form.dispatchEvent(new Event("reset"));
	};
	const label = new Element("label");
	label.htmlFor = "client";
	label.textContent = "Client";
	const fallback = { hidden: false };
	const field = new Element("div");
	field.ownerDocument = doc;
	field.dataset = { entrySearchLabel: "Search clients" };
	field.hasAttribute = (name) => name === "data-entry-picker-enhanced" ? field.dataset.entryPickerEnhanced === "true" : Object.hasOwn(field.attrs, name);
	field.querySelectorAll = () => [control];
	field.querySelector = (selector) => selector === ":scope > label.entry-field-label" ? label : selector === "[data-entry-remote-fallback]" ? fallback : null;
	field.contains = (node) => node === control || field.children.includes(node);
	field.form = null;
	const root = { querySelectorAll: () => [field] };
	const respond = (request, options, total = options.length) => request.resolve({ ok: true, json: async () => ({ options, page: 1, total }) });
	try {
		wireEntryFields(root);
		const trigger = field.children.find((node) => node.className === "entry-picker-trigger");
		const panel = field.children.find((node) => node.className === "entry-picker-panel");
		const search = panel.children.find((node) => node.type === "search");
		const list = panel.children.find((node) => node.getAttribute("role") === "listbox");
		assert.equal(search.maxLength, 200);
		assert.equal(trigger.getAttribute("aria-required"), "true");
		assert.equal(trigger.getAttribute("aria-invalid"), "true");
		assert.equal(trigger.getAttribute("aria-describedby"), "client-error");
		control.removeAttribute("aria-invalid");
		control.removeAttribute("aria-describedby");
		observer();
		assert.equal(trigger.getAttribute("aria-invalid"), null);
		assert.equal(trigger.getAttribute("aria-describedby"), null);
		const labelClick = new Event("click", { cancelable: true });
		label.dispatchEvent(labelClick);
		assert.equal(labelClick.defaultPrevented, true);
		assert.equal(trigger.getAttribute("aria-expanded"), "true");
		assert.equal(panel.hidden, false);
		assert.equal(focused, search);
		label.dispatchEvent(new Event("click", { cancelable: true }));
		assert.equal(trigger.getAttribute("aria-expanded"), "true");
		respond(requests[0], [{ value: "new", label: "New client" }]);
		await new Promise((resolve) => setTimeout(resolve, 0));
		assert.ok(list.children.some((node) => node.textContent === "New client"));
		list.children.find((node) => node.textContent === "New client").dispatchEvent(new Event("click"));
		assert.equal(control.value, "new");
		trigger.dispatchEvent(new Event("click"));
		search.value = "other";
		search.dispatchEvent(new Event("input"));
		assert.equal(list.children.length, 0);
		respond(requests[1], [{ value: "stale", label: "Stale client" }]);
		await new Promise((resolve) => setTimeout(resolve, 275));
		assert.equal(list.children.length, 0);
		respond(requests[2], [{ value: "other", label: "Other client" }]);
		await new Promise((resolve) => setTimeout(resolve, 0));
		assert.deepEqual(list.children.map((node) => node.textContent), ["Other client"]);
		assert.ok(control.options.includes(original));
		assert.ok(control.options.includes(empty));
		form.reset();
		await new Promise((resolve) => setTimeout(resolve, 0));
		assert.equal(control.value, "original");
		assert.equal(trigger.children[0].textContent, "Original client");
		search.value = "abandoned";
		search.dispatchEvent(new Event("input"));
		trigger.dispatchEvent(new Event("click"));
		trigger.dispatchEvent(new Event("click"));
		respond(requests[3], [{ value: "first", label: "First client" }], 50);
		await new Promise((resolve) => setTimeout(resolve, 0));
		const paging = panel.children.find((node) => node.children.some((child) => child.textContent === "Next"));
		const next = paging.children.find((node) => node.textContent === "Next");
		assert.equal(next.disabled, false);
		next.dispatchEvent(new Event("click"));
		assert.equal(new URL(requests[4].url).searchParams.get("page"), "2");
		await new Promise((resolve) => setTimeout(resolve, 275));
		assert.equal(requests.length, 5, "closed search must not restart page 1");
	} finally {
		globalThis.fetch = oldFetch;
		globalThis.MutationObserver = oldObserver;
	}
});
