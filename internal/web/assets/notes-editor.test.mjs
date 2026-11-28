import test from "node:test";
import assert from "node:assert/strict";
import { wireNotesEditor } from "./notes-editor-core.mjs";

function fixture(value, serialized = value, probeSerialized, inline = false) {
	const handlers = new Map();
	const node = (name) => ({
		hidden: false, dataset: {},
		addEventListener(type, fn) { handlers.set(`${name}:${type}`, fn); },
		focus() { this.focused = true; },
		setAttribute(name, value) { this[name] = value; }, removeAttribute(name) { delete this[name]; }, dispatchEvent() {},
	});
	const form = node("form"), source = Object.assign(node("source"), { value, maxLength: 5000, labels: [{ textContent: "Notes" }] });
	const rich = node("rich"), toolbar = node("toolbar"), warning = node("warning"), cancel = node("cancel"), summary = node("summary"), sourceMode = node("sourceMode");
	const link = node("link"), linkPanel = node("linkPanel"), linkURL = node("linkURL"), linkError = node("linkError"), linkApply = node("linkApply"), linkCancel = node("linkCancel");
	linkPanel.hidden = true;
	toolbar.querySelectorAll = () => [];
	const doc = Object.assign(node("document"), { createElement: () => ({}) });
	const elements = { "[data-notes-form]": form, "[data-notes-source]": source, "[data-notes-rich]": rich, "[data-notes-toolbar]": toolbar, "[data-notes-warning]": warning, "[data-notes-cancel]": cancel, summary, "[data-notes-source-mode]": sourceMode, "[data-notes-link]": link, "[data-notes-link-panel]": linkPanel, "[data-notes-link-url]": linkURL, "[data-notes-link-error]": linkError, "[data-notes-link-apply]": linkApply, "[data-notes-link-cancel]": linkCancel };
	if (inline) { delete elements["[data-notes-form]"]; delete elements["[data-notes-cancel]"]; delete elements.summary; }
	const root = Object.assign(node("root"), { open: !inline, dataset: { notesSaved: value }, ownerDocument: doc, querySelector: (selector) => elements[selector], closest: () => form, hasAttribute: (name) => inline && name === "data-notes-inline", contains: () => false });
	let update, current = serialized, created = 0, linkedURL;
	const create = () => {
		created++;
		const probe = created > 1;
		return { getMarkdown: () => probe && probeSerialized !== undefined ? probeSerialized : current, destroy() {}, focus() {}, subscribe(type, fn) { if (type === "update") update = fn; }, state: { isActive: {}, can: { setLink: () => true } }, commands: { setLink(url) { linkedURL = url; return true; } } };
	};
	wireNotesEditor(root, create);
	return { root, form, source, rich, toolbar, warning, sourceMode, handlers, link, linkPanel, linkURL, linkError, linkedURL: () => linkedURL, created: () => created, change(markdown) { current = markdown; update(); } };
}

function event() { return { prevented: false, preventDefault() { this.prevented = true; }, stopPropagation() {} }; }

test("unsupported or non-round-tripping notes stay editable as exact source", () => {
	for (const [value, serialized] of [["# heading", "# heading"], ["simple text", "simple text\n"]]) {
		const f = fixture(value, serialized);
		assert.equal(f.source.value, value);
		assert.equal(f.source.hidden, false);
		assert.equal(f.warning.hidden, false);
		assert.equal(f.rich.hidden, true);
	}
});

test("reopening canonical lists and links keeps visual editing enabled", () => {
	for (const source of ["* first\n* second", "[site](https://example.com/)"]) {
		const f = fixture(source);
		assert.equal(f.source.hidden, true, source);
		assert.equal(f.rich.hidden, false, source);
		assert.equal(f.warning.hidden, true, source);
	}
});

test("source mode keeps the toolbar and returns to visual editing without losing the draft", () => {
	const f = fixture("hello");
	f.change("**hello**");
	f.handlers.get("sourceMode:click")();
	assert.equal(f.toolbar.hidden, false);
	assert.equal(f.source.hidden, false);
	assert.equal(f.source.value, "**hello**");
	assert.equal(f.link.disabled, true);
	assert.equal(f.sourceMode["aria-pressed"], "true");
	f.handlers.get("sourceMode:click")();
	assert.equal(f.source.hidden, true);
	assert.equal(f.rich.hidden, false);
	assert.equal(f.source.value, "**hello**");
	assert.equal(f.sourceMode["aria-pressed"], "false");
	assert.equal(f.link.disabled, false);
});

test("unsupported source stays intact when visual preview is requested", () => {
	const f = fixture("# heading");
	assert.equal(f.toolbar.hidden, false);
	f.handlers.get("sourceMode:click")();
	assert.equal(f.source.value, "# heading");
	assert.equal(f.source.hidden, false);
	assert.equal(f.warning.hidden, false);
});

test("visual edits sync into the submitted field, and cancel restores saved text", () => {
	const f = fixture("hello");
	assert.equal(f.source.hidden, true);
	f.change("**hello**");
	assert.equal(f.source.value, "**hello**");
	const submit = event();
	f.handlers.get("form:submit")(submit);
	assert.equal(submit.prevented, false);
	f.handlers.get("cancel:click")(event());
	assert.equal(f.source.value, "hello");
	assert.equal(f.root.open, false);
});

test("Escape and an outside click discard edits without submitting", () => {
	for (const trigger of ["escape", "outside"]) {
		const f = fixture("hello");
		f.change("changed");
		const e = event();
		if (trigger === "escape") {
			e.key = "Escape";
			f.handlers.get("root:keydown")(e);
			assert.equal(e.prevented, true);
		} else {
			e.target = {};
			f.handlers.get("document:click")(e);
		}
		assert.equal(f.source.value, "hello");
		assert.equal(f.root.open, false);
	}
});

test("overlong visual edits return to source without submitting or discarding text", () => {
	const f = fixture("hello");
	const text = "x".repeat(5001);
	f.change(text);
	const submit = event();
	f.handlers.get("form:submit")(submit);
	assert.equal(submit.prevented, true);
	assert.equal(f.source.value, text);
	assert.equal(f.source.hidden, false);
	assert.equal(f.warning.hidden, false);
});

test("unstable serialization blocks submission and preserves the draft", () => {
	const f = fixture("hello", "hello", "lost");
	f.change("**hello**");
	const submit = event();
	f.handlers.get("form:submit")(submit);
	assert.equal(submit.prevented, true);
	assert.equal(f.source.value, "**hello**");
	assert.equal(f.source.hidden, false);
	assert.equal(f.warning.hidden, false);
});

test("link control rejects unsafe URLs and applies a valid URL to the selection", () => {
	const f = fixture("hello");
	f.handlers.get("link:click")();
	assert.equal(f.linkPanel.hidden, false);
	f.linkURL.value = "javascript:alert(1)";
	f.handlers.get("linkApply:click")();
	assert.equal(f.linkedURL(), undefined);
	assert.equal(f.linkError.hidden, false);
	f.linkURL.value = "https://example.com/page";
	f.handlers.get("linkApply:click")();
	assert.equal(f.linkedURL(), "https://example.com/page");
	assert.equal(f.linkPanel.hidden, true);
});

test("inline editor preserves the creation draft through outside clicks, Escape and picker submits", () => {
	const f = fixture("hello", "hello", "unstable", true);
	assert.equal(f.source.hidden, true);
	assert.equal(f.source.focused, undefined);
	f.change("**hello**");
	assert.equal(f.source.value, "**hello**");
	assert.equal(f.handlers.has("document:click"), false);
	const escape = event();
	escape.key = "Escape";
	f.handlers.get("root:keydown")(escape);
	assert.equal(escape.prevented, false);
	const picker = { ...event(), submitter: { formNoValidate: true } };
	f.handlers.get("form:submit")(picker);
	assert.equal(picker.prevented, false);
	assert.equal(f.source.value, "**hello**");
	const submit = event();
	f.handlers.get("form:submit")(submit);
	assert.equal(submit.prevented, true);
	assert.equal(f.source.value, "**hello**");
	assert.equal(f.source.hidden, false);
});

test("inline unsupported markdown stays editable without stealing focus", () => {
	const f = fixture("# heading", "# heading", undefined, true);
	assert.equal(f.source.value, "# heading");
	assert.equal(f.source.hidden, false);
	assert.equal(f.source.focused, undefined);
	assert.equal(f.warning.hidden, false);
});
