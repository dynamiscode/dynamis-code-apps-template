import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";

const [repo, number, reviewPath] = process.argv.slice(2);
if (!/^[\w.-]+\/[\w.-]+$/.test(repo || "") || !/^[1-9]\d*$/.test(number || "")) {
  console.error("Usage: node scripts/pr-security-inventory.mjs OWNER/REPO PR_NUMBER [review.json]");
  process.exit(2);
}

function api(endpoint, paginate = false) {
  return JSON.parse(execFileSync(process.env.GH_PATH || "gh", ["api", ...(paginate ? ["--paginate", "--slurp"] : []), endpoint], { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] }));
}

try {
  const pr = api(`repos/${repo}/pulls/${number}`);
  const head = pr.head.sha;
  const merge = pr.merge_commit_sha;
  if (!head || !merge || pr.state !== "open") throw new Error("Open PR with available head and merge commit required; retry when GitHub has computed the merge commit");
  const review = reviewPath ? JSON.parse(readFileSync(reviewPath, "utf8")) : null;
  if (review && (review.repository !== repo || review.pull_request !== Number(number) || review.head !== head || review.merge !== merge || !Array.isArray(review.dispositions))) {
    throw new Error("Review must match repository, PR, head and merge commit and contain a dispositions array");
  }
  const dispositions = new Map();
  for (const entry of review?.dispositions || []) {
    if (!entry || typeof entry.id !== "string" || dispositions.has(entry.id) || !["fixed", "false-positive", "accepted-risk"].includes(entry.disposition) || typeof entry.reviewer !== "string" || !entry.reviewer.trim() || typeof entry.reason !== "string" || !entry.reason.trim()) {
      throw new Error("Each unique disposition needs an id, fixed/false-positive/accepted-risk disposition, reviewer, and reason");
    }
    dispositions.set(entry.id, entry);
  }
  const runs = new Map();
  const mergeRuns = [];
  for (const sha of new Set([head, merge])) {
    for (const page of api(`repos/${repo}/commits/${sha}/check-runs?per_page=100`, true)) {
      if (!Array.isArray(page.check_runs)) throw new Error("Invalid check runs response");
      for (const run of page.check_runs) {
        runs.set(run.id, run);
        if (sha === merge) mergeRuns.push(run);
      }
    }
  }
  if (!runs.size) throw new Error("No check runs found; security inventory is incomplete");
  const findings = [];
  for (const run of runs.values()) {
    const pages = api(`repos/${repo}/check-runs/${run.id}/annotations?per_page=100`, true);
    const annotations = pages.flat();
    if (annotations.length !== run.output.annotations_count) throw new Error(`Incomplete annotations for check run ${run.id}`);
    for (const annotation of annotations) {
      const digest = createHash("sha256").update(JSON.stringify([run.id, annotation.path, annotation.start_line, annotation.end_line, annotation.annotation_level, annotation.title, annotation.message, annotation.raw_details])).digest("hex");
      findings.push({ id: `annotation:${digest}`, check_run: run.name, check_run_id: run.id, ...annotation });
    }
  }
  for (const page of api(`repos/${repo}/code-scanning/alerts?state=open&ref=refs%2Fpull%2F${number}%2Fmerge&per_page=100`, true)) {
    if (!Array.isArray(page)) throw new Error("Invalid code scanning alerts response");
    for (const alert of page) findings.push({ id: `alert:${alert.number}`, alert });
  }
  findings.sort((a, b) => a.id.localeCompare(b.id));
  const current = api(`repos/${repo}/pulls/${number}`);
  if (current.state !== "open" || current.head.sha !== head || current.merge_commit_sha !== merge) throw new Error("PR changed during inventory; retry");
  const ids = new Set(findings.map((finding) => finding.id));
  const stale = [...dispositions.keys()].filter((id) => !ids.has(id));
  const pending = findings.filter((finding) => !dispositions.has(finding.id)).length;
  const incomplete = [...runs.values()].filter((run) => run.status !== "completed").map((run) => ({ id: run.id, name: run.name, status: run.status }));
  const requiredChecks = ["CodeQL (Go)", "CodeQL (JavaScript)"].map((name) => {
    const latest = mergeRuns.filter((run) => run.name === name).sort((a, b) => b.id - a.id)[0];
    return { name, id: latest?.id ?? null, status: latest?.status ?? "missing", conclusion: latest?.conclusion ?? null };
  });
  const securityChecksIncomplete = requiredChecks.some((run) => run.status !== "completed" || run.conclusion !== "success");
  const output = {
    repository: repo, pull_request: Number(number), head, merge,
    checks: [...runs.values()].map(({ id, name, status, conclusion, html_url }) => ({ id, name, status, conclusion, html_url })).sort((a, b) => a.id - b.id),
    findings: findings.map((finding) => ({ ...finding, review: dispositions.get(finding.id) || null })),
    pending, stale_dispositions: stale, incomplete_checks: incomplete, required_security_checks: requiredChecks,
  };
  console.log(JSON.stringify(output, null, 2));
  if (pending || stale.length || incomplete.length || securityChecksIncomplete) process.exitCode = 1;
} catch (error) {
  console.error(`Security inventory failed: ${error.message}`);
  process.exitCode = 2;
}
