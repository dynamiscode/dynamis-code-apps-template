import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import test from "node:test";

const checker = fileURLToPath(new URL("./check-review-images.mjs", import.meta.url));

test("rejects unreferenced review images and reports duplicates", () => {
  const root = mkdtempSync(path.join(tmpdir(), "review-images-"));
  try {
    mkdirSync(path.join(root, "docs/reviews"), { recursive: true });
    writeFileSync(path.join(root, "docs/reviews/a.png"), "same");
    writeFileSync(path.join(root, "docs/reviews/b.png"), "same");
    writeFileSync(path.join(root, "docs/reviews/review.md"), "[before](a.png)\n");
    const result = spawnSync(process.execPath, [checker, root], { encoding: "utf8" });
    assert.equal(result.status, 1);
    assert.match(result.stderr, /unreferenced review image: docs\/reviews\/b.png/);
    assert.match(result.stderr, /duplicate review images: docs\/reviews\/a.png, docs\/reviews\/b.png/);
    writeFileSync(path.join(root, "docs/reviews/review.md"), "[before](a.png) [after](b.png)\n");
    const referenced = spawnSync(process.execPath, [checker, root], { encoding: "utf8" });
    assert.equal(referenced.status, 0, referenced.stderr);
    assert.match(referenced.stderr, /duplicate review images/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
