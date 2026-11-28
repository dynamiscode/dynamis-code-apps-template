import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

const stateNames = ["empty", "populated", "error", "success", "permission", "long-content"];
const missing = (value) => !value || /^(?:todo|tbd|n\/a|none provided)$/i.test(value);
const validLink = (value) => (value.match(/https:\/\/[^\s)>]+/gi) ?? []).some((link) => {
  try {
    const url = new URL(link);
    return url.protocol === "https:" && Boolean(url.hostname);
  } catch {
    return false;
  }
});

export function checkReview(body, head, resolveRevision = (revision) => execFileSync("git", ["rev-parse", "--verify", `${revision}^{commit}`], { encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] }).trim()) {
  const section = (body ?? "").match(/^## Browser UX review\s*$([\s\S]*?)(?=^## |$(?![\s\S]))/m)?.[1]
    ?.replace(/<!--[\s\S]*?-->/g, "") ?? "";
  const fields = new Map();
  const journeys = [];
  let orphanState = false;
  for (const match of section.matchAll(/^[ \t]*-[ \t]*(Status|Reason|Revision|Journey|State|Role|Viewport|Actions and outcomes|Evidence|Unresolved gaps):[ \t]*(.*?)[ \t]*$/gim)) {
    const key = match[1].toLowerCase();
    const value = match[2].trim();
    if (key === "journey") {
      journeys.push({ name: value, states: [] });
      continue;
    }
    if (key === "state") {
      if (!journeys.length) orphanState = true;
      else journeys[journeys.length - 1].states.push(value);
      continue;
    }
    if (fields.has(key)) return `Duplicate UX review field: ${key}`;
    fields.set(key, value);
  }
  const status = fields.get("status")?.toLowerCase();
  if (status !== "reviewed" && status !== "exempt") return "UX review status must be reviewed or exempt; blocked reviews cannot pass";
  const required = status === "exempt" ? ["reason"] : ["revision", "role", "viewport", "actions and outcomes", "evidence", "unresolved gaps"];
  for (const key of required) {
    if (missing(fields.get(key))) return `Missing UX review field: ${key}`;
  }
  if (status === "reviewed") {
    const revision = fields.get("revision").toLowerCase();
    if (!/^[0-9a-f]{7,40}$/.test(revision) || !/^[0-9a-f]{40}$/.test(head ?? "")) return "UX review revision must identify the current PR head commit";
    try {
      if (resolveRevision(revision) !== head) return "UX review revision must identify the current PR head commit";
    } catch {
      return "UX review revision is unknown or ambiguous";
    }
    if (!validLink(fields.get("evidence"))) return "UX review evidence must include a valid HTTPS link";
    if (!journeys.length) return "Missing UX review field: journey";
    if (orphanState) return "UX review state must follow a journey";
    const journeyNames = new Set();
    for (const journey of journeys) {
      if (missing(journey.name)) return "Missing UX review field: journey";
      if (journeyNames.has(journey.name.toLowerCase())) return `Duplicate UX review journey: ${journey.name}`;
      journeyNames.add(journey.name.toLowerCase());
      const states = new Set();
      for (const state of journey.states) {
        const parts = state.split("|").map((part) => part.trim());
        const [name, classification, detail, artifact] = parts;
        if (!stateNames.includes(name?.toLowerCase())) return `Invalid UX review state: ${name}`;
        const normalizedName = name.toLowerCase();
        if (states.has(normalizedName)) return `Duplicate UX review state: ${name}`;
        states.add(normalizedName);
        if (classification?.toLowerCase() === "blocked") return `Blocked UX review state: ${name}`;
        if (!["exercised", "not applicable"].includes(classification?.toLowerCase()) || missing(detail)) return `Invalid UX review state classification or detail: ${name}`;
        if (classification.toLowerCase() === "exercised") {
          if (parts.length !== 4 || !validLink(artifact)) return `UX review state ${name} must include a valid HTTPS artifact link`;
        } else if (parts.length !== 3) {
          return `Invalid UX review state: ${name}`;
        }
      }
      for (const name of stateNames) {
        if (!states.has(name)) return `Missing UX review state ${name} for journey: ${journey.name}`;
      }
    }
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
