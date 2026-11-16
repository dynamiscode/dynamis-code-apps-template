import test from "node:test";
import assert from "node:assert/strict";
import items from "./items.js";

const row = () => ({ row: true });
const form = (container, dirty = false, submitting = false) => ({
	dataset: { dirty: dirty ? "true" : "", submitting: submitting ? "true" : "" },
	closest: () => container
});

test("item list swap preserves a different dirty row", () => {
	const source = form(row());
	const dirtyRow = row();
	const dirty = form(dirtyRow, true);
	assert.deepEqual(items.itemSwapPlan([source, dirty], source, false, null), { defer: false, rows: [dirtyRow] });
});

test("item list swap preserves a focused row and defers focused controls outside rows", () => {
	const source = form(row());
	const focusedRow = row();
	const focused = form(focusedRow);
	assert.deepEqual(items.itemSwapPlan([source, focused], source, false, focused), { defer: false, rows: [focusedRow] });
	assert.deepEqual(items.itemSwapPlan([source, form(null)], source, false, form(null)), { defer: true, rows: [] });
});

test("item edit validation keeps submitted values in the current form", () => {
	const source = form(row(), true, true);
	assert.deepEqual(items.itemSwapPlan([source], source, true, null), { defer: true, rows: [] });
	assert.deepEqual(items.itemSwapPlan([source], source, false, null), { defer: false, rows: [] });
});

test("successful create swap does not preserve submitted create form", () => {
	const create = form(null, false, true);
	assert.deepEqual(items.itemSwapPlan([create], create, false, create), { defer: false, rows: [] });
	assert.deepEqual(items.itemSwapPlan([create], create, false, null), { defer: false, rows: [] });
});

test("a rendered item error restores an optimistically hidden delete row", () => {
	const hiddenRow = { dataset: { optimistic: "hidden" } };
	const source = { _optimisticRow: hiddenRow };
	assert.equal(items.restoreFailedOptimisticRow(source), true);
	assert.equal(hiddenRow.dataset.optimistic, undefined);
	assert.equal(source._optimisticRow, null);
	assert.equal(items.restoreFailedOptimisticRow(source), false);
});

test("unsaved filter values defer a whole-list swap", () => {
	const source = form(row());
	const filter = form(null, true);
	assert.deepEqual(items.itemSwapPlan([source, filter], source, false, null), { defer: true, rows: [] });
});
