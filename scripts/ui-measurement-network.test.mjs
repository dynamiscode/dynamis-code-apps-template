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
