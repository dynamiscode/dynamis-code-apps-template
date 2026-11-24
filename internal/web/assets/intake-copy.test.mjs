import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const { copyIntakeLink } = createRequire(import.meta.url)("./alpine-components.js");

test("copies an absolute link and selects it if clipboard access fails", async () => {
	const field = { value: "/intake/example", focus() { this.focused = true; }, select() { this.selected = true; } };
	let copied;
	assert.equal(await copyIntakeLink(field, { writeText: async value => { copied = value; } }, "http://localhost:8080"), true);
	assert.equal(copied, "http://localhost:8080/intake/example");
	assert.equal(await copyIntakeLink(field, undefined, "http://localhost:8080"), false);
	assert.equal(field.value, copied);
	assert.equal(field.focused && field.selected, true);
});
