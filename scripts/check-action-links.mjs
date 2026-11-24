import { readFileSync, readdirSync } from 'node:fs';
import { join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

// Read opening tags without mistaking Go comparison operators for tag endings.
export function controls(source) {
  const result = [];
  const start = /<(a|button)\b/g;
  for (let match; (match = start.exec(source));) {
    let depth = 0, quote = '', escaped = false, end = start.lastIndex;
    for (; end < source.length; end++) {
      const c = source[end];
      if (quote) {
        if (escaped) escaped = false;
        else if (c === '\\') escaped = true;
        else if (c === quote) quote = '';
      } else if (c === '"' || c === "'" || c === '`') quote = c;
      else if (c === '{') depth++;
      else if (c === '}') depth--;
      else if (c === '>' && depth === 0) break;
    }
    result.push({ tag: source.slice(match.index, end + 1), line: source.slice(0, match.index).split('\n').length });
    start.lastIndex = end + 1;
  }
  return result;
}

export function violations(source, path = '', exceptions = []) {
  return controls(source).filter(({ tag }) => {
    if (exceptions.some(item => item.path === path && tag.includes(item.selector) && item.reason)) return false;
    const cls = tag.match(/\bclass\s*=\s*(?:"([^"]*)"|\{([^}]*)\})/s);
    const classes = (cls?.[1] ?? cls?.[2] ?? '').match(/[\w:-]+/g) ?? [];
    return !(classes.includes('btn') || classes.includes('border')) || classes.includes('btn-ghost') || classes.includes('btn-link');
  });
}

function templates(dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap(entry => {
    const path = join(dir, entry.name);
    return entry.isDirectory() ? templates(path) : path.endsWith('.templ') ? [path] : [];
  });
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const root = resolve(fileURLToPath(new URL('..', import.meta.url)));
  const exceptions = JSON.parse(readFileSync(join(root, 'scripts/action-exceptions.json'), 'utf8'));
  let count = 0;
  const files = templates(join(root, 'internal/web'));
  for (const exception of exceptions) {
    const file = join(root, exception.path);
    const matches = files.includes(file) ? controls(readFileSync(file, 'utf8')).filter(item => item.tag.includes(exception.selector)) : [];
    if (!exception.reason || !exception.selector || matches.length !== 1) {
      console.error(`Invalid or ambiguous action exception: ${exception.path} ${exception.selector}`);
      count++;
    }
  }
  for (const file of files) {
    const path = relative(root, file);
    for (const item of violations(readFileSync(file, 'utf8'), path, exceptions)) {
      console.error(`${path}:${item.line}: unbounded action: ${item.tag.replace(/\s+/g, ' ')}`);
      count++;
    }
  }
  if (count) process.exitCode = 1;
}
