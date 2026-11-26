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
for (const file of files(docs).filter((name) => name.endsWith(".md"))) {
  for (const match of readFileSync(file, "utf8").matchAll(/\]\((?:<([^>]+)>|([^\s)]+))(?:\s+[^)]*)?\)/g)) {
    const target = (match[1] ?? match[2]).split("#", 1)[0];
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
