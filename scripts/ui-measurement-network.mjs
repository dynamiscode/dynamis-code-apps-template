export function observeNetwork(cdp) {
  const pending = new Set();
  const result = { requests: 0, redirects: 0, transportFailures: 0, httpErrors: 0,
    cacheHits: 0, completedEncodedBytes: 0 };
  const listeners = {
    "Network.requestWillBeSent": (event) => {
      result.requests++;
      if (event.redirectResponse) result.redirects++;
      pending.add(event.requestId);
    },
    "Network.responseReceived": (event) => {
      if (pending.has(event.requestId) && event.response.status >= 400) result.httpErrors++;
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
