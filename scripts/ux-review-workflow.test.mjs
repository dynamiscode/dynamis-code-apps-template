import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

test("body edits validate UX separately without replacing required CI", () => {
  const ci = readFileSync(new URL("../.github/workflows/ci.yml", import.meta.url), "utf8");
  const edits = readFileSync(new URL("../.github/workflows/ux-review.yml", import.meta.url), "utf8");
  assert.match(ci, /types: \[opened, synchronize, reopened\]/);
  assert.match(edits, /types: \[opened, synchronize, reopened, edited\]/);
  assert.match(ci, /accessibility:[\s\S]*?node scripts\/check-pr-ux-review\.mjs "\$GITHUB_EVENT_PATH"/);
  assert.match(edits, /node scripts\/check-pr-ux-review\.mjs "\$GITHUB_EVENT_PATH"/);
  const uxFilter = /grep -E '(.*?)' >\/dev\/null; then\n\s+ux=true/;
  const ciFilter = ci.match(uxFilter);
  const editFilter = edits.match(uxFilter);
  assert.ok(ciFilter);
  assert.ok(editFilter);
  assert.equal(editFilter[1], ciFilter[1]);
  assert.match(edits, /if: steps\.paths\.outputs\.ux == 'true'/);
  assert.match(edits, /github\.workflow.*github\.event\.pull_request\.number/);
});
