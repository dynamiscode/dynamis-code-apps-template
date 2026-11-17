import assert from "node:assert/strict";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import test from "node:test";

const checker = fileURLToPath(new URL("./check-doc-link-anchors.mjs", import.meta.url));

test("accepts heading, duplicate, and HTML anchors", () => {
  const root = fixture();
  try {
    writeFileSync(path.join(root, "README.md"), [
      "# Home",
      "[heading](page.md#hello-world)",
      "[duplicate](page.md#repeat-1)",
      "[HTML id](page.md#explicit-anchor)",
      "[same page](#home)",
      "[external](https://example.com/page#external)",
    ].join("\n"));
    writeFileSync(path.join(root, "page.md"), [
      "# Home",
      "## Hello, world!",
      "## Repeat",
      "## Repeat",
      '<span id="explicit-anchor"></span>',
    ].join("\n"));

    const result = runChecker(root);
    assert.equal(result.status, 0, result.stderr);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("rejects a missing Markdown anchor", () => {
  const root = fixture();
  try {
    writeFileSync(path.join(root, "README.md"), "[missing](page.md#not-there)\n");
    writeFileSync(path.join(root, "page.md"), "# Existing section\n");

    const result = runChecker(root);
    assert.equal(result.status, 1);
    assert.match(result.stderr, /broken Markdown anchor: README\.md -> page\.md#not-there/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

function fixture() {
  return mkdtempSync(path.join(tmpdir(), "doc-link-anchors-"));
}

function runChecker(root) {
  return spawnSync(process.execPath, [checker, root], { encoding: "utf8" });
}
