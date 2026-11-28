import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const componentsCode = fs.readFileSync(path.join(__dirname, "alpine-components.js"), "utf8");

// loadRealtimeList runs app.js against one live list element and records the
// EventSource listeners and the htmx refreshes the list triggers.
function loadRealtimeList(listID, events) {
  const sources = [];
  const documentListeners = {};
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
  let list = {
    id: listID,
    dataset: {
      events,
      liveConnecting: "connecting", liveConnected: "connected", liveUnavailable: "unavailable",
      liveReconnecting: "reconnecting", liveWaiting: "waiting"
    }
  };
  let status = { textContent: "" };
  let editing = false;
  let archiveOpen = false;
  const requests = [];
  let currentRequest;
  let swap;
  const ajaxCalls = [];
  const document = {
    querySelector: (selector) => selector === "#realtime-status" ? status
      : selector === "[data-client-archive-dialog]:modal" ? (archiveOpen ? {} : null)
      : selector.includes("[data-events]") || selector === `#${listID}` ? list : null,
    addEventListener: (name, listener) => (documentListeners[name] ||= []).push(listener)
  };
  const htmx = { config: {}, ajax: (...args) => {
    ajaxCalls.push(args);
    currentRequest = { target: list, request: { method: args[0].toLowerCase() }, response: { status: 200 } };
    requests.push(currentRequest);
    for (const listener of documentListeners["htmx:before:request"] || []) listener({ detail: { ctx: currentRequest } });
  } };
  const window = {
    EventSource: FakeEventSource,
    htmx,
    dynamisForms: { isEditing: () => editing, refreshDecision: (value) => value ? "defer" : "refresh" }
  };
  const location = { pathname: events.replace(/\/events$/, ""), search: "?search=Alpha&status=active" };
  const exportStyle = { module: true, exports: {} };
  new Function("module", "window", "document", "htmx", "EventSource", "location", componentsCode)(
    exportStyle, window, document, htmx, FakeEventSource, location
  );
  exportStyle.exports.wireRealtime();
  return {
    sources, ajaxCalls, location, requests,
    documentListeners,
    fireDocument(name, event = {}) { for (const listener of documentListeners[name] || []) listener(event); },
    setEditing(value) { editing = value; },
    setArchiveOpen(value) { archiveOpen = value; },
    replaceList(nextList, nextStatus) { list = nextList; status = nextStatus; },
    status() { return status; },
    beforeSwap(method = "get", ctx = currentRequest) {
      if (method !== "get") ctx = { target: list, request: { method } };
      ctx ||= { target: list, request: { method } };
      const event = { detail: { ctx }, defaultPrevented: false, prevented: false, preventDefault() { this.prevented = true; this.defaultPrevented = true; } };
      this.fireDocument("htmx:before:swap", event);
      return event;
    },
    afterSwap(ctx = currentRequest) { this.fireDocument("htmx:after:swap", { detail: { ctx } }); },
    afterRequest(ctx = currentRequest) { this.fireDocument("htmx:after:request", { detail: { ctx, failed: false } }); },
    finallyRequest(method = "get", ctx = currentRequest) { this.fireDocument("htmx:finally:request", { detail: { ctx: ctx || { target: list, request: { method } } } }); },
    replaceAfterSwap(nextList, nextStatus) { this.replaceList(nextList, nextStatus); this.afterSwap(); }
  };
}

function fire(source, name) {
  for (const listener of source.listeners[name] || []) listener({});
}

test("client list refreshes over SSE and preserves URL state", () => {
  const live = loadRealtimeList("client-list", "/workspaces/w1/clients/events");
  const { sources, ajaxCalls, location } = live;
  assert.equal(sources.length, 1);
  assert.equal(sources[0].url, "/workspaces/w1/clients/events");

  fire(sources[0], "client.changed");
  assert.deepEqual(ajaxCalls, [["GET", location.pathname + location.search, { target: "#client-list", swap: "outerHTML" }]]);

  fire(sources[0], "resync");
  live.afterSwap(live.requests[0]);
  assert.equal(ajaxCalls.length, 2);
  assert.deepEqual(ajaxCalls[1], ["GET", location.pathname + location.search, { target: "#client-list", swap: "outerHTML" }]);
});

test("client realtime refresh uses the current list and status after repeated swaps", () => {
  const live = loadRealtimeList("client-list", "/workspaces/w1/clients/events");
  const [source] = live.sources;
  fire(source, "open");
  const firstList = {
    id: "client-list",
    dataset: { liveWaiting: "waiting" }
  };
  const firstStatus = { textContent: "" };
  live.replaceList(firstList, firstStatus);
  fire(source, "client.changed");
  const firstRequest = live.requests[0];
  live.afterRequest(firstRequest);
  live.beforeSwap("get", firstRequest);
  assert.equal(live.ajaxCalls.length, 1);
  assert.equal(live.ajaxCalls[0][2].target, "#client-list");

  const secondList = {
    id: "client-list",
    dataset: { liveWaiting: "waiting" }
  };
  const secondStatus = { textContent: "" };
  live.replaceList(secondList, secondStatus);
  live.afterSwap(firstRequest);
  live.finallyRequest("get", firstRequest);
  assert.equal(secondStatus.textContent, "connected", "replacement restores current SSE state");
  fire(source, "error");
  assert.equal(secondStatus.textContent, "reconnecting");
  fire(source, "resync");
  live.replaceList({ id: "client-list", dataset: { liveWaiting: "waiting" } }, { textContent: "" });
  const secondRequest = live.requests[1];
  live.afterRequest(secondRequest);
  live.beforeSwap("get", secondRequest);
  live.afterSwap(secondRequest);
  live.finallyRequest("get", secondRequest);
  assert.equal(secondStatus.textContent, "reconnecting", "replacement keeps actual SSE status");
  assert.equal(live.ajaxCalls.length, 2);
});

test("delayed realtime response defers at swap when list became focused or dirty", () => {
  const live = loadRealtimeList("client-list", "/workspaces/w1/clients/events");
  const [source] = live.sources;
  fire(source, "client.changed");
  fire(source, "resync");
  assert.equal(live.ajaxCalls.length, 1, "overlapping events coalesce before response");

  const request = live.requests[0];
  live.afterRequest(request);
  live.setEditing(true);
  const swap = live.beforeSwap("get", request);
  assert.equal(swap.prevented, true, "late edit cancels replacement");
  assert.equal(live.status().textContent, "waiting");

  const mutation = live.beforeSwap("post");
  assert.equal(mutation.prevented, false, "list mutation response remains owned by route handler");
  live.finallyRequest("get", request);
  assert.equal(live.ajaxCalls.length, 1, "cancelled response does not trigger a timer retry");
});

test("archive confirmation defers list refreshes and late swaps until closed", async () => {
  const live = loadRealtimeList("client-list", "/workspaces/w1/clients/events");
  const [source] = live.sources;
  fire(source, "client.changed");
  live.setArchiveOpen(true);
  assert.equal(live.beforeSwap().prevented, true);
  live.finallyRequest();
  fire(source, "resync");
  assert.equal(live.ajaxCalls.length, 1);
  live.setArchiveOpen(false);
  live.fireDocument("close");
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(live.ajaxCalls.length, 2);
});

test("client refresh waits for focus and dirty input to clear", async () => {
  const live = loadRealtimeList("client-list", "/workspaces/w1/clients/events");
  const [source] = live.sources;
  const list = { id: "client-list", dataset: { liveWaiting: "waiting" } };
  live.replaceList(list, { textContent: "" });

  live.setEditing(true);
  fire(source, "client.changed");
  assert.equal(live.ajaxCalls.length, 0, "focused edits must defer refresh");
  live.setEditing(false);
  live.fireDocument("focusout");
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(live.ajaxCalls.length, 1, "blurred unchanged forms may refresh");
  live.afterSwap(live.requests[0]);

  live.setEditing(true);
  fire(source, "resync");
  live.fireDocument("focusout");
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(live.ajaxCalls.length, 1, "dirty forms must remain untouched after focus leaves");
  live.setEditing(false);
  live.fireDocument("change");
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(live.ajaxCalls.length, 2, "refresh resumes after dirty input clears");
});

test("refreshes arriving during an in-flight response schedule one follow-up", () => {
  const live = loadRealtimeList("client-list", "/workspaces/w1/clients/events");
  const [source] = live.sources;
  fire(source, "client.changed");
  fire(source, "resync");
  fire(source, "client.changed");
  assert.equal(live.ajaxCalls.length, 1, "overlapping events coalesce while request runs");
  const first = live.requests[0];
  live.afterRequest(first);
  live.beforeSwap("get", first);
  live.afterSwap(first);
  assert.equal(live.ajaxCalls.length, 2, "events during request coalesce to one next request");
  live.finallyRequest("get", first);
  assert.equal(live.ajaxCalls.length, 2, "old after:request cannot clear the newer request");
  live.afterRequest(live.requests[1]);
  live.beforeSwap("get", live.requests[1]);
  live.afterSwap(live.requests[1]);
  live.finallyRequest("get", live.requests[1]);
  assert.equal(live.ajaxCalls.length, 2);
});
