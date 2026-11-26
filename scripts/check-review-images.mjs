import { createHash } from "node:crypto";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import path from "node:path";

const root = path.resolve(process.argv[2] ?? ".");
const docs = path.join(root, "docs");

function files(dir) {
  if (!existsSync(dir)) return [];
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const name = path.join(dir, entry.name);
    return entry.isDirectory() ? files(name) : entry.isFile() ? [name] : [];
  });
}

const images = ["reviews", "evidence"].flatMap((dir) => files(path.join(docs, dir)))
  .filter((name) => /\.(png|jpe?g|webp|gif|svg)$/i.test(name)).sort();
const referenced = new Set();
const rootMarkdown = readdirSync(root, { withFileTypes: true })
  .filter((entry) => entry.isFile() && entry.name.endsWith(".md"))
  .map((entry) => path.join(root, entry.name));
for (const file of [...rootMarkdown, ...files(docs).filter((name) => name.endsWith(".md"))]) {
  let fence = "";
  const lines = readFileSync(file, "utf8").split(/\r?\n/).filter((line) => {
    const marker = line.match(/^ {0,3}(`{3,}|~{3,})/);
    if (marker && (!fence || marker[1][0] === fence[0] && marker[1].length >= fence.length)) {
      fence = fence ? "" : marker[1];
      return false;
    }
    return !fence;
  });
  const text = lines.join("\n");
  const definitions = new Map([...text.matchAll(/^ {0,3}\[([^\]]+)\]:\s*(?:<([^>]+)>|(\S+))/gm)]
    .map((match) => [match[1].trim().toLowerCase(), match[2] ?? match[3]]));
  const targets = [
    ...[...text.matchAll(/\]\((?:<([^>]+)>|([^\s)]+))(?:\s+[^)]*)?\)/g)].map((m) => m[1] ?? m[2]),
    ...[...text.matchAll(/\[([^\]]+)\]\[([^\]]*)\]/g)].map((m) => definitions.get((m[2] || m[1]).trim().toLowerCase())),
    ...[...text.matchAll(/<img\b[^>]*\bsrc\s*=\s*["']([^"']+)["']/gi)].map((m) => m[1]),
  ];
  for (const value of targets) {
    const target = value?.split("#", 1)[0];
    if (!target || /^(?:[a-z][a-z\d+.-]*:|\/)/i.test(target)) continue;
    try { referenced.add(path.resolve(path.dirname(file), decodeURIComponent(target))); } catch { /* Invalid links are handled by doc-links. */ }
  }
}

let failed = false;
const hashes = new Map();
for (const file of images) {
  const relative = path.relative(root, file).split(path.sep).join("/");
  if (!referenced.has(file)) {
    console.error(`unreferenced review image: ${relative}`);
    failed = true;
  }
  const hash = createHash("sha256").update(readFileSync(file)).digest("hex");
  if (hashes.has(hash)) console.error(`duplicate review images: ${hashes.get(hash)}, ${relative}`);
  else hashes.set(hash, relative);
}
if (failed) process.exitCode = 1;
