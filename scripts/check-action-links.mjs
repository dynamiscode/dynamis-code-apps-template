export function violations(source) {
  return [...source.matchAll(/<a\b[^>]*>/gs)].map(match => match[0]).filter(tag => {
    const classes = tag.match(/\bclass="([^"]*)"/)?.[1]?.split(/\s+/) ?? [];
    if (!classes.includes('link') || classes.includes('border')) return false;
    return !/\bdata-action-exception="(?:demo|accessibility)"/.test(tag);
  });
}
