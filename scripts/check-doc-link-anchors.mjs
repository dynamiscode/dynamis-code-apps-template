import { readFile, readdir } from "node:fs/promises";
import path from "node:path";

const root = path.resolve(process.argv[2] ?? process.cwd());
const markdownFiles = await findMarkdownFiles(root);
const documents = new Map();
let failed = false;

for (const file of markdownFiles) {
  const content = await readFile(file, "utf8");
  documents.set(file, { content, anchors: collectAnchors(content) });
}

for (const [file, document] of documents) {
  for (const destination of markdownDestinations(document.content)) {
    const link = parseDestination(destination);
    if (!link || !link.fragment) continue;

    const target = link.file ? path.resolve(path.dirname(file), link.file) : file;
    const targetDocument = documents.get(target);
    if (!targetDocument) continue;

    let fragment;
    try {
      fragment = decodeURIComponent(link.fragment);
    } catch {
      fragment = link.fragment;
    }
    if (!targetDocument.anchors.has(fragment)) {
      console.error(`broken Markdown anchor: ${path.relative(root, file)} -> ${destination}`);
      failed = true;
    }
  }
}

if (failed) process.exitCode = 1;

async function findMarkdownFiles(directory) {
  const result = [];
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    if (entry.isDirectory() && [".git", "node_modules", "vendor"].includes(entry.name)) continue;
    const fullPath = path.join(directory, entry.name);
    if (entry.isDirectory()) result.push(...await findMarkdownFiles(fullPath));
    else if (entry.isFile() && entry.name.toLowerCase().endsWith(".md")) result.push(fullPath);
  }
  return result;
}

function markdownDestinations(content) {
  const destinations = [];
  const lines = content.split(/\r?\n/);
  let fence = null;
  for (const line of lines) {
    const marker = line.match(/^\s*(`{3,}|~{3,})/);
    if (marker) {
      if (fence === null) fence = marker[1][0];
      else if (fence === marker[1][0]) fence = null;
      continue;
    }
    if (fence !== null) continue;
    for (const match of line.matchAll(/\]\((<[^>]+>|[^)]*)\)/g)) {
      destinations.push(match[1].startsWith("<") ? match[1].slice(1, -1) : match[1]);
    }
  }
  return destinations;
}

function parseDestination(value) {
  const trimmed = value.trim();
  if (!trimmed || /^(?:[a-z][a-z0-9+.-]*:|\/|\\)/i.test(trimmed)) return null;
  const destination = trimmed.match(/^(?:<([^>]+)>|(\S+))/);
  if (!destination) return null;
  const target = destination[1] ?? destination[2];
  const hash = target.indexOf("#");
  if (hash < 0) return null;
  const rawFragment = target.slice(hash + 1);
  const rawFile = target.slice(0, hash);
  let decodedFile = rawFile;
  try {
    decodedFile = decodeURIComponent(rawFile);
  } catch {
    // Keep the literal path; the existing file check reports unresolved paths.
  }
  return { file: decodedFile, fragment: rawFragment };
}

function collectAnchors(content) {
  const anchors = new Set();
  const headings = new Map();
  const lines = content.split(/\r?\n/);
  let fence = null;
  for (const line of lines) {
    const marker = line.match(/^\s*(`{3,}|~{3,})/);
    if (marker) {
      if (fence === null) fence = marker[1][0];
      else if (fence === marker[1][0]) fence = null;
      continue;
    }
    if (fence !== null) continue;

    for (const match of line.matchAll(/\b(?:id|name)\s*=\s*(["'])(.*?)\1/gi)) anchors.add(match[2]);

    const heading = line.match(/^\s{0,3}#{1,6}\s+(.+?)\s*#*\s*$/);
    if (!heading) continue;
    const slug = headingSlug(heading[1]);
    const count = headings.get(slug) ?? 0;
    headings.set(slug, count + 1);
    anchors.add(count === 0 ? slug : `${slug}-${count}`);
  }
  return anchors;
}

function headingSlug(value) {
  const text = value
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/\[([^\]]+)\]\([^)]*\)/g, "$1")
    .replace(/`([^`]*)`/g, "$1")
    .replace(/&(?:amp|quot|apos|lt|gt);/gi, " ")
    .replace(/[\*_~]/g, "");
  return stripCompleteHTMLTags(text);
}

function stripCompleteHTMLTags(value) {
  let text = "";
  for (let index = 0; index < value.length;) {
    const end = value[index] === "<" ? completeHTMLTagEnd(value, index) : -1;
    if (end >= 0) {
      index = end + 1;
      continue;
    }
    text += value[index];
    index++;
  }
  return text.toLowerCase().replace(/[^\p{L}\p{N}_\-\s]/gu, "").trim().replace(/[\s-]+/g, "-");
}

function completeHTMLTagEnd(value, start) {
  if (value.startsWith("<!--", start)) {
    const end = value.indexOf("-->", start + 4);
    return end < 0 ? -1 : end + 2;
  }

  const name = value.slice(start + 1).match(/^\/?[A-Za-z][A-Za-z0-9:-]*/);
  if (!name) return -1;

  const boundary = value[start + 1 + name[0].length];
  if (boundary !== ">" && boundary !== "/" && !/\s/.test(boundary ?? "")) return -1;

  let quote = "";
  for (let index = start + name[0].length + 1; index < value.length; index++) {
    const character = value[index];
    if (quote) {
      if (character === quote) quote = "";
    } else if (character === "\"" || character === "'") {
      quote = character;
    } else if (character === ">") {
      return index;
    }
  }
  return -1;
}
