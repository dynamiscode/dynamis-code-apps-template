import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import { test } from "node:test";
import { observeWorkspaceAttribution } from "./ui-measurement-workspace-attribution.mjs";

test("classifies repeated requests without exposing request data", () => {
  const cdp = new EventEmitter();
  const stop = observeWorkspaceAttribution(cdp);
  const request = (requestId, type, method) => cdp.emit("Network.requestWillBeSent", {
    requestId, type, request: { method, url: "https://secret.test/private?token=hidden" },
  });
  request("one", "Document", "GET");
  cdp.emit("Network.responseReceived", { requestId: "one", response: { status: 200 } });
  request("two", "Fetch", "GET");
  cdp.emit("Network.responseReceived", { requestId: "two", response: { status: 200 } });
  request("three", "XHR", "POST");
  request("four", "XHR", "GET");
  const result = stop();
  assert.deepEqual(result.repeatedGets[0], { firstType: "Document", repeatedType: "Fetch", firstStatus: 200, repeatedStatus: 200 });
  assert.deepEqual(result.xhrMethods, { POST: 1, GET: 1 });
  assert.doesNotMatch(JSON.stringify(result), /secret|private|token|hidden/);
  assert.equal(cdp.listenerCount("Network.requestWillBeSent"), 0);
});

test("keeps missing statuses and unknown methods bounded", () => {
  const cdp = new EventEmitter();
  const stop = observeWorkspaceAttribution(cdp);
  cdp.emit("Network.requestWillBeSent", { requestId: "one", type: "private", request: { method: "GET", url: "secret" } });
  cdp.emit("Network.requestWillBeSent", { requestId: "two", type: "Fetch", request: { method: "GET", url: "secret" } });
  cdp.emit("Network.responseReceived", { requestId: "two", response: { status: "secret" } });
  cdp.emit("Network.requestWillBeSent", { requestId: "three", type: "XHR", request: { method: "secret", url: "secret" } });
  const result = stop();
  assert.deepEqual(result, { repeatedGets: [{ firstType: "Other", repeatedType: "Fetch", firstStatus: null, repeatedStatus: null }], xhrMethods: { other: 1 } });
  assert.doesNotMatch(JSON.stringify(result), /secret/);
});
