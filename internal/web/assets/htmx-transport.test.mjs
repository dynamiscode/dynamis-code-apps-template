import assert from "node:assert/strict";
import fs from "node:fs";
import test from "node:test";

const code = fs.readFileSync(new URL("./alpine-components.js", import.meta.url), "utf8");

test("HTMX rejects unknown, non-HTML and attachment response bodies before parsing", () => {
  const listeners = new Map();
  const document = { addEventListener: (name, listener) => listeners.set(name, listener) };
  new Function("window", "document", "location", code)({}, document, { origin: "https://app.example" });
  const beforeResponse = listeners.get("htmx:before:response");
  assert.equal(typeof beforeResponse, "function");

  for (const { status, type, disposition, url = "https://app.example/workspace", denied } of [
    { status: 200, type: "application/json", denied: true },
    { status: 200, type: "application/problem+json", denied: true },
    { status: 200, type: "text/event-stream", denied: true },
    { status: 200, type: "application/octet-stream", denied: true },
    { status: 200, type: "text/html", disposition: "Attachment; filename=a.txt", denied: true },
    { status: 200, type: "text/html", disposition: "unknown", denied: true },
    { status: 200, type: "text/html", url: "https://storage.example/file", denied: true },
    { status: 200, type: "text/html", url: "https://app.example.invalid/file", denied: true },
    { status: 200, type: "text/html", url: "", denied: true },
    { status: 200, type: null, denied: true },
    { status: 200, type: "text/html;", denied: true },
    { status: 200, type: "text/html; charset", denied: true },
    { status: 200, type: "text/html, application/json", denied: true },
    { status: 200, type: "text/html;charset=UTF-8", denied: false },
    { status: 200, type: 'text/html; charset="UTF-8"', denied: false },
    { status: 422, type: "text/HTML ; charset=utf-8", denied: false },
    { status: 204, type: null, denied: false },
    { status: 205, type: null, denied: false },
    { status: 304, type: null, denied: false },
  ]) {
    const values = new Map([["Content-Type", type], ["Content-Disposition", disposition ?? null]]);
    let cancelled = false;
    beforeResponse({
      detail: { ctx: { response: { status, headers: values, raw: { url } } } },
      preventDefault() { cancelled = true; },
    });
    assert.equal(cancelled, denied, `${status} ${type} ${disposition ?? ""}`);
  }
});
