import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const checker = fileURLToPath(new URL("./check-formal-verification.mjs", import.meta.url));

test("reports covered changes without treating them as proof of drift", () => {
  const root = fixture();
  try {
    git(root, "init", "-q");
    writeFileSync(path.join(root, "internal/example.go"), "package example\n");
    git(root, "add", "internal/example.go");
    git(root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "-qm", "baseline");
    const sync = git(root, "rev-parse", "HEAD").trim();
    writeSkill(root, `Sync commit: \`${sync}\`\nSee \`internal/example.go\`.\n`);
    const unchanged = run(root);
    assert.equal(unchanged.status, 0, unchanged.stderr);
    assert.equal(unchanged.stderr, "");
    writeFileSync(path.join(root, "internal/example.go"), "package changed\n");
    const result = run(root);
    assert.equal(result.status, 0);
    assert.match(result.stderr, /covered files changed.*review required/s);
    assert.match(result.stderr, /internal\/example\.go/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("rejects missing referenced files and reports unknown history", () => {
  const root = fixture();
  try {
    git(root, "init", "-q");
    writeSkill(root, `Sync commit: \`${"a".repeat(40)}\`\nSee \`internal/missing.go\`.\n`);
    const result = run(root);
    assert.equal(result.status, 1);
    assert.match(result.stderr, /missing referenced file internal\/missing\.go/);
    assert.match(result.stderr, /review status unknown/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("does not apply template coverage to generated applications", () => {
  const root = fixture();
  try {
    writeFileSync(path.join(root, "template.lock"), "{}");
    const result = run(root);
    assert.equal(result.status, 0);
    assert.match(result.stdout, /generated applications must establish their own coverage/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

function fixture() {
  const root = mkdtempSync(path.join(tmpdir(), "formal-coverage-"));
  for (const dir of [".agents/skills/check", "verification", "internal"]) mkdirSync(path.join(root, dir), { recursive: true });
  writeFileSync(path.join(root, "verification/coverage.json"), JSON.stringify({ check: ["internal/example.go"] }));
  return root;
}

function writeSkill(root, text) {
  writeFileSync(path.join(root, ".agents/skills/check/SKILL.md"), text);
}

function git(root, ...args) {
  return execFileSync("git", args, { cwd: root, encoding: "utf8" });
}

function run(root) {
  return spawnSync(process.execPath, [checker, root], { encoding: "utf8" });
}
