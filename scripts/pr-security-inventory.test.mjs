import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const script = fileURLToPath(new URL("./pr-security-inventory.mjs", import.meta.url));
const pr = { state: "open", head: { sha: "head" }, merge_commit_sha: "merge" };
const run = { id: 7, name: "CodeQL", status: "completed", conclusion: "success", output: { annotations_count: 1 } };
const annotation = { path: "main.go", start_line: 3, end_line: 3, annotation_level: "warning", title: "unsafe", message: "inspect" };

function fixture() {
  const dir = mkdtempSync(path.join(tmpdir(), "security-inventory-"));
  const data = {
    "repos/acme/app/pulls/12": pr,
    "repos/acme/app/commits/head/check-runs?per_page=100": [{ check_runs: [run] }, { check_runs: [] }],
    "repos/acme/app/commits/merge/check-runs?per_page=100": [{ check_runs: [run] }],
    "repos/acme/app/check-runs/7/annotations?per_page=100": [[annotation]],
    "repos/acme/app/code-scanning/alerts?state=open&ref=refs%2Fpull%2F12%2Fmerge&per_page=100": [[{ number: 42, html_url: "https://example.invalid/42" }], [{ number: 43 }]],
  };
  writeFileSync(path.join(dir, "fixture-api"), `#!/usr/bin/env node\nconst fs = require('node:fs'); const data = JSON.parse(fs.readFileSync(process.env.FIXTURE)); const key = process.argv.at(-1); if (!(key in data)) process.exit(1); console.log(JSON.stringify(data[key]));\n`, { mode: 0o755 });
  const save = () => writeFileSync(path.join(dir, "fixture.json"), JSON.stringify(data));
  const invoke = (review) => {
    save();
    if (review) writeFileSync(path.join(dir, "review.json"), JSON.stringify(review));
    return spawnSync(process.execPath, [script, "acme/app", "12", ...(review ? [path.join(dir, "review.json")] : [])], { encoding: "utf8", env: { ...process.env, GH_PATH: path.join(dir, "fixture-api"), FIXTURE: path.join(dir, "fixture.json") } });
  };
  return { dir, data, invoke };
}

test("inventories all annotations and paginated alerts even when checks are green", () => {
  const f = fixture();
  try {
    const first = f.invoke();
    assert.equal(first.status, 1, first.stderr);
    const inventory = JSON.parse(first.stdout);
    assert.equal(inventory.pending, 3);
    assert.equal(inventory.checks.length, 1);
    assert.deepEqual(inventory.findings.map((item) => item.id.startsWith("alert:") ? item.id : "annotation"), ["alert:42", "alert:43", "annotation"]);
    const review = { repository: "acme/app", pull_request: 12, head: "head", merge: "merge", dispositions: inventory.findings.map(({ id }) => ({ id, disposition: "accepted-risk", reviewer: "maintainer", reason: "Reviewed impact" })) };
    assert.equal(f.invoke(review).status, 0);
    review.dispositions.pop();
    assert.equal(f.invoke(review).status, 1);
    review.dispositions.push({ id: "alert:99", disposition: "fixed", reviewer: "maintainer", reason: "Resolved" });
    assert.deepEqual(JSON.parse(f.invoke(review).stdout).stale_dispositions, ["alert:99"]);
    review.head = "old";
    assert.equal(f.invoke(review).status, 2);
  } finally { rmSync(f.dir, { recursive: true, force: true }); }
});

test("fails closed on unavailable, incomplete or unfinished data", () => {
  const f = fixture();
  try {
    delete f.data["repos/acme/app/code-scanning/alerts?state=open&ref=refs%2Fpull%2F12%2Fmerge&per_page=100"];
    assert.equal(f.invoke().status, 2);
    f.data["repos/acme/app/code-scanning/alerts?state=open&ref=refs%2Fpull%2F12%2Fmerge&per_page=100"] = [[]];
    f.data["repos/acme/app/check-runs/7/annotations?per_page=100"] = [[]];
    assert.equal(f.invoke().status, 2);
    f.data["repos/acme/app/check-runs/7/annotations?per_page=100"] = [[annotation]];
    f.data["repos/acme/app/commits/head/check-runs?per_page=100"][0].check_runs[0].status = "in_progress";
    const unfinished = f.invoke();
    assert.equal(unfinished.status, 1, unfinished.stderr);
  } finally { rmSync(f.dir, { recursive: true, force: true }); }
});
