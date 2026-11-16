const assets = new Set(["app.css", "clients.css", "components.css", "import.css",
  "app.js", "forms.js", "htmx.min.js", "mfa.js", "notifications.js"]);

export function requestClass(rawURL, baseURL, type) {
  try {
    const url = new URL(rawURL);
    if (url.origin !== new URL(baseURL).origin) return "external";
    const match = /^\/assets\/([^/]+)$/.exec(url.pathname);
    if (match && assets.has(match[1])) return `asset:${match[1]}`;
    if (url.pathname === "/favicon.ico") return "favicon";
  } catch {
    return "other";
  }
  return ["document", "xhr", "fetch", "eventsource"].includes(type) ? type : "other";
}

export function recordRequest(counts, rawURL, baseURL, type) {
  const key = requestClass(rawURL, baseURL, type);
  counts[key] = (counts[key] ?? 0) + 1;
}
