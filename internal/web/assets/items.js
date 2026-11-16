(() => {
	"use strict";

	function itemSwapPlan(forms, sourceForm, editValidationError, focusedForm) {
		if (editValidationError) return { defer: true, rows: [] };
		const protectedForms = forms.filter((form) => form !== sourceForm && (
			form === focusedForm || form.dataset.dirty === "true" || form.dataset.submitting === "true"
		));
		if (focusedForm && focusedForm !== sourceForm && !protectedForms.includes(focusedForm)) {
			protectedForms.push(focusedForm);
		}
		if (protectedForms.some((form) => !form.closest("[data-item-row]"))) {
			return { defer: true, rows: [] };
		}
		return {
			defer: false,
			rows: [...new Set(protectedForms.map((form) => form.closest("[data-item-row]")))],
		};
	}

	function restoreFailedOptimisticRow(form) {
		const row = form?._optimisticRow;
		if (!row) return false;
		delete row.dataset.optimistic;
		form._optimisticRow = null;
		return true;
	}

	if (typeof module !== "undefined" && module.exports) module.exports = { itemSwapPlan, restoreFailedOptimisticRow };
	if (typeof document === "undefined" || typeof window === "undefined") return;
	if (!window.htmx) return;

	let refreshPending = false;
	let rowsToRestore = [];
	let focusAfterSwap = null;
	const list = () => document.querySelector("#item-list");
	const forms = (root) => [...root.querySelectorAll("form[data-dirty-track]")];
	const status = (root) => root.querySelector("#realtime-status");
	const requestForm = (event) => event.detail?.ctx?.sourceElement?.closest?.("form") || null;
	const formError = (event) => event.detail?.ctx?.response?.headers?.get?.("HX-Trigger")
		?.split(",").map((value) => value.trim()).includes("form-error") || false;

	function showError(root, event) {
		const responseText = event.detail?.ctx?.text;
		if (!formError(event) || !responseText) return;
		const documentFromResponse = new DOMParser().parseFromString(responseText, "text/html");
		const source = documentFromResponse.querySelector('[role="alert"]');
		if (!source?.textContent?.trim()) return;
		let alert = root.querySelector('[data-item-response-error]');
		if (!alert) {
			alert = document.createElement("p");
			alert.className = "alert alert-error error";
			alert.setAttribute("role", "alert");
			alert.setAttribute("tabindex", "-1");
			alert.dataset.itemResponseError = "";
			root.insertBefore(alert, root.firstChild);
		}
		alert.textContent = source.textContent.trim();
		alert.focus();
	}

	function refreshWhenClean() {
		const root = list();
		if (!refreshPending || !root || forms(root).some((form) =>
			form.dataset.dirty === "true" || form.dataset.submitting === "true")) return;
		const activeForm = document.activeElement?.closest?.("form[data-dirty-track]");
		if (activeForm && root.contains(activeForm)) return;
		refreshPending = false;
		window.htmx.ajax("GET", location.pathname + location.search, {
			target: "#item-list", swap: "outerHTML"
		});
	}

	function restoreDirtyRows(root) {
		const body = root.querySelector("tbody");
		if (!body) return;
		for (const row of rowsToRestore) {
			const incoming = [...body.querySelectorAll("[data-item-row]")]
				.find((candidate) => candidate.dataset.itemRow === row.dataset.itemRow);
			if (incoming) incoming.replaceWith(row);
			else body.append(row);
		}
		rowsToRestore = [];
		focusAfterSwap?.focus();
		focusAfterSwap = null;
	}

	document.addEventListener("htmx:before:swap", (event) => {
		const root = list();
		if (!root || event.detail?.ctx?.target !== root || event.defaultPrevented) return;
		const sourceForm = requestForm(event);
		const isEditError = sourceForm?.matches("[data-item-edit]") && formError(event);
		const hasFormError = formError(event);
		if (hasFormError) restoreFailedOptimisticRow(sourceForm);
		const focusedForm = document.activeElement?.closest?.("form[data-dirty-track]");
		const active = document.activeElement;
		const focusedFormInList = focusedForm && root.contains(focusedForm) ? focusedForm : null;
		const plan = itemSwapPlan(forms(root), sourceForm, isEditError, focusedFormInList);
		if (!plan.defer) {
			rowsToRestore = plan.rows;
			focusAfterSwap = plan.rows.some((row) => row.contains(active)) ? active : null;
			refreshPending ||= plan.rows.length > 0
				|| plan.rows.some((row) => row === sourceForm?.closest?.("[data-item-row]"));
			return;
		}
		event.preventDefault();
		showError(root, event);
		if (!isEditError) {
			refreshPending = true;
			const liveStatus = status(root);
			if (liveStatus) liveStatus.textContent = root.dataset.liveWaiting || "";
		}
	});
	document.addEventListener("htmx:after:swap", (event) => {
		if (event.detail?.ctx?.target?.id !== "item-list") return;
		const root = list();
		if (!root) return;
		restoreDirtyRows(root);
		setTimeout(refreshWhenClean, 0);
	});

	for (const name of ["input", "change", "focusout", "htmx:finally:request"]) {
		document.addEventListener(name, () => setTimeout(refreshWhenClean, 0));
	}
})();
