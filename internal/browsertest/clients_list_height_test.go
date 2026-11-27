//go:build browser

package browsertest

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestClientsListHeightAndScrolling(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	path := eval[string](t, tab, `location.pathname`)
	prefix := fmt.Sprintf("List height %d", time.Now().UnixNano())
	create := func(n int) {
		navigate(t, tab, path+"/new")
		fillByLabel(t, tab, "Display name", fmt.Sprintf("%s %02d", prefix, n))
		clickByText(t, tab, "Create client")
		eventually(t, tab, `/\/clients\/[^/?]+$/.test(location.pathname)`, "client creation did not open detail")
	}
	openResults := func() {
		navigate(t, tab, path+"?"+url.Values{"q": {prefix}}.Encode())
		waitVisible(t, tab, ".list-view-table tbody tr")
	}

	create(0)
	openResults()
	if !eval[bool](t, tab, `(() => {
		const table = document.querySelector('.list-view-table');
		const footer = document.querySelector('.list-footer');
		const gap = footer.getBoundingClientRect().top - table.getBoundingClientRect().bottom;
		return table.scrollHeight <= table.clientHeight + 1 && gap >= 0 && gap < 20;
	})()`) {
		t.Fatal("short list scrolls or leaves excess space before its footer")
	}

	for i := 1; i < 12; i++ {
		create(i)
	}
	run(t, tab, chromedp.EmulateViewport(1280, 600))
	openResults()
	if !eval[bool](t, tab, `(() => {
		const table = document.querySelector('.list-view-table');
		const footer = document.querySelector('.list-footer').getBoundingClientRect();
		return table.scrollHeight > table.clientHeight && footer.bottom <= innerHeight;
	})()`) {
		t.Fatal("long list does not scroll within the viewport with its footer visible")
	}
	if !eval[bool](t, tab, `(() => {
		const table = document.querySelector('.list-view-table');
		const header = table.querySelector('thead th');
		const before = header.getBoundingClientRect().top;
		const last = table.querySelector('tbody tr:last-child a');
		last.focus();
		const row = last.getBoundingClientRect();
		const bounds = table.getBoundingClientRect();
		return document.activeElement === last && table.scrollTop > 0 &&
			Math.abs(header.getBoundingClientRect().top - before) < 2 &&
			row.bottom <= bounds.bottom + 1;
	})()`) {
		t.Fatal("keyboard focus cannot reach the last row while keeping the header sticky")
	}

	run(t, tab, chromedp.EmulateViewport(390, 600))
	if !eval[bool](t, tab, `(() => {
		const cards = document.querySelector('.list-view-cards');
		const footer = document.querySelector('.list-footer');
		footer.scrollIntoView();
		const rect = footer.getBoundingClientRect();
		return getComputedStyle(cards).display !== 'none' && rect.bottom <= innerHeight &&
			document.documentElement.scrollWidth <= document.documentElement.clientWidth;
	})()`) {
		t.Fatal("mobile cards or footer are unreachable or overflow horizontally")
	}
}
