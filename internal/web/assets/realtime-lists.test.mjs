import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const appJsCode = fs.readFileSync(path.join(__dirname, "app.js"), "utf8");

// loadRealtimeList runs app.js against one live list element and records the
// EventSource listeners and the htmx refreshes the list triggers.
function loadRealtimeList(listID, events) {
  const sources = [];
  class FakeEventSource {
    constructor(url) {
      this.url = url;
      this.listeners = {};
      sources.push(this);
    }
    addEventListener(name, listener) {
      (this.listeners[name] ||= []).push(listener);
    }
    close() {}
  }
  FakeEventSource.CLOSED = 2;
  const list = {
    id: listID,
    dataset: {
      events,
      liveConnecting: "connecting", liveConnected: "connected", liveUnavailable: "unavailable",
      liveReconnecting: "reconnecting", liveWaiting: "waiting"
    }
  };
  const ajaxCalls = [];
  const htmx = { config: {}, ajax: (...args) => ajaxCalls.push(args) };
  const document = {
    querySelector: (selector) => (selector.includes("[data-events]") || selector === `#${listID}` ? list : null),
    addEventListener: () => {}
  };
  const window = { EventSource: FakeEventSource, htmx };
  const location = { pathname: events.replace(/\/events$/, ""), search: "?search=Alpha&status=active" };
  new Function("window", "document", "htmx", "EventSource", "location", appJsCode)(
    window, document, htmx, FakeEventSource, location
  );
  return { sources, ajaxCalls, location };
}

function fire(source, name) {
  for (const listener of source.listeners[name] || []) listener({});
}

test("client list refreshes over SSE and preserves URL state", () => {
  const { sources, ajaxCalls, location } = loadRealtimeList("client-list", "/workspaces/w1/clients/events");
  assert.equal(sources.length, 1);
  assert.equal(sources[0].url, "/workspaces/w1/clients/events");

  fire(sources[0], "client.changed");
  assert.deepEqual(ajaxCalls, [["GET", location.pathname + location.search, { target: "#client-list", swap: "outerHTML" }]]);

  fire(sources[0], "resync");
  assert.equal(ajaxCalls.length, 2);
  assert.deepEqual(ajaxCalls[1], ["GET", location.pathname + location.search, { target: "#client-list", swap: "outerHTML" }]);
});

test("item list still refreshes on item.changed and ignores client events", () => {
  const { sources, ajaxCalls } = loadRealtimeList("item-list", "/workspaces/w1/items/events");
  fire(sources[0], "client.changed");
  assert.equal(ajaxCalls.length, 0, "a client event must not refresh the item list");

  fire(sources[0], "item.changed");
  assert.equal(ajaxCalls.length, 1);
  assert.equal(ajaxCalls[0][2].target, "#item-list");
});
