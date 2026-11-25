import test from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdtempSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { verifyReviewedScripts } from './check-js-actions.mjs';

test('reviewed browser actions are bound to exact bytes', t => {
  const dir = mkdtempSync(join(tmpdir(), 'reviewed-browser-scripts-'));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  const path = join(dir, 'actions.js');
  const original = 'createAction("button", "btn btn-outline");';
  const reviewed = { 'actions.js': {
    bytes: Buffer.byteLength(original),
    sha256: createHash('sha256').update(original).digest('hex'),
    reason: 'Test fixture with native outlined button',
  } };
  writeFileSync(path, original);
  assert.deepEqual(verifyReviewedScripts(dir, reviewed), []);
  for (const changed of [
    original.replace('btn-outline', 'btn-ghost  '), // Same-length hash mutation.
    original + 'document.createElement(/* action */ "button");',
    original + 'document.createElementNS("http://www.w3.org/1999/xhtml", "button");',
    original + 'b.className = "btn btn-ghost";',
    original + 'createAction("a", "border border-black/0");',
    original + 'createAction("a", "border [border-width:0]");',
  ]) {
    writeFileSync(path, changed);
    assert.deepEqual(verifyReviewedScripts(dir, reviewed), ['actions.js: browser script differs from the reviewed bytes']);
  }
  writeFileSync(path, original);
  writeFileSync(join(dir, 'unreviewed.min.js'), 'document.createElement("button");');
  assert.deepEqual(verifyReviewedScripts(dir, reviewed), ['unreviewed.min.js: missing a valid reviewed browser script entry']);
  rmSync(join(dir, 'unreviewed.min.js'));
  renameSync(path, join(dir, 'renamed.js'));
  assert.deepEqual(verifyReviewedScripts(dir, reviewed), [
    'actions.js: stale reviewed browser script entry',
    'renamed.js: missing a valid reviewed browser script entry',
  ]);
  for (const name of ['preview.html', 'icon.svg']) writeFileSync(join(dir, name), '<script>makeAction()</script>');
  assert.ok(verifyReviewedScripts(dir, reviewed).some(item => item.startsWith('icon.svg:')));
  assert.ok(verifyReviewedScripts(dir, reviewed).some(item => item.startsWith('preview.html:')));
});
