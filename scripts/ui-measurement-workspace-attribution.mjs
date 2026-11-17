const types = new Set(["Document", "XHR", "Fetch", "EventSource"]);
const methods = new Set(["GET", "POST", "PUT", "PATCH", "DELETE"]);

export function observeWorkspaceAttribution(cdp) {
  const seen = new Map();
  const requests = new Map();
  const repeatedGets = [];
  const xhrMethods = {};
  const onRequest = (event) => {
    const method = methods.has(event.request?.method) ? event.request.method : "other";
    const type = types.has(event.type) ? event.type : "Other";
    if (type === "XHR") xhrMethods[method] = (xhrMethods[method] || 0) + 1;
    if (method !== "GET" || !event.request?.url) return;
    const previous = seen.get(event.request.url);
    if (previous) {
      const entry = { firstType: previous.type, repeatedType: type, firstStatus: previous.status ?? null, repeatedStatus: null };
      repeatedGets.push(entry);
      requests.set(event.requestId, entry);
    } else {
      const entry = { type, status: null };
      seen.set(event.request.url, entry);
      requests.set(event.requestId, entry);
    }
  };
  const onResponse = (event) => {
    const entry = requests.get(event.requestId);
    const status = event.response?.status;
    if (!entry || !Number.isInteger(status) || status < 100 || status > 599) return;
    if ("repeatedType" in entry) entry.repeatedStatus = status;
    else entry.status = status;
  };
  cdp.on("Network.requestWillBeSent", onRequest);
  cdp.on("Network.responseReceived", onResponse);
  return () => {
    cdp.off("Network.requestWillBeSent", onRequest);
    cdp.off("Network.responseReceived", onResponse);
    return { repeatedGets, xhrMethods };
  };
}
