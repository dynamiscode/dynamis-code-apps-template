import assert from "node:assert/strict";
import { test } from "node:test";
import { recordRequest } from "./ui-measurement-admin-attribution.mjs";

test("attribution exposes only allowlisted local assets and aggregates other requests", () => {
  const counts = {};
  const base = "http://127.0.0.1:58096";
  for (const [url, type] of [
    [`${base}/assets/forms.js?secret=value`, "script"],
    [`${base}/assets/forms.js`, "script"],
    [`${base}/workspaces/private?token=hidden`, "document"],
    [`${base}/assets/private-secret.js`, "script"],
    ["https://example.invalid/private-token", "script"],
  ]) recordRequest(counts, url, base, type);
  assert.deepEqual(counts, { "asset:forms.js": 2, document: 1, other: 1, external: 1 });
  assert.doesNotMatch(JSON.stringify(counts), /private|secret|token|hidden|example/);
});
