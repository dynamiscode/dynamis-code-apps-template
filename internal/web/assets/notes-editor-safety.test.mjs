import test from "node:test";
import assert from "node:assert/strict";
import { safeLinkURL, safeVisualSource } from "./notes-editor-safety.mjs";

test("visual editing accepts common text, paragraphs, simple lists and links", () => {
	for (const source of ["", "Hello world!", "Price: $12.50 (paid); thanks?", "First line.\n\nSecond line.", "**Important** note", "*café*", "- first\n- second", "* first\n* second", "1. first\n2. second", "Visit [our site](https://example.com/path?q=1).", "* [site](https://example.com/)"]) {
		assert.equal(safeVisualSource(source), true, source);
	}
});

test("unsupported Markdown remains in source mode", () => {
	for (const source of ["# Heading", "<b>HTML</b>", "~~strike~~", "`code`", "a_b", "a\\*b", "![image](https://example.com/image)", "[bad](javascript:alert)", "- top\n  - nested", "a | b\n--- | ---", "> quote"]) {
		assert.equal(safeVisualSource(source), false, source);
	}
});

test("link URLs exclude scripts, credentials, whitespace and relative paths", () => {
	for (const url of ["https://example.com/path", "http://example.com/"]) assert.equal(safeLinkURL(url), true, url);
	for (const url of ["", "javascript:alert(1)", "data:text/html,hi", "/relative", "https://user:password@example.com", "https://example.com/a b", "https://example.com\\@evil.test"]) assert.equal(safeLinkURL(url), false, url);
});
