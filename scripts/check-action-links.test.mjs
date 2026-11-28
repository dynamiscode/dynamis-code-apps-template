import { test } from 'node:test';
import { strict as assert } from 'node:assert';
import { readFileSync } from 'node:fs';
import { violations } from './check-action-links.mjs';

test('rejects aliased component calls', () => {
  assert.equal(violations('import ui "example/atoms"\n@ui.Anchor(ui.AnchorProps{Class: "link"})').length, 1);
});

test('rejects prebuilt component props', () => {
  assert.equal(violations('@atoms.Anchor(props)').length, 1);
});

test('rejects unknown dynamic class expressions', () => {
  assert.equal(violations('<a class={ "btn", extra }>Open</a>').length, 1);
  assert.equal(violations('<a class={ "btn", unknown() }>Open</a>').length, 1);
  assert.equal(violations('<a class={ actionattrs.LinkClass(unknown()) }>Open</a>').length, 1);
  assert.equal(violations('<a class={ "btn", templ.KV(extra, active) }>Open</a>').length, 1);
});

test('rejects conditional removal of a boundary', () => {
  assert.equal(violations('<a class={ "btn", templ.KV("border-0", active) }>Open</a>').length, 1);
});

test('rejects attribute spreads on controls', () => {
  assert.equal(violations('<a class="btn" { attrs... }>Open</a>').length, 1);
});

test('rejects unsafe extra classes after bounded helpers', () => {
  const source = 'func listViewOptionClass(applied bool) string {\n return "btn"\n}\n<a class={ listViewOptionClass(true), "border-0" }>View</a>';
  assert.equal(violations(source).length, 1);
});

test('rejects component attribute class overrides and unresolved attributes', () => {
  assert.equal(violations('@atoms.Button(atoms.ButtonProps{Class: "btn-primary", Attrs: templ.Attributes{"class": "btn-ghost"}})').length, 1);
  assert.equal(violations('@atoms.Button(atoms.ButtonProps{Class: "btn-primary", Attrs: attrs})').length, 1);
  assert.equal(violations('@atoms.Button(atoms.ButtonProps{Class: "btn-primary", Attrs: templ.Attributes{key: value}})').length, 1);
  assert.equal(violations('@atoms.Button(atoms.ButtonProps{Class: "btn-primary", Attrs: templ.Attributes{attrs...}})').length, 1);
  assert.equal(violations('@atoms.Button(atoms.ButtonProps{Class: "btn-primary", Attrs: templ.Attributes{"type": "submit", "data-value": format(nested(a, b), []string{"x", "y"})}})').length, 0);
});

test('rejects unsafe ConfirmButton attributes', () => {
  assert.equal(violations('@molecules.ConfirmButton("Delete", "Sure?", templ.Attributes{"class": "btn-ghost"})').length, 1);
  assert.equal(violations('@molecules.ConfirmButton("Delete", "Sure?", nil)').length, 0);
  assert.equal(violations('@molecules.ConfirmButton("Delete", "Sure?", templ.Attributes{"name": "action", "value": action})').length, 0);
  assert.equal(violations('@molecules.ConfirmButton("Delete", "Sure?", templ.Attributes{"class": "btn btn-error danger"})').length, 0);
  assert.equal(violations('@molecules.ConfirmButton("Delete", "Sure?", attrs)').length, 1);
  assert.equal(violations('@molecules.ConfirmButton("Delete", "Sure?", templ.Attributes{"class": unknown})').length, 1);
});

test('exception selectors do not match substrings of other attributes', () => {
  const exceptions = [{ path: 'demo.templ', selector: 'class="skip-link"', reason: 'skip control' }];
  assert.equal(violations('<a data-other-class="skip-link">Open</a>', 'demo.templ', exceptions).length, 1);
});

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

test('rejects unsafe component props and conditional boundary classes', () => {
  assert.equal(violations('@atoms.Anchor(atoms.AnchorProps{Href: "/clients", Class: "link"})').length, 1);
  assert.equal(violations('@atoms.Anchor(atoms.AnchorProps{Href: "/clients", Class: "btn btn-primary"})').length, 0);
  assert.equal(violations('@atoms.Button(atoms.ButtonProps{Class: "btn-ghost"})').length, 1);
  assert.equal(violations('@atoms.Button(atoms.ButtonProps{Class: "btn-primary"})').length, 0);
  assert.equal(violations('<button class={ templ.KV("btn", active) }>Save</button>').length, 1);
  assert.equal(violations('<a class={ "btn", templ.KV("btn-ghost", active) }>Open</a>').length, 1);
});

test('dynamic class helper branches must each return bounded classes', () => {
  const good = 'func listViewOptionClass(applied bool) string {\n if applied { return "btn btn-active" }\n return "btn"\n}\n<a class={ listViewOptionClass(true) }>View</a>';
  assert.equal(violations(good).length, 0);
  assert.equal(violations(good.replace('return "btn"', 'return "link"')).length, 1);
  assert.equal(violations(good.replace('return "btn"', 'return other')).length, 1);
});

test('checks real shared helper and component calls', () => {
  const root = new URL('../', import.meta.url);
  const views = readFileSync(new URL('internal/web/components/organisms/list_views.templ', root), 'utf8');
  assert.equal(violations(views).length, 0);
  assert.equal(violations(views.replace('return "list-view-option border border-base-300 rounded-field"', 'return "link"')).length, 2);
  const buttons = readFileSync(new URL('internal/web/settings_pages.templ', root), 'utf8');
  assert.equal(violations(buttons).length, 0);
  const unbounded = buttons.replace('class="btn btn-primary"', 'class="link"');
  assert.notEqual(unbounded, buttons);
  assert.equal(violations(unbounded).length, 1);
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
