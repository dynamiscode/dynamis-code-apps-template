import { test } from 'node:test';
import { strict as assert } from 'node:assert';
import { readFileSync } from 'node:fs';
import { violations } from './check-action-links.mjs';

test('rejects an unbounded navigation link', () => {
  assert.equal(violations('<a class="link" href="/clients">Clients</a>').length, 1);
});

test('checks actual templ controls and accepts bounded roles', () => {
  assert.equal(violations('<a class="link rounded-field border border-base-300 px-1" href="/clients">Clients</a>').length, 0);
  assert.equal(violations('<button class={ "btn", templ.KV("btn-primary", enabled) }>Save</button>').length, 0);
  assert.equal(violations('<button class="btn btn-ghost">Save</button>').length, 1);
  assert.equal(violations('<a href={ templ.SafeURL(path) }>Open</a>').length, 1);
  assert.equal(violations('<a class="link" href="/clients" data-action-exception="demo">Clients</a>').length, 1);
});

test('repository demo and accessibility exceptions accept their real controls', () => {
  const root = new URL('../', import.meta.url);
  const exceptions = JSON.parse(readFileSync(new URL('scripts/action-exceptions.json', root), 'utf8'));
  for (const path of ['internal/web/components/atoms/link.templ', 'internal/web/components/molecules/skip_link.templ']) {
    const source = readFileSync(new URL(path, root), 'utf8');
    assert.equal(violations(source, path).length, 1);
    assert.equal(violations(source, path, exceptions).length, 0);
  }
});

test('exceptions apply only to their documented path and control', () => {
  const exceptions = [{ path: 'demo.templ', selector: 'href="#demo"', reason: 'catalog variant' },
    { path: 'skip.templ', selector: 'class="skip-link"', reason: 'accessible skip link' }];
  assert.equal(violations('<a href="#demo">Demo</a>', 'demo.templ', exceptions).length, 0);
  assert.equal(violations('<a class="skip-link" href="#main">Skip</a>', 'skip.templ', exceptions).length, 0);
  assert.equal(violations('<a href="#demo">Demo</a>', 'other.templ', exceptions).length, 1);
  assert.equal(violations('<a href="#other">Other</a>', 'demo.templ', exceptions).length, 1);
});
