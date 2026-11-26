import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

export function checkReview(body, head, resolveRevision = (revision) => execFileSync("git", ["rev-parse", "--verify", `${revision}^{commit}`], { encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] }).trim()) {
  const section = (body ?? "").match(/^## Browser UX review\s*$([\s\S]*?)(?=^## |$(?![\s\S]))/m)?.[1]
    ?.replace(/<!--[\s\S]*?-->/g, "") ?? "";
  const fields = new Map();
  for (const match of section.matchAll(/^[ \t]*-[ \t]*(Status|Reason|Revision|Journey|Role|Viewport|Actions and outcomes|Evidence|Unresolved gaps):[ \t]*(.*?)[ \t]*$/gim)) {
    const key = match[1].toLowerCase();
    if (fields.has(key)) return `Duplicate UX review field: ${key}`;
    fields.set(key, match[2].trim());
  }
  const status = fields.get("status")?.toLowerCase();
  if (status !== "reviewed" && status !== "exempt") return "UX review status must be reviewed or exempt; blocked reviews cannot pass";
  const required = status === "exempt" ? ["reason"] : ["revision", "journey", "role", "viewport", "actions and outcomes", "evidence", "unresolved gaps"];
  for (const key of required) {
    if (!fields.get(key) || /^(?:todo|tbd|n\/a|none provided)$/i.test(fields.get(key))) return `Missing UX review field: ${key}`;
  }
  if (status === "reviewed") {
    const revision = fields.get("revision").toLowerCase();
    if (!/^[0-9a-f]{7,40}$/.test(revision) || !/^[0-9a-f]{40}$/.test(head ?? "")) return "UX review revision must identify the current PR head commit";
    try {
      if (resolveRevision(revision) !== head) return "UX review revision must identify the current PR head commit";
    } catch {
      return "UX review revision is unknown or ambiguous";
    }
    const links = fields.get("evidence").match(/https:\/\/[^\s)>]+/gi) ?? [];
    const valid = links.some((link) => {
      try {
        const url = new URL(link);
        return url.protocol === "https:" && Boolean(url.hostname);
      } catch {
        return false;
      }
    });
    if (!valid) return "UX review evidence must include a valid HTTPS link";
  }
  return null;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    const event = JSON.parse(readFileSync(process.argv[2], "utf8"));
    const error = checkReview(event.pull_request?.body, event.pull_request?.head?.sha);
    if (error) throw new Error(error);
  } catch (error) {
    console.error(`UX review gate: ${error.message}`);
    process.exitCode = 1;
  }
}
