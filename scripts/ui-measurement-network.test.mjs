import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import { test } from "node:test";
import { observeNetwork } from "./ui-measurement-network.mjs";

test("distinguishes HTTP errors from transport failures", () => {
  const cdp = new EventEmitter();
  const stop = observeNetwork(cdp);
  for (const [requestId, status] of [["ok", 200], ["missing", 404], ["error", 500]]) {
    cdp.emit("Network.requestWillBeSent", { requestId });
    cdp.emit("Network.responseReceived", { requestId, type: requestId === "missing" ? "Image" : "secret",
      response: { status, url: requestId === "missing" ? "http://localhost/favicon.ico?secret=hidden" : "http://localhost/private/secret" } });
    cdp.emit("Network.loadingFinished", { requestId, encodedDataLength: 10 });
  }
  cdp.emit("Network.requestWillBeSent", { requestId: "failed" });
  cdp.emit("Network.loadingFailed", { requestId: "failed" });
  const result = stop();
  assert.equal(result.httpErrors, 2);
  assert.deepEqual(result.httpErrorDetails, [
    { status: 404, type: "Image", resource: "favicon" },
    { status: 500, type: "Other", resource: "other" },
  ]);
  assert.doesNotMatch(JSON.stringify(result), /secret|private|localhost/);
  assert.equal(result.transportFailures, 1);
  assert.equal(result.requests, 4);
  assert.equal(result.completedEncodedBytes, 30);
  assert.equal(result.unfinished, 0);
  assert.equal(cdp.listenerCount("Network.responseReceived"), 0);
});

test("attributes assets and repeated GETs without exposing URLs or request data", () => {
  const cdp = new EventEmitter();
  const stop = observeNetwork(cdp);
  for (const [requestId, method, url, type] of [
    ["a", "GET", "https://example.test/assets/app.js?secret=private", "Script"],
    ["b", "GET", "https://example.test/assets/app.js?secret=private", "Script"],
    ["c", "POST", "https://example.test/private/token", "Fetch"],
    ["d", "POST", "https://example.test/private/token", "Fetch"],
    ["e", "GET", "https://example.test/assets/secret.js", "Script"],
    ["f", "GET", "https://example.test/events?token=secret", "EventSource"],
  ]) cdp.emit("Network.requestWillBeSent", { requestId, type, request: { method, url } });
  const result = stop();
  assert.deepEqual(result.requestClasses, { "app.js": 2, Fetch: 2, Script: 1, EventSource: 1 });
  assert.equal(result.repeatedGetRequests, 1);
  assert.equal(result.unfinished, 6);
  assert.doesNotMatch(JSON.stringify(result), /secret|private|example|token/);
});

test("records reported stream and redirect bytes separately", () => {
  const cdp = new EventEmitter();
  const stop = observeNetwork(cdp);
  cdp.emit("Network.requestWillBeSent", { requestId: "stream", type: "EventSource" });
  cdp.emit("Network.dataReceived", { requestId: "stream", encodedDataLength: 12 });
  cdp.emit("Network.dataReceived", { requestId: "stream", encodedDataLength: -1 });
  cdp.emit("Network.requestWillBeSent", { requestId: "redirect", redirectResponse: { encodedDataLength: 34 } });
  cdp.emit("Network.loadingFinished", { requestId: "redirect", encodedDataLength: 56 });
  const result = stop();
  assert.equal(result.eventSourceReportedEncodedBytes, 12);
  assert.equal(result.redirectReportedEncodedBytes, 34);
  assert.equal(result.completedEncodedBytes, 56);
  assert.equal(result.unfinished, 1);
});
