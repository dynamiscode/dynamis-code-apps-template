import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(process.argv[2] ?? path.join(path.dirname(fileURLToPath(import.meta.url)), ".."));
if (existsSync(path.join(root, "template.lock"))) {
  console.log("Formal verification coverage check is template-only; generated applications must establish their own coverage.");
  process.exit(0);
}
const skills = path.join(root, ".agents/skills");
const coverage = JSON.parse(readFileSync(path.join(root, "verification/coverage.json"), "utf8"));
let invalid = false;

for (const name of readdirSync(skills)) {
  const file = path.join(skills, name, "SKILL.md");
  if (!existsSync(file)) continue;
  const contents = readFileSync(file, "utf8");
  for (const match of contents.matchAll(/`((?:internal|verification|docs|scripts|cmd)\/[\w./-]+\.(?:go|js|mjs|templ|tla|cfg|md|sh))`/g)) {
    if (!existsSync(path.join(root, match[1]))) {
      console.error(`${path.relative(root, file)}: missing referenced file ${match[1]}`);
      invalid = true;
    }
  }
}

for (const [name, files] of Object.entries(coverage)) {
  const file = path.join(skills, name, "SKILL.md");
  if (!existsSync(file)) {
    console.error(`missing verification skill: ${name}`);
    invalid = true;
    continue;
  }
  const sync = readFileSync(file, "utf8").match(/Sync commit: `([a-f0-9]{40})`/)?.[1];
  if (!sync) {
    console.error(`${name}: missing explicit sync commit`);
    invalid = true;
    continue;
  }
  try {
    execFileSync("git", ["merge-base", "--is-ancestor", sync, "HEAD"], { cwd: root, stdio: "ignore" });
    const changed = execFileSync("git", ["diff", "--name-only", sync, "--", ...files], { cwd: root, encoding: "utf8" }).trim();
    if (changed) console.warn(`${name}: covered files changed since ${sync}; review required:\n${changed}`);
  } catch {
    console.warn(`${name}: sync commit ${sync} is unavailable or not an ancestor; review status unknown`);
  }
}

if (!invalid) console.log("Skill file references checked; see any drift warnings above.");
process.exitCode = invalid ? 1 : 0;
