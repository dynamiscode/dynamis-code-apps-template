import { test } from 'node:test';
import { strict as assert } from 'node:assert';
import { violations } from './check-action-links.mjs';

test('rejects an unbounded navigation link', () => {
  assert.equal(violations('<a class="link" href="/clients">Clients</a>').length, 1);
});

test('accepts bounded links and narrowly marked examples', () => {
  assert.equal(violations('<a class="link rounded-field border border-base-300 px-1" href="/clients">Clients</a>').length, 0);
  assert.equal(violations('<a class="link" href="#demo" data-action-exception="demo">Link variant</a>').length, 0);
  assert.equal(violations('<a class="link" href="#main" data-action-exception="accessibility">Skip</a>').length, 0);
  assert.equal(violations('<a class="link" href="/clients" data-action-exception="anything">Clients</a>').length, 1);
});
