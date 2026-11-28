import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const { engagementClientPicker } = createRequire(import.meta.url)("./alpine-components.js");

test("selecting a search result changes only the client and returns focus", () => {
	let click, focused = false, prevented = false;
	const changes = [];
	const selected = { value: "first", dispatchEvent: (event) => { changes.push([event.type, event.bubbles]); } };
	const current = { textContent: "First client" };
	const title = { value: "Draft title" };
	const notes = { value: "# Draft\n\nKeep this" };
	const stage = { value: "open" };
	const picker = { open: true, contains: () => true, querySelector: () => ({ focus: () => { focused = true; } }) };
	const root = {
		elements: [selected, title, notes, stage],
		querySelector: (selector) => ({ "[data-client-picker]": picker, "[data-selected-client]": selected, "[data-client-current]": current, '[name="name"]': title, '[name="notes"]': notes, '[name="stage"]': stage })[selector],
		addEventListener: (_, handler) => { click = handler; },
	};
	const component = engagementClientPicker();
	component.$root = root;
	component.init();
	const option = { dataset: { clientId: "second" }, textContent: "Second client" };
	click({ target: { closest: () => option }, preventDefault: () => { prevented = true; } });
	assert.equal(prevented, true);
	assert.equal(selected.value, "second");
	assert.deepEqual(changes, [["change", true]]);
	assert.equal(current.textContent, "Second client");
	assert.equal(picker.open, false);
	assert.equal(focused, true);
	assert.deepEqual(root.elements.map((field) => field.value), ["second", "Draft title", "# Draft\n\nKeep this", "open"]);
});
