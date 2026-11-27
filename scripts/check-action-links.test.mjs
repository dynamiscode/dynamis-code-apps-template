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

test('accepts conditional visual class variants', () => {
  assert.equal(violations('<a class={ "link", templ.KV("border-0", active) }>Open</a>').length, 0);
});

test('rejects attribute spreads on controls', () => {
  assert.equal(violations('<a class="btn" { attrs... }>Open</a>').length, 1);
});

test('rejects unknown classes after bounded helpers', () => {
  const source = 'func listViewOptionClass(applied bool) string {\n return "link"\n}\n<a class={ listViewOptionClass(true), extra }>View</a>';
  assert.equal(violations(source).length, 1);
});

test('rejects component attribute class overrides and unresolved attributes', () => {
  assert.equal(violations('@atoms.Button(atoms.ButtonProps{Class: "primary", Attrs: templ.Attributes{"class": "ghost"}})').length, 1);
  assert.equal(violations('@atoms.Button(atoms.ButtonProps{Class: "btn-primary", Attrs: attrs})').length, 1);
  assert.equal(violations('@atoms.Button(atoms.ButtonProps{Class: "btn-primary", Attrs: templ.Attributes{key: value}})').length, 1);
  assert.equal(violations('@atoms.Button(atoms.ButtonProps{Class: "btn-primary", Attrs: templ.Attributes{attrs...}})').length, 1);
  assert.equal(violations('@atoms.Button(atoms.ButtonProps{Class: "btn-primary", Attrs: templ.Attributes{"type": "submit", "data-value": format(nested(a, b), []string{"x", "y"})}})').length, 0);
});

test('rejects unsafe ConfirmButton attributes', () => {
  assert.equal(violations('@molecules.ConfirmButton("Delete", "Sure?", templ.Attributes{"class": "ghost"})').length, 1);
  assert.equal(violations('@molecules.ConfirmButton("Delete", "Sure?", templ.Attributes{"style": "color:red"})').length, 1);
  assert.equal(violations('@molecules.ConfirmButton("Delete", "Sure?", nil)').length, 0);
  assert.equal(violations('@molecules.ConfirmButton("Delete", "Sure?", templ.Attributes{"name": "action", "value": action})').length, 0);
  assert.equal(violations('@molecules.ConfirmButton("Delete", "Sure?", attrs)').length, 1);
  assert.equal(violations('@molecules.ConfirmButton("Delete", "Sure?", templ.Attributes{"data-value": unknown})').length, 0);
});

test('exception selectors do not match substrings of other attributes', () => {
  const exceptions = [{ path: 'demo.templ', selector: 'class="skip-link"', reason: 'skip control' }];
  assert.equal(violations('<a class="link" data-other-class="skip-link" { attrs... }>Open</a>', 'demo.templ', exceptions).length, 1);
});


test('checks controls without enforcing visual classes', () => {
  assert.equal(violations('<a class="link" href="/clients">Clients</a>').length, 0);
  assert.equal(violations('<a class="anything border-0 btn-ghost" href="/clients">Clients</a>').length, 0);
  assert.equal(violations('<a href={ templ.SafeURL(path) }>Open</a>').length, 1);
  assert.equal(violations('<a class="link" href="/clients" data-action-exception="demo">Clients</a>').length, 0);
});

test('checks component class inputs without enforcing visual classes', () => {
  assert.equal(violations('@atoms.Anchor(atoms.AnchorProps{Href: "/clients", Class: "link"})').length, 0);
  assert.equal(violations('@atoms.Button(atoms.ButtonProps{Class: "anything"})').length, 0);
  assert.equal(violations('@atoms.Button(atoms.ButtonProps{Class: unknown})').length, 1);
  assert.equal(violations('<button class={ templ.KV("anything", active) }>Save</button>').length, 0);
  assert.equal(violations('<a class={ "base", unknown }>Open</a>').length, 1);
});

test('dynamic class helper branches must each return literals', () => {
  const good = 'func listViewOptionClass(applied bool) string {\n if applied { return "selected" }\n return "link"\n}\n<a class={ listViewOptionClass(true) }>View</a>';
  assert.equal(violations(good).length, 0);
  assert.equal(violations(good.replace('return "link"', 'return other')).length, 1);
});

test('checks real shared helper and component calls', () => {
  const root = new URL('../', import.meta.url);
  const views = readFileSync(new URL('internal/web/components/organisms/list_views.templ', root), 'utf8');
  assert.equal(violations(views).length, 0);
  assert.equal(violations(views.replace('return "list-view-option border border-base-300 rounded-field"', 'return "link"')).length, 0);
  const buttons = readFileSync(new URL('internal/web/settings_pages.templ', root), 'utf8');
  assert.equal(violations(buttons).length, 0);
  assert.equal(violations(buttons.replace('Class: "btn btn-primary"', 'Class: "link"')).length, 0);
});

test('repository demo and accessibility exceptions accept their real controls', () => {
  const root = new URL('../', import.meta.url);
  const exceptions = JSON.parse(readFileSync(new URL('scripts/action-exceptions.json', root), 'utf8'));
  for (const path of ['internal/web/components/atoms/link.templ', 'internal/web/components/molecules/skip_link.templ']) {
    const source = readFileSync(new URL(path, root), 'utf8');
    assert.equal(violations(source, path).length, path.endsWith('/link.templ') ? 1 : 0);
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
