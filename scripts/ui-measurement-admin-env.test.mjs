import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { test } from "node:test";

test("measurement application environment excludes inherited external settings", () => {
  const result = spawnSync("sh", ["scripts/ui-measurement-admin-env.sh", "/tmp/isolated-fixture", "58096", "env"], {
    encoding: "utf8",
    env: { ...process.env, OTEL_EXPORTER_OTLP_ENDPOINT: "https://example.invalid",
      STORAGE_DRIVER: "s3", STORAGE_LOCAL_PATH: "/outside", SMTP_HOST: "example.invalid",
      CLOUDFLARE_EMAIL_ACCOUNT_ID: "example" },
  });
  assert.equal(result.status, 0);
  const entries = Object.fromEntries(result.stdout.trim().split("\n").map((line) => {
    const index = line.indexOf("=");
    return [line.slice(0, index), line.slice(index + 1)];
  }));
  assert.equal(entries.STORAGE_DRIVER, "local");
  assert.equal(entries.STORAGE_LOCAL_PATH, "/tmp/isolated-fixture/files");
  assert.equal(entries.SQLITE_PATH, "/tmp/isolated-fixture/app.db");
  assert.equal(entries.OTEL_SDK_DISABLED, "true");
  for (const name of ["OTEL_EXPORTER_OTLP_ENDPOINT", "SMTP_HOST", "CLOUDFLARE_EMAIL_ACCOUNT_ID"]) {
    assert.equal(entries[name], undefined);
  }
});
