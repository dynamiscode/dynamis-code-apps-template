export function observeNetwork(cdp) {
  const pending = new Set();
  const result = { requests: 0, redirects: 0, transportFailures: 0, httpErrors: 0,
    httpErrorDetails: [], cacheHits: 0, completedEncodedBytes: 0, requestClasses: {},
    repeatedGetRequests: 0, eventSourceReportedEncodedBytes: null,
    redirectReportedEncodedBytes: null };
  const resourceTypes = new Set(["Document", "Stylesheet", "Script", "Image", "Font",
    "XHR", "Fetch", "EventSource", "Other"]);
  const assets = new Set(["app.css", "clients.css", "components.css", "import.css",
    "app.js", "forms.js", "htmx.min.js", "mfa.js", "notifications.js"]);
  const seenGets = new Set();
  const streams = new Set();
  const listeners = {
    "Network.requestWillBeSent": (event) => {
      result.requests++;
      if (event.redirectResponse) {
        result.redirects++;
        const bytes = event.redirectResponse.encodedDataLength;
        if (Number.isFinite(bytes) && bytes >= 0) {
          result.redirectReportedEncodedBytes = (result.redirectReportedEncodedBytes ?? 0) + bytes;
        }
      }
      if (event.type === "EventSource") streams.add(event.requestId);
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
      streams.delete(event.requestId);
      result.completedEncodedBytes += event.encodedDataLength;
    },
    "Network.loadingFailed": (event) => {
      if (pending.delete(event.requestId)) result.transportFailures++;
      streams.delete(event.requestId);
    },
    "Network.dataReceived": (event) => {
      if (!streams.has(event.requestId) || !pending.has(event.requestId)) return;
      if (!Number.isFinite(event.encodedDataLength) || event.encodedDataLength < 0) return;
      result.eventSourceReportedEncodedBytes = (result.eventSourceReportedEncodedBytes ?? 0) + event.encodedDataLength;
    },
    "Network.requestServedFromCache": () => { result.cacheHits++; },
  };
  for (const [name, listener] of Object.entries(listeners)) cdp.on(name, listener);
  return () => {
    for (const [name, listener] of Object.entries(listeners)) cdp.off(name, listener);
    return { ...result, unfinished: pending.size };
  };
}
