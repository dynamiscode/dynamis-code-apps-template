import { createHash } from 'node:crypto';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

function isScript(path) {
  return /\.(?:cjs|mjs|js|html?|svg)$/i.test(path) && !/\.test\.mjs$/i.test(path);
}

export function verifyReviewedScripts(root, reviewed) {
  const failures = [];
  const found = new Set();
  function walk(directory) {
    for (const entry of readdirSync(directory, { withFileTypes: true })) {
      const absolute = join(directory, entry.name);
      const path = relative(root, absolute).split(sep).join('/');
      if (entry.isSymbolicLink()) {
        failures.push(`${path}: symbolic links are not reviewed browser assets`);
      } else if (entry.isDirectory()) {
        walk(absolute);
      } else if (isScript(path)) {
        found.add(path);
        const review = Object.hasOwn(reviewed, path) && reviewed[path];
        if (!review || !review.reason?.trim() || !/^[0-9a-f]{64}$/.test(review.sha256) || !Number.isSafeInteger(review.bytes) || review.bytes < 0) {
          failures.push(`${path}: missing a valid reviewed browser script entry`);
          continue;
        }
        if (!entry.isFile() || statSync(absolute).size !== review.bytes
          || createHash('sha256').update(readFileSync(absolute)).digest('hex') !== review.sha256) {
          failures.push(`${path}: browser script differs from the reviewed bytes`);
        }
      }
    }
  }
  walk(root);
  for (const path of Object.keys(reviewed)) {
    if (!found.has(path)) failures.push(`${path}: stale reviewed browser script entry`);
  }
  return failures.sort();
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const root = fileURLToPath(new URL('..', import.meta.url));
  const reviewed = JSON.parse(readFileSync(join(root, 'scripts/reviewed-browser-scripts.json'), 'utf8'));
  for (const failure of verifyReviewedScripts(join(root, 'internal/web/assets'), reviewed)) {
    console.error(`internal/web/assets/${failure}`);
    process.exitCode = 1;
  }
}
