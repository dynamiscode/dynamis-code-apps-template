export function observeNetwork(cdp) {
  const pending = new Set();
  const result = { requests: 0, redirects: 0, transportFailures: 0, httpErrors: 0,
    httpErrorDetails: [], cacheHits: 0, completedEncodedBytes: 0, requestBodyBytes: 0,
    requestsWithUnknownBodyBytes: 0 };
  const resourceTypes = new Set(["Document", "Stylesheet", "Script", "Image", "Font",
    "XHR", "Fetch", "EventSource", "Other"]);
  const listeners = {
    "Network.requestWillBeSent": (event) => {
      result.requests++;
      if (event.redirectResponse) result.redirects++;
      if (event.request?.hasPostData) {
        if (typeof event.request.postData === "string") {
          result.requestBodyBytes += Buffer.byteLength(event.request.postData);
        } else {
          const contentLength = Object.entries(event.request.headers ?? {})
            .find(([name]) => name.toLowerCase() === "content-length")?.[1];
          const bodyBytes = Number(contentLength);
          if (/^\d+$/.test(String(contentLength ?? "")) && Number.isSafeInteger(bodyBytes)) {
            result.requestBodyBytes += bodyBytes;
          } else {
            result.requestsWithUnknownBodyBytes++;
          }
        }
      }
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
