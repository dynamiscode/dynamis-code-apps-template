//go:build browser

package browsertest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/input"
)

func stageCardPoint(t *testing.T, tab context.Context, name string, fraction float64) []float64 {
	t.Helper()
	return eval[[]float64](t, tab, fmt.Sprintf(`(() => {
		const card = [...document.querySelectorAll('[data-stage-card]')].find(el => el.textContent.includes(%s));
		const rect = card.getBoundingClientRect(), handle = card.querySelector('.list-column-handle').getBoundingClientRect();
		return [handle.left + handle.width / 2, rect.top + rect.height * %f];
	})()`, jsString(name), fraction))
}

func dragStage(t *testing.T, tab context.Context, source []float64, targetPoint func() []float64, gapPosition string) {
	t.Helper()
	columnMouse(t, tab, input.DispatchMouseEventTypeMouseMoved, source, false)
	columnMouse(t, tab, input.DispatchMouseEventTypeMousePressed, source, true)
	lifted := []float64{source[0], source[1] + 10}
	moveColumnPointer(t, tab, source, lifted, 5)
	eventually(t, tab, `Boolean(document.querySelector('.stage-drag-gap'))`, "stage drag did not show an insertion gap")
	target := targetPoint()
	point := lifted
	for i := 1; i <= 20; i++ {
		fraction := float64(i) / 20
		point = []float64{lifted[0] + (target[0]-lifted[0])*fraction, lifted[1] + (target[1]-lifted[1])*fraction}
		columnMouse(t, tab, input.DispatchMouseEventTypeMouseMoved, point, true)
		if eval[bool](t, tab, gapPosition) {
			break
		}
	}
	eventually(t, tab, gapPosition, "stage drag gap did not reach the target")
	columnMouse(t, tab, input.DispatchMouseEventTypeMouseReleased, point, false)
}

func TestStageDragReorderAndGroupConfirmation(t *testing.T) {
	tab := newTab(t, 1280, 2400, "")
	signIn(t, tab)
	workspace := openWorkspaceSettings(t, tab, "/engagement-stages")
	suffix := fmt.Sprint(time.Now().UnixNano())
	first, second := "Drag first "+suffix, "Drag second "+suffix
	createStage := func(name string) {
		t.Helper()
		navigate(t, tab, workspace+"/engagement-stages")
		click(t, tab, "#stages details > summary")
		fill(t, tab, "#stage-name", name)
		click(t, tab, `form:has(#stage-name) button[type=submit]`)
		eventually(t, tab, fmt.Sprintf(`Boolean(document.querySelector('[data-stage-group="open"]')?.textContent.includes(%s))`, jsString(name)), "stage was not created")
	}
	createStage(first)
	createStage(second)

	navigate(t, tab, workspace+"/clients/new")
	fill(t, tab, "#client-display-name", "Drag client "+suffix)
	click(t, tab, `form:has(#client-display-name) button[type=submit]`)
	eventually(t, tab, `/\/clients\/[^/?]+$/.test(location.pathname)`, "client was not created")
	click(t, tab, `a[href*="/engagements/new?client="]`)
	waitVisible(t, tab, "#engagement-name")
	fill(t, tab, "#engagement-name", "Drag engagement "+suffix)
	selectEngagementStage(t, tab, second)
	click(t, tab, `form:has(#engagement-name) .modal-action button[type=submit]`)
	eventually(t, tab, `/\/engagements\/[^/?]+$/.test(location.pathname)`, "engagement was not created")

	navigate(t, tab, workspace+"/engagement-stages")
	eventually(t, tab, `Boolean(document.querySelector('[data-stage-order-bound]'))`, "stage drag was not initialized")
	source := stageCardPoint(t, tab, second, 0.5)
	columnMouse(t, tab, input.DispatchMouseEventTypeMouseMoved, source, false)
	columnMouse(t, tab, input.DispatchMouseEventTypeMousePressed, source, true)
	lifted := []float64{source[0], source[1] + 10}
	moveColumnPointer(t, tab, source, lifted, 5)
	eventually(t, tab, `Boolean(document.querySelector('.stage-drag-gap'))`, "stage drag did not start")
	target := eval[[]float64](t, tab, `(() => { const r = document.querySelector('[data-stage-group="won"]').getBoundingClientRect(); return [r.left + r.width / 2, r.top + r.height / 2]; })()`)
	moveColumnPointer(t, tab, lifted, target, 10)
	eventually(t, tab, `Boolean(document.querySelector('[data-stage-group="won"].is-stage-drop-target'))`, "drag target was not highlighted")
	if !eval[bool](t, tab, `(() => {
		const groups = [...document.querySelectorAll('[data-stage-group]')];
		const empty = groups.find(group => !group.querySelector('[data-stage-card]:not(.is-dragging)'));
		return groups.length === 3 && groups.every(group => getComputedStyle(group).outlineStyle !== 'none') &&
			empty && getComputedStyle(empty, '::after').content === '"Drop here"' &&
			!!document.querySelector('[data-stage-group].is-stage-drop-target');
	})()`) {
		t.Fatal("drag must highlight groups, identify the target, and label an empty group")
	}
	call(t, tab, input.CancelDragging, cdp.Empty{})
	columnMouse(t, tab, input.DispatchMouseEventTypeMouseReleased, target, false)
	eventually(t, tab, `!document.querySelector('.stage-drag-gap, .stage-drag-preview, .is-stage-dragging, .is-stage-drop-target') && [...document.querySelectorAll('[data-stage-group]')].every(group => getComputedStyle(group, '::after').content === 'none')`, "cancel left drag guidance visible")
	eventually(t, tab, fmt.Sprintf(`document.querySelector('[data-stage-group="open"]')?.textContent.includes(%s)`, jsString(second)), "cancel changed the stage group")
	dragStage(t, tab, stageCardPoint(t, tab, second, 0.5), func() []float64 {
		return eval[[]float64](t, tab, fmt.Sprintf(`(() => { const card = [...document.querySelectorAll('[data-stage-card]')].find(el => el.textContent.includes(%s)), r = card.getBoundingClientRect(); return [r.left + 25, r.top + r.height / 2 - 15]; })()`, jsString(first)))
	}, fmt.Sprintf(`Boolean(document.querySelector('.stage-drag-gap')?.nextElementSibling?.textContent.includes(%s))`, jsString(first)))
	order := fmt.Sprintf(`(() => { const cards = [...document.querySelectorAll('[data-stage-group="open"] [data-stage-card]')]; return cards.findIndex(c => c.textContent.includes(%s)) < cards.findIndex(c => c.textContent.includes(%s)) && cards.some(c => c.textContent.includes(%s)); })()`, jsString(second), jsString(first), jsString(second))
	eventually(t, tab, order, "within-group drop did not save the order")
	navigate(t, tab, workspace+"/engagement-stages")
	eventually(t, tab, order, "stage order did not persist")

	moveToWon := func() {
		t.Helper()
		eventually(t, tab, `Boolean(document.querySelector('[data-stage-order-bound]'))`, "stage drag was not initialized")
		dragStage(t, tab, stageCardPoint(t, tab, second, 0.5), func() []float64 {
			return eval[[]float64](t, tab, `(() => { const group = document.querySelector('[data-stage-group="won"]'), r = group.getBoundingClientRect(); return [r.left + r.width / 2, r.bottom - 3]; })()`)
		}, `document.querySelector('.stage-drag-gap')?.parentElement?.dataset.stageGroup === 'won'`)
		waitVisible(t, tab, `dialog[aria-labelledby="confirm-stage-move-title"]`)
		if !eval[bool](t, tab, fmt.Sprintf(`(() => { const values = [...document.querySelectorAll('dialog[aria-labelledby="confirm-stage-move-title"] dd')].map(el => el.textContent.trim()); return JSON.stringify(values) === JSON.stringify([%s, 'Open', 'Won', '1']); })()`, jsString(second))) {
			t.Fatal("confirmation must show stage, old and new groups, and one affected engagement")
		}
	}
	moveToWon()
	click(t, tab, `dialog[aria-labelledby="confirm-stage-move-title"] a`)
	eventually(t, tab, order, "cancelling changed stage order")
	moveToWon()
	other := newTab(t, 1280, 2400, "")
	signIn(t, other)
	navigate(t, other, workspace+"/engagement-stages")
	click(t, other, "#stages details > summary")
	fill(t, other, "#stage-name", "Concurrent stage "+suffix)
	click(t, other, `form:has(#stage-name) button[type=submit]`)
	eventually(t, other, fmt.Sprintf(`Boolean(document.querySelector('[data-stage-group="open"]')?.textContent.includes(%s))`, jsString("Concurrent stage "+suffix)), "concurrent stage was not created")
	click(t, tab, `dialog input[name="confirm_move"]`)
	click(t, tab, `dialog form button[type="submit"]`)
	eventually(t, tab, `document.body.textContent.includes('Stages or the number of affected engagements changed')`, "stale confirmation was not rejected")
	eventually(t, tab, order, "stale confirmation changed the stage order")
	moveToWon()
	click(t, tab, `dialog input[name="confirm_move"]`)
	click(t, tab, `dialog form button[type="submit"]`)
	eventually(t, tab, fmt.Sprintf(`Boolean(document.querySelector('[data-stage-group="won"]')?.textContent.includes(%s))`, jsString(second)), "confirmed move did not place stage in Won")
	navigate(t, tab, workspace+"/engagement-stages")
	eventually(t, tab, fmt.Sprintf(`Boolean(document.querySelector('[data-stage-group="won"]')?.textContent.includes(%s))`, jsString(second)), "confirmed group change did not persist")
}

func TestStageMoveMenuOnTouch(t *testing.T) {
	tab := newTab(t, 390, 844, "")
	call(t, tab, emulation.SetTouchEmulationEnabled, emulation.SetTouchEmulationEnabledParams{Enabled: true})
	signIn(t, tab)
	openWorkspaceSettings(t, tab, "/engagement-stages")
	name := "Touch stage " + fmt.Sprint(time.Now().UnixNano())
	click(t, tab, "#stages details > summary")
	fill(t, tab, "#stage-name", name)
	click(t, tab, `form:has(#stage-name) button[type=submit]`)
	cardSelector := `[data-stage-card]:has(strong)`
	eventually(t, tab, fmt.Sprintf(`Array.from(document.querySelectorAll(%s)).some(card => card.querySelector('strong')?.textContent === %s)`, jsString(cardSelector), jsString(name)), "stage was not created")
	id := eval[string](t, tab, fmt.Sprintf(`Array.from(document.querySelectorAll(%s)).find(card => card.querySelector('strong')?.textContent === %s).dataset.stageId`, jsString(cardSelector), jsString(name)))
	card := fmt.Sprintf(`[data-stage-card][data-stage-id="%s"]`, id)
	if !eval[bool](t, tab, fmt.Sprintf(`(() => {
		const label = document.querySelector(%s).querySelector('.stage-move-menu-label');
		return matchMedia('(pointer: coarse)').matches && label && label.getBoundingClientRect().width > 0 && label.textContent.trim() === 'Move stage';
	})()`, jsString(card))) {
		t.Fatal("touch users must see the Move stage menu")
	}
	click(t, tab, card+` summary.list-row-menu-button`)
	waitVisible(t, tab, card+` button[name="kind"][value="won"]`)
	click(t, tab, card+` button[name="kind"][value="won"]`)
	waitVisible(t, tab, `dialog[aria-labelledby="confirm-stage-move-title"]`)
	click(t, tab, `dialog[aria-labelledby="confirm-stage-move-title"] a`)
	eventually(t, tab, fmt.Sprintf(`document.querySelector('[data-stage-group="open"]')?.textContent.includes(%s)`, jsString(name)), "cancelling touch move changed the group")
}
