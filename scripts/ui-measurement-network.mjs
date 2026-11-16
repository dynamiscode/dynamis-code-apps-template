export function observeNetwork(cdp) {
  const pending = new Set();
  const result = { requests: 0, redirects: 0, transportFailures: 0, httpErrors: 0,
    httpErrorDetails: [], cacheHits: 0, completedEncodedBytes: 0, requestClasses: {},
    repeatedGetRequests: 0 };
  const resourceTypes = new Set(["Document", "Stylesheet", "Script", "Image", "Font",
    "XHR", "Fetch", "EventSource", "Other"]);
  const assets = new Set(["app.css", "clients.css", "components.css", "import.css",
    "app.js", "forms.js", "htmx.min.js", "mfa.js", "notifications.js"]);
  const seenGets = new Set();
  const listeners = {
    "Network.requestWillBeSent": (event) => {
      result.requests++;
      if (event.redirectResponse) result.redirects++;
      let category = resourceTypes.has(event.type) ? event.type : "Other";
      try {
        const url = new URL(event.request.url);
        const asset = url.pathname.startsWith("/assets/") ? url.pathname.slice(8) : "";
        if (assets.has(asset)) category = asset;
        if (event.request.method === "GET") {
          if (seenGets.has(event.request.url)) result.repeatedGetRequests++;
          seenGets.add(event.request.url);
        }
      } catch {
        // Unknown requests retain only their allowlisted resource type.
      }
      result.requestClasses[category] = (result.requestClasses[category] || 0) + 1;
      pending.add(event.requestId);
    },
    "Network.responseReceived": (event) => {
      if (!pending.has(event.requestId) || event.response.status < 400) return;
      result.httpErrors++;
      let resource = "other";
      try {
        if (new URL(event.response.url).pathname === "/favicon.ico") resource = "favicon";
      } catch {
        // Invalid URLs remain unclassified.
      }
      result.httpErrorDetails.push({ status: event.response.status,
        type: resourceTypes.has(event.type) ? event.type : "Other", resource });
    },
    "Network.loadingFinished": (event) => {
      if (!pending.delete(event.requestId)) return;
      result.completedEncodedBytes += event.encodedDataLength;
    },
    "Network.loadingFailed": (event) => {
      if (pending.delete(event.requestId)) result.transportFailures++;
    },
    "Network.requestServedFromCache": () => { result.cacheHits++; },
  };
  for (const [name, listener] of Object.entries(listeners)) cdp.on(name, listener);
  return () => {
    for (const [name, listener] of Object.entries(listeners)) cdp.off(name, listener);
    return { ...result, unfinished: pending.size };
  };
}
