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
  const start = /@([\w]+)\.(Anchor|Link|Button|ConfirmButton)\s*\(/g;
  for (let match; (match = start.exec(source));) {
    let depth = 1, quote = '', escaped = false, end = start.lastIndex;
    for (; end < source.length && depth; end++) {
      const c = source[end];
      if (quote) {
        if (escaped) escaped = false;
        else if (c === '\\') escaped = true;
        else if (c === quote) quote = '';
      } else if (c === '"' || c === "'" || c === '`') quote = c;
      else if (c === '(') depth++;
      else if (c === ')') depth--;
    }
    result.push({ tag: source.slice(match.index, end), line: source.slice(0, match.index).split('\n').length, kind: match[2], namespace: match[1] });
    start.lastIndex = end;
  }
  return result;
}

function approved(classes) {
  return typeof classes === 'string';
}

function boundedHelper(source, expression) {
  const call = expression.match(/^\s*(listActionItemClass|listViewOptionClass)\(/);
  if (!call) return false;
  const body = source.match(new RegExp(`func ${call[1]}\\([^\\n]*\\) string \\{([\\s\\S]*?)\\n\\}`))?.[1];
  if (!body) return false;
  const returns = [...body.matchAll(/\breturn\s+([^\n}]+)/g)];
  return returns.length > 0 && returns.every(match => /^"[^"\\]*"$/.test(match[1].trim()));
}


function splitFields(source) {
  const fields = [];
  const stack = [];
  let quote = '', escaped = false, start = 0;
  for (let i = 0; i < source.length; i++) {
    const c = source[i];
    if (quote) {
      if (escaped) escaped = false;
      else if (c === '\\' && quote !== '`') escaped = true;
      else if (c === quote) quote = '';
    } else if (c === '"' || c === "'" || c === '`') quote = c;
    else if (c === '/' && (source[i + 1] === '/' || source[i + 1] === '*')) return null;
    else if ('({['.includes(c)) stack.push(c);
    else if (')}]'.includes(c)) {
      if (stack.pop() !== { ')': '(', '}': '{', ']': '[' }[c]) return null;
    } else if (c === ',' && !stack.length) {
      fields.push(source.slice(start, i).trim());
      start = i + 1;
    }
  }
  if (quote || stack.length) return null;
  fields.push(source.slice(start).trim());
  return fields.filter(Boolean);
}

function safeAttributes(expression) {
  const map = expression.match(/^templ\.Attributes\s*\{([\s\S]*)\}$/);
  if (!map) return false;
  const fields = splitFields(map[1]);
  return fields !== null && fields.every(field => {
    const entry = field.match(/^"([a-z][a-z0-9-]*)"\s*:\s*([\s\S]+)$/);
    return !!entry && !!entry[2].trim() && entry[1] !== 'class' && entry[1] !== 'style';
  });
}

export function violations(source, path = '', exceptions = []) {
  const excepted = tag => exceptions.some(item => item.path === path && item.reason && item.selector &&
    (tag === item.selector || new RegExp(`(?:^|[\\s<])${item.selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}(?=[\\s/>}]|$)`).test(tag)));
  const badTags = controls(source).filter(({ tag }) => {
    if (excepted(tag)) return false;
    if (/\{[^}]*\.\.\.\s*\}/s.test(tag)) return true;
    const cls = tag.match(/\bclass\s*=\s*(?:"([^"]*)"|\{([^}]*)\})/s);
    if (!cls) return true;
    if (cls[1] !== undefined) return !approved(cls[1]);
    const fields = splitFields(cls[2]);
    if (!fields) return true;
    let base = '', helper = false;
    for (const field of fields) {
      const literal = field.match(/^"([^"\\]*)"$/);
      if (literal) {
        if (!approved(literal[1])) return true;
        base += ' ' + literal[1];
        continue;
      }
      const conditional = field.match(/^templ\.KV\(([\s\S]*)\)$/);
      if (conditional) {
        const args = splitFields(conditional[1]);
        const conditionalClass = args?.[0]?.match(/^"([^"\\]*)"$/);
        if (!args || args.length !== 2 || !conditionalClass || !approved(conditionalClass[1])) return true;
        helper = true;
        continue;
      }
      const expression = field.match(/^actionattrs\.LinkClass\(([\s\S]*)\)$/)?.[1] ?? field;
      if (!/^(?:listActionItemClass\(action\)|listViewOptionClass\((?:view\.Current|listViewNoneApplied\(data\)|true)\))$/.test(expression) || !boundedHelper(source, expression)) return true;
      helper = true;
    }
    return !base && !helper;
  });
  const badCalls = componentCalls(source).filter(({ tag, kind, namespace }) => {
    if (excepted(tag)) return false;
    if (namespace === 'molecules' && kind === 'ConfirmButton') {
      const args = splitFields(tag.slice(tag.indexOf('(') + 1, -1));
      return !args || args.length !== 3 || (args[2] !== 'nil' && !safeAttributes(args[2]));
    }
    if (namespace !== 'atoms' || !new RegExp(`^@atoms\\.${kind}\\(atoms\\.${kind}Props\\s*\\{`).test(tag)) return true;
    const props = tag.match(/^@atoms\.\w+\(atoms\.\w+Props\s*\{([\s\S]*)\}\s*\)$/);
    if (!props) return true;
    const fields = splitFields(props[1]);
    if (!fields) return true;
    const attributes = fields.filter(field => /^Attrs\s*:/.test(field));
    if (attributes.length > 1 || (attributes.length && !safeAttributes(attributes[0].replace(/^Attrs\s*:\s*/, '')))) return true;
    const classes = fields.filter(field => /^Class\s*:/.test(field));
    if (classes.length > 1) return true;
    if (!classes.length) return kind !== 'Button';
    const literal = classes[0].replace(/^Class\s*:\s*/, '').trim().match(/^"([^"\\]*)"$/);
    return !literal || !approved(literal[1]);
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
