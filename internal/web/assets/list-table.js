(() => {
	"use strict";

	// Clicking or tapping a list row opens its link. Real links and form controls keep their own behavior,
	// and a text selection made by dragging is left alone. The first visible cell carries a real link, so keyboard
	// users reach the same page without this script.
	const interactive = "a, button, input, select, textarea, label, summary, details";

	function rowOpenTarget(event) {
		const row = event.target.closest("[data-row-href]");
		if (!row || event.target.closest(interactive)) return null;
		if (window.getSelection && window.getSelection().toString() !== "") return null;
		return row.dataset.rowHref;
	}

	// Dropdowns on list toolbars submit as soon as the value changes, so their Apply buttons are not
	// needed. Without this script the Apply buttons stay visible and work as before.
	document.documentElement.classList.add("js-enhanced");
	document.querySelectorAll("[data-submit-button]").forEach((button) => {
		button.hidden = true;
	});
	document.addEventListener("change", (event) => {
		const select = event.target;
		if (select && select.dataset && select.dataset.autosubmit && select.form) select.form.requestSubmit();
	});

	// Shows or hides add-filter inputs as the condition changes. A hidden input is disabled, so the
	// browser does not send it. The visible value is required, and the "to" value is required when shown.
	function syncCondition(form) {
		const checked = form.querySelector('input[name="filter_op"]:checked');
		const operator = checked ? checked.value : "";
		form.querySelectorAll("[data-hide-for-operator]").forEach((section) => {
			const hide = section.dataset.hideForOperator === operator;
			section.hidden = hide;
			section.querySelectorAll("input, select").forEach((input) => {
				input.disabled = hide;
				input.required = !hide;
			});
		});
		form.querySelectorAll("[data-show-for-operator]").forEach((section) => {
			const show = section.dataset.showForOperator === operator;
			section.hidden = !show;
			section.querySelectorAll("input, select").forEach((input) => {
				input.disabled = !show;
				input.required = show;
			});
		});
	}

	document.querySelectorAll('input[name="filter_op"]').forEach((input) => {
		if (input.form) syncCondition(input.form);
	});
	document.addEventListener("change", (event) => {
		const input = event.target;
		if (input && input.name === "filter_op" && input.form) syncCondition(input.form);
	});

	document.addEventListener("click", (event) => {
		const href = rowOpenTarget(event);
		if (!href) return;
		if (event.ctrlKey || event.metaKey || event.shiftKey) {
			window.open(href, "_blank", "noopener");
			return;
		}
		window.location.assign(href);
	});

	// Live search: the search form swaps the results region as the user types. htmx pushes the results-fragment
	// address into history, so the address bar gets the page's own address back. A failed search shows the
	// status line next to the form.
	const withoutFragmentParams = (url) => {
		// Only the fragment marker leaves the address bar; all=1 stays so a reload shows the same full page.
		url.searchParams.delete("part");
		return url;
	};
	document.addEventListener("htmx:after:history:push", () => {
		const url = withoutFragmentParams(new URL(location.href));
		history.replaceState(history.state, "", url.pathname + url.search + url.hash);
	});
	const searchStatus = (event) => {
		const element = event.detail?.ctx?.elt || event.target;
		const form = element?.closest?.(".list-search");
		return form ? form.querySelector(".list-search-status") : null;
	};
	document.addEventListener("htmx:before:request", (event) => {
		const status = searchStatus(event);
		if (status) status.hidden = true;
	});
	const showSearchFailure = (event) => {
		const status = searchStatus(event);
		if (!status) return;
		status.textContent = status.dataset.errorText || "";
		status.hidden = false;
	};
	document.addEventListener("htmx:response:error", showSearchFailure);
	document.addEventListener("htmx:error", showSearchFailure);

	// Sorting, filtering, paging, and search swap the results region, which does not announce itself. The
	// toolbar's live region repeats the footer count ("Showing 25 of 412 rows") when that count changes.
	document.addEventListener("htmx:after:settle", () => {
		const count = document.querySelector("#list-results .list-footer-count");
		const announce = document.querySelector("[data-list-announce]");
		if (count && announce && announce.textContent !== count.textContent) announce.textContent = count.textContent;
	});

	// Menus that open a panel from a button: row actions, columns, add-filter steps, and new-record forms. Only one
	// is open at a time. An open panel is fixed to the window, so the table's scroll area cannot hide it, and it is
	// placed inside the window: below its button, or above it when there is no room below.
	const menuSelector = "details.list-menu";
	const margin = 8;
	const placeMenu = (menu) => {
		const panel = menu.querySelector(":scope > .dropdown-content");
		const summary = menu.querySelector(":scope > summary");
		if (!panel || !summary) return;
		panel.style.position = "fixed";
		panel.style.right = "auto";
		panel.style.width = "";
		panel.style.maxHeight = `${innerHeight - 2 * margin}px`;
		panel.style.overflowY = "auto";
		const rect = summary.getBoundingClientRect();
		const width = Math.min(panel.offsetWidth, innerWidth - 2 * margin);
		panel.style.width = `${width}px`;
		const height = panel.offsetHeight;
		const left = Math.max(margin, Math.min(rect.right - width, innerWidth - width - margin));
		let top = rect.bottom + 4;
		if (top + height > innerHeight - margin) top = Math.max(margin, rect.top - 4 - height);
		panel.style.left = `${left}px`;
		panel.style.top = `${top}px`;
	};
	const placeOpenMenus = () => {
		document.querySelectorAll(menuSelector).forEach((menu) => { if (menu.open) placeMenu(menu); });
	};
	document.addEventListener("toggle", (event) => {
		const menu = event.target;
		if (!(menu instanceof HTMLDetailsElement) || !menu.matches(menuSelector) || !menu.open) return;
		document.querySelectorAll(menuSelector).forEach((other) => { if (other !== menu && other.open) other.open = false; });
		placeMenu(menu);
	}, true);
	window.addEventListener("resize", placeOpenMenus);
	window.addEventListener("scroll", placeOpenMenus, true);
	document.addEventListener("click", (event) => {
		document.querySelectorAll(menuSelector).forEach((menu) => { if (menu.open && !menu.contains(event.target)) menu.open = false; });
	});
	document.addEventListener("keydown", (event) => {
		if (event.key !== "Escape") return;
		document.querySelectorAll(menuSelector).forEach((menu) => { if (menu.open) menu.open = false; });
	});

	// Column order: the columns menu lists the columns in table order. The move buttons and dragging change the order,
	// and the new order is saved from the menu's hidden order field. The first column, the name, stays first.
	const saveColumnOrder = (list) => {
		const form = list.closest(".list-menu")?.querySelector("form.list-column-order");
		if (!form) return;
		form.querySelector('input[name="order"]').value = [...list.querySelectorAll(":scope > li[data-column-key]")].map((row) => row.dataset.columnKey).join(",");
		form.requestSubmit();
	};
	let draggedColumn = null;
	document.addEventListener("dragstart", (event) => {
		const row = event.target.closest?.("li[data-column-key]");
		if (!row || row.hasAttribute("data-locked")) {
			event.preventDefault();
			return;
		}
		draggedColumn = row;
		event.dataTransfer.effectAllowed = "move";
		event.dataTransfer.setData("text/plain", row.dataset.columnKey);
	});
	document.addEventListener("dragover", (event) => {
		const row = event.target.closest?.("li[data-column-key]");
		if (!draggedColumn || !row || row === draggedColumn || row.parentElement !== draggedColumn.parentElement) return;
		event.preventDefault();
		const rect = row.getBoundingClientRect();
		const after = event.clientY > rect.top + rect.height / 2;
		row.parentElement.insertBefore(draggedColumn, after ? row.nextElementSibling : row);
	});
	document.addEventListener("drop", (event) => {
		if (!draggedColumn) return;
		event.preventDefault();
		const list = draggedColumn.parentElement;
		draggedColumn = null;
		saveColumnOrder(list);
	});
	// Bulk selection: select-all picks the rows on this page. The bar counts the chosen rows and enables its buttons only
		// when at least one row is chosen. A results swap replaces the rows and the bar together, so the selection clears.
		// A client renders a table checkbox and a card checkbox with the same value, so the bar counts distinct values
		// and a change to one is mirrored to the other.
		function syncBulk() {
			const rows = [...document.querySelectorAll("input[data-bulk-row]")];
			const everyClient = new Set(rows.map((row) => row.value)).size;
			const chosen = new Set(rows.filter((row) => row.checked).map((row) => row.value)).size;
			const all = document.querySelector("input[data-bulk-all]");
			if (all) {
				all.checked = everyClient > 0 && chosen === everyClient;
				all.indeterminate = chosen > 0 && chosen < everyClient;
			}
			const count = document.querySelector("[data-count-template]");
			if (count) count.textContent = count.dataset.countTemplate.replace("COUNT", String(chosen));
			// The actions appear only while at least one client is selected.
			const bar = document.getElementById("client-bulk");
			if (bar) bar.classList.toggle("is-selecting", chosen > 0);
		}
		document.addEventListener("change", (event) => {
			const target = event.target;
			if (!target || !target.matches) return;
			if (target.matches("input[data-bulk-all]")) {
				document.querySelectorAll("input[data-bulk-row]").forEach((row) => { row.checked = target.checked; });
			} else if (target.matches("input[data-bulk-row]")) {
				document.querySelectorAll("input[data-bulk-row]").forEach((row) => {
					if (row.value === target.value) row.checked = target.checked;
				});
			}
			if (target.matches("input[data-bulk-all], input[data-bulk-row]")) syncBulk();
		});
		document.addEventListener("htmx:after:settle", syncBulk);
		document.addEventListener("list:results-changed", syncBulk);
		syncBulk();

	document.addEventListener("dragend", () => {
		draggedColumn = null;
	});
})();
