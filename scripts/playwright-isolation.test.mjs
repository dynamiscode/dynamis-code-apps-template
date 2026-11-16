import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { mkdtemp, rm, stat } from "node:fs/promises";
import http from "node:http";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import { promisify } from "node:util";
import { test } from "node:test";

const run = promisify(execFile);

test("fixture ignores inherited app configuration and uses disposable storage", { timeout: 180_000 }, async () => {
  const work = await mkdtemp(path.join(os.tmpdir(), "playwright-isolation-test-"));
  let exports = 0;
  const collector = http.createServer((request, response) => {
    exports++;
    response.end();
  });
  const socket = net.createServer();
  try {
    await new Promise((resolve) => collector.listen(0, "127.0.0.1", resolve));
    await new Promise((resolve) => socket.listen(0, "127.0.0.1", resolve));
    const port = socket.address().port;
    await new Promise((resolve, reject) => socket.close((error) => error ? reject(error) : resolve()));
    const externalStorage = path.join(work, "outside-storage");
    const { stdout } = await run("./scripts/playwright-smoke.sh", [], {
      timeout: 170_000,
      env: { ...process.env, PLAYWRIGHT_PORT: String(port), UI_MEASURE: "1",
        OTEL_EXPORTER_OTLP_ENDPOINT: `http://127.0.0.1:${collector.address().port}`,
        OTEL_METRIC_EXPORT_INTERVAL: "5s", STORAGE_DRIVER: "s3",
        STORAGE_LOCAL_PATH: externalStorage, OIDC_ENABLED: "invalid",
        SMTP_HOST: "invalid", HTTP_SECURE: "invalid" },
    });
    const result = JSON.parse(stdout);
    assert.deepEqual(result.results.map((entry) => entry.cache), ["cold", "warm"]);
    assert.equal(exports, 0, "fixture exported telemetry");
    await assert.rejects(stat(externalStorage), { code: "ENOENT" });
  } finally {
    await new Promise((resolve) => collector.close(resolve));
    await rm(work, { recursive: true, force: true });
  }
});
