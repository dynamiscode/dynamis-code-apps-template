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

export function componentCalls(source) {
  const result = [];
  const start = /@atoms\.(Anchor|Link|Button)\(atoms\.\1Props\s*\{/g;
  for (let match; (match = start.exec(source));) {
    let depth = 1, quote = '', escaped = false, end = start.lastIndex;
    for (; end < source.length && depth; end++) {
      const c = source[end];
      if (quote) {
        if (escaped) escaped = false;
        else if (c === '\\') escaped = true;
        else if (c === quote) quote = '';
      } else if (c === '"' || c === "'" || c === '`') quote = c;
      else if (c === '{') depth++;
      else if (c === '}') depth--;
    }
    result.push({ tag: source.slice(match.index, end), line: source.slice(0, match.index).split('\n').length, kind: match[1] });
    start.lastIndex = end;
  }
  return result;
}

function approved(classes, needsBoundary = true) {
  const tokens = classes.split(/\s+/);
  return (!needsBoundary || tokens.includes('btn') || tokens.includes('border')) &&
    !tokens.includes('btn-ghost') && !tokens.includes('btn-link');
}

function boundedHelper(source, expression) {
  const call = expression.match(/^\s*(listActionItemClass|listViewOptionClass)\(/);
  if (!call) return false;
  const body = source.match(new RegExp(`func ${call[1]}\\([^\\n]*\\) string \\{([\\s\\S]*?)\\n\\}`))?.[1];
  if (!body) return false;
  const returns = [...body.matchAll(/\breturn\s+([^\n}]+)/g)];
  return returns.length > 0 && returns.every(match => {
    const literal = match[1].trim().match(/^"([^"\\]*)"$/);
    return literal && approved(literal[1]);
  });
}

export function violations(source, path = '', exceptions = []) {
  const excepted = tag => exceptions.some(item => item.path === path && tag.includes(item.selector) && item.reason);
  const badTags = controls(source).filter(({ tag }) => {
    if (excepted(tag)) return false;
    const cls = tag.match(/\bclass\s*=\s*(?:"([^"]*)"|\{([^}]*)\})/s);
    const expression = cls?.[2] ?? '';
    // Only an unconditional top-level literal may supply the boundary.
    const base = cls?.[1] ?? expression.match(/^\s*"([^"\\]*(?:\\.[^"\\]*)*)"/)?.[1] ?? '';
    const allLiterals = [...expression.matchAll(/"([^"\\]*(?:\\.[^"\\]*)*)"/g)].map(m => m[1]).join(' ');
    return !(approved(base) || boundedHelper(source, expression)) || !approved(allLiterals, false);
  });
  const badCalls = componentCalls(source).filter(({ tag, kind }) => {
    if (excepted(tag)) return false;
    const cls = tag.match(/\bClass\s*:\s*([^,}\n]+)/);
    if (!cls) return kind !== 'Button';
    const literal = cls[1].trim().match(/^"([^"\\]*)"$/);
    return !literal || !approved(literal[1], kind !== 'Button');
  });
  return [...badTags, ...badCalls];
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
    const source = files.includes(file) ? readFileSync(file, 'utf8') : '';
    const matches = [...controls(source), ...componentCalls(source)].filter(item => item.tag.includes(exception.selector));
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
