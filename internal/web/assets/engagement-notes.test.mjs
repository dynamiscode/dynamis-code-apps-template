import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const { engagementNotesEditor } = createRequire(import.meta.url)("./alpine-components.js");

function fixture() {
	const node = () => ({
		hidden: false, attrs: {}, events: {}, textContent: "", tabIndex: 0,
		classList: { add() {}, remove() {} },
		addEventListener(name, fn) { (this.events[name] ??= []).push(fn); },
		setAttribute(name, value) { this.attrs[name] = value; },
		removeAttribute(name) { delete this.attrs[name]; },
		emit(name, event) { return (this.events[name] ?? []).map((fn) => fn(event)); },
		click() { return Promise.all(this.emit("click")); },
		focus() { this.focused = true; },
	});
	const textarea = Object.assign(node(), {
		value: "Hello world", selectionStart: 6, selectionEnd: 11, maxLength: 5000,
		setRangeText(text, start, end) { this.value = this.value.slice(0, start) + text + this.value.slice(end); },
		setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; },
		dispatchEvent(event) { this.emit(event.type, event); },
	});
	const controls = node(), tools = node(), writePanel = node(), output = node(), write = node(), preview = node(), count = node();
	controls.hidden = true;
	output.hidden = true;
	count.hidden = true;
	const form = { querySelector: () => ({ value: "csrf-token" }) };
	const root = {
		dataset: { countLabel: "Characters", previewUrl: "/preview", previewLoading: "Loading", previewFailed: "Failed", previewEmpty: "Empty" },
		querySelector: (selector) => ({
			textarea, "[data-notes-controls]": controls, "[data-notes-tools]": tools,
			"[data-notes-write-panel]": writePanel, "[data-notes-output]": output,
			"[data-notes-write]": write, "[data-notes-preview]": preview, "[data-notes-count]": count,
		})[selector],
		closest: () => form,
	};
	const editor = engagementNotesEditor();
	editor.$root = root;
	editor.init();
	return { textarea, controls, tools, writePanel, output, write, preview, count };
}

test("formatting keeps selection and updates character count", () => {
	const { textarea, controls, tools, count } = fixture();
	assert.equal(controls.hidden, false);
	assert.equal(count.textContent, "Characters: 11 / 5000");
	tools.emit("click", { target: { closest: () => ({ dataset: { markdown: "bold" } }) } });
	assert.equal(textarea.value, "Hello **world**");
	assert.equal(textarea.value.slice(textarea.selectionStart, textarea.selectionEnd), "world");
	assert.equal(textarea.focused, true);
	assert.equal(count.textContent, "Characters: 15 / 5000");
});

test("list formatting prefixes each selected line", () => {
	const { textarea, tools } = fixture();
	textarea.value = "first\nsecond";
	textarea.selectionStart = 2;
	textarea.selectionEnd = textarea.value.length;
	tools.emit("click", { target: { closest: () => ({ dataset: { markdown: "list" } }) } });
	assert.equal(textarea.value, "- first\n- second");
	assert.equal(textarea.value.slice(textarea.selectionStart, textarea.selectionEnd), "rst\n- second");
});

test("preview tabs preserve the draft and use server-rendered HTML", async (t) => {
	const previousFetch = globalThis.fetch;
	t.after(() => { globalThis.fetch = previousFetch; });
	globalThis.fetch = async (url, options) => {
		assert.equal(url, "/preview");
		assert.equal(options.body.get("csrf"), "csrf-token");
		assert.equal(options.body.get("notes"), "Hello world");
		return { ok: true, headers: new Headers({ "content-type": "text/html; charset=utf-8" }), text: async () => "<p>Hello world</p>" };
	};
	const { textarea, tools, writePanel, output, write, preview } = fixture();
	await preview.click();
	assert.equal(writePanel.hidden, true);
	assert.equal(tools.hidden, true);
	assert.equal(output.hidden, false);
	assert.equal(output.innerHTML, "<p>Hello world</p>");
	assert.equal(preview.attrs["aria-selected"], "true");
	assert.equal(write.tabIndex, -1);
	write.click();
	assert.equal(textarea.value, "Hello world");
	assert.equal(writePanel.hidden, false);
	assert.equal(output.hidden, true);
	assert.equal(write.attrs["aria-selected"], "true");
});

test("late preview cannot replace the draft after returning to write", async (t) => {
	const previousFetch = globalThis.fetch;
	t.after(() => { globalThis.fetch = previousFetch; });
	let resolve;
	globalThis.fetch = () => new Promise((done) => { resolve = done; });
	const { textarea, output, write, preview } = fixture();
	const pending = preview.click();
	let prevented = false;
	preview.emit("keydown", { key: "ArrowLeft", preventDefault() { prevented = true; } });
	resolve({ ok: true, headers: new Headers({ "content-type": "text/html" }), text: async () => "<p>Old</p>" });
	await pending;
	assert.equal(prevented, true);
	assert.equal(write.focused, true);
	assert.equal(output.hidden, true);
	assert.notEqual(output.innerHTML, "<p>Old</p>");
	assert.equal(textarea.value, "Hello world");
});

test("failed preview reports the error without losing text", async (t) => {
	const previousFetch = globalThis.fetch;
	t.after(() => { globalThis.fetch = previousFetch; });
	globalThis.fetch = async () => ({ ok: false, headers: new Headers() });
	const { textarea, output, preview, write } = fixture();
	await preview.click();
	assert.equal(output.textContent, "Failed");
	assert.equal(output.attrs["aria-busy"], undefined);
	write.click();
	assert.equal(textarea.value, "Hello world");
});
