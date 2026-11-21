(() => {
	"use strict";

	// Alpine components for the app's browser behaviors. Each factory keeps its
	// DOM access inside init() (via this.$root) so the node tests can drive it
	// with a fake root element; registration runs only in a browser.

	// notificationsLive: the unread badge and the live status follow the
	// notification event stream. A resync reloads the page; a stream error only
	// announces the reconnect, the stream itself retries on its own.
	const notificationsLive = () => ({
		init() {
			const root = this.$root;
			const unreadCount = root.querySelector("[data-unread-count]");
			if (!unreadCount || !window.EventSource) return;
			const liveStatus = root.querySelector("[data-notification-live]");
			const cursor = root.querySelector("[data-notification-cursor]")?.dataset.notificationCursor ?? "";
			const stream = new EventSource(`/notifications/events?after=${encodeURIComponent(cursor)}`);
			stream.addEventListener("open", () => {
				if (liveStatus) liveStatus.textContent = "";
			});
			stream.addEventListener("notification.created", () => {
				const count = Number(unreadCount.textContent);
				if (Number.isSafeInteger(count) && count >= 0) unreadCount.textContent = String(count + 1);
				if (liveStatus) liveStatus.textContent = liveStatus.dataset.updated;
			});
			stream.addEventListener("error", () => {
				if (liveStatus) liveStatus.textContent = liveStatus.dataset.disconnected;
			});
			stream.addEventListener("resync", () => location.reload());
		},
	});


	// WebAuthn passkey flows. The shared helpers carry only the server's safe
	// text in notices, never raw internals; a cancel re-enables the buttons, a
	// server rejection shows its message, any other failure reloads.
	const isCancellation = (err) => {
		if (!err) return false;
		const name = err.name || "";
		const msg = (err.message || "").toLowerCase();
		return name === "NotAllowedError" || name === "AbortError" || name === "TimeoutError" || msg.includes("abort") || msg.includes("cancel") || msg.includes("timeout") || msg.includes("notallowed");
	};
	const passkeyNotice = (message, container) => {
		let notice = document.querySelector("p.error[role=alert]");
		if (!notice) {
			notice = document.createElement("p");
			notice.className = "error";
			notice.setAttribute("role", "alert");
			const parent = container || document.getElementById("main") || document.querySelector("main") || document.body;
			const heading = parent.querySelector("h1, h2");
			if (heading && heading.nextSibling) {
				parent.insertBefore(notice, heading.nextSibling);
			} else {
				parent.prepend(notice);
			}
		}
		notice.textContent = message;
		notice.hidden = false;
		if (typeof notice.focus === "function") {
			notice.setAttribute("tabindex", "-1");
			notice.focus();
		}
	};
	class RejectedResponse extends Error {}
	const rejectIfNotOk = async (response) => {
		if (response.ok) return;
		let detail = "";
		try {
			const body = await response.json();
			if (typeof body.detail === "string") detail = body.detail;
		} catch {
			detail = "";
		}
		throw new RejectedResponse(detail.trim() || "The passkey registration failed.");
	};
	const passkeyBusy = async (buttons, fn, { cancelMessage, container }) => {
		const setButtonsDisabled = (disabled) => buttons.forEach((b) => { b.disabled = disabled; });
		setButtonsDisabled(true);
		try {
			await fn();
		} catch (err) {
			if (err instanceof RejectedResponse) {
				setButtonsDisabled(false);
				passkeyNotice(err.message, container);
			} else if (isCancellation(err)) {
				setButtonsDisabled(false);
				passkeyNotice(cancelMessage, container);
			} else {
				window.location.reload();
			}
		}
	};
	const decodeChallenge = (value) => Uint8Array.from(atob(value.replace(/-/g, "+").replace(/_/g, "/") + "==="), (c) => c.charCodeAt(0));
	const encodeCredential = (value) => { let binary = ""; new Uint8Array(value).forEach((byte) => binary += String.fromCharCode(byte)); return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, ""); };

	// passkeyLogin: signs in with the passkey offered by the page's challenge,
	// then posts the credential and follows the server's return-to header.
	const passkeyLogin = () => ({
		init() {
			const button = document.getElementById("passkey-login");
			const optionsNode = document.getElementById("mfa-options");
			if (!window.PublicKeyCredential || !navigator.credentials) return;
			if (!button || !optionsNode) return;
			button.addEventListener("click", () => passkeyBusy(document.querySelectorAll("button, input[type=submit]"), async () => {
				const options = JSON.parse(optionsNode.textContent);
				options.publicKey.challenge = decodeChallenge(options.publicKey.challenge);
				(options.publicKey.allowCredentials || []).forEach((item) => item.id = decodeChallenge(item.id));
				const credential = await navigator.credentials.get({ publicKey: options.publicKey });
				const response = { id: credential.id, rawId: encodeCredential(credential.rawId), type: credential.type, response: { clientDataJSON: encodeCredential(credential.response.clientDataJSON), authenticatorData: encodeCredential(credential.response.authenticatorData), signature: encodeCredential(credential.response.signature), userHandle: credential.response.userHandle ? encodeCredential(credential.response.userHandle) : null } };
				const result = await fetch("/mfa/passkey", { method: "POST", headers: { "Content-Type": "application/json", "X-MFA-CSRF": button.dataset.csrf }, body: JSON.stringify(response) });
				if (result.ok) {
					window.location = result.headers.get("X-MFA-Return-To") || "/";
				} else {
					window.location.reload();
				}
			}, { cancelMessage: "Passkey verification was canceled or timed out." }));
		},
	});

	// passkeyEnroll: registers a passkey with the server's challenge, then shows
	// the recovery codes it returns.
	const passkeyEnroll = () => ({
		init() {
			const enrollment = document.getElementById("passkey-enroll");
			if (!window.PublicKeyCredential || !navigator.credentials) return;
			if (!enrollment) return;
			enrollment.addEventListener("submit", (event) => {
				event.preventDefault();
				return passkeyBusy(enrollment.querySelectorAll("button, input[type=submit]"), async () => {
					const csrf = enrollment.querySelector("[name=csrf]").value;
					const response = await fetch(enrollment.action, { method: "POST", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify({ password: enrollment.querySelector("[name=password]").value }) });
					await rejectIfNotOk(response);
					const data = await response.json();
					const options = data.options;
					options.publicKey.challenge = decodeChallenge(options.publicKey.challenge);
					options.publicKey.user.id = decodeChallenge(options.publicKey.user.id);
					(options.publicKey.excludeCredentials || []).forEach((item) => item.id = decodeChallenge(item.id));
					const credential = await navigator.credentials.create({ publicKey: options.publicKey });
					const result = { id: credential.id, rawId: encodeCredential(credential.rawId), type: credential.type, response: { clientDataJSON: encodeCredential(credential.response.clientDataJSON), attestationObject: encodeCredential(credential.response.attestationObject), transports: credential.response.getTransports ? credential.response.getTransports() : [] } };
					const completed = await fetch("/api/v1/auth/mfa/passkeys/register", { method: "POST", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf, "X-MFA-Challenge": data.challenge, "X-MFA-Name": "Passkey" }, body: JSON.stringify(result) });
					await rejectIfNotOk(completed);
					const completedData = await completed.json();
					if (Array.isArray(completedData.recoveryCodes) && completedData.recoveryCodes.length > 0) {
						const output = document.getElementById("mfa-recovery-output");
						const values = document.getElementById("mfa-recovery-values");
						if (!output || !values) throw new Error();
						values.textContent = completedData.recoveryCodes.join("\n");
						output.hidden = false;
						enrollment.hidden = true;
						return;
					}
					window.location.reload();
				}, { cancelMessage: "Passkey registration was canceled or timed out.", container: enrollment.parentElement });
			});
		},
	});

	// Item list swap orchestration: keeps a dirty or focused row across
	// htmx swaps, restores optimistic rows that failed, and refreshes the
	// list only when nothing is being edited. The plan itself is pure.
	const itemSwapPlan = (forms, sourceForm, editValidationError, focusedForm) => {
		if (editValidationError) return { defer: true, rows: [], forms: [] };
		const protectedForms = forms.filter((form) => form !== sourceForm && (
			form === focusedForm || form.dataset.dirty === "true" || form.dataset.submitting === "true"
		));
		if (focusedForm && focusedForm !== sourceForm && !protectedForms.includes(focusedForm)) {
			protectedForms.push(focusedForm);
		}
		const rows = [];
		const standaloneForms = [];
		for (const form of protectedForms) {
			const row = form.closest("[data-item-row]");
			if (row) rows.push(row);
			else standaloneForms.push(form);
		}
		return {
			defer: false,
			rows: [...new Set(rows)],
			forms: [...new Set(standaloneForms)],
		};
	};

	const restoreFailedOptimisticRow = (form) => {
		const row = form?._optimisticRow;
		if (!row) return false;
		delete row.dataset.optimistic;
		form._optimisticRow = null;
		return true;
	};

	// itemList: mounted on the item list root. The wiring resolves the root
	// from the document on every event, exactly like the script it replaces,
	// so a swapped-in list keeps working under one set of listeners.
	const itemList = () => ({
		init() {
			if (window.__itemListWired) return;
			// htmx loads after this file in document order; retry until it is there.
			if (!window.htmx) { setTimeout(() => this.init(), 25); return; }
			window.__itemListWired = true;

			let refreshPending = false;
			let rowsToRestore = [];
			let formsToRestore = [];
			let focusAfterSwap = null;
			const list = () => document.querySelector("#item-list");
			const forms = (root) => [...root.querySelectorAll("form[data-dirty-track]")];
			const status = (root) => root.querySelector("#realtime-status");
			const requestForm = (event) => event.detail?.ctx?.sourceElement?.closest?.("form") || null;
			const formError = (event) => event.detail?.ctx?.response?.headers?.get?.("HX-Trigger")
				?.split(",").map((value) => value.trim()).includes("form-error") || false;

			const showError = (root, event) => {
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
			};

			const refreshWhenClean = () => {
				const root = list();
				if (!refreshPending || !root || forms(root).some((form) =>
					form.dataset.dirty === "true" || form.dataset.submitting === "true")) return;
				const activeForm = document.activeElement?.closest?.("form[data-dirty-track]");
				if (activeForm && root.contains(activeForm)) return;
				refreshPending = false;
				window.htmx.ajax("GET", location.pathname + location.search, {
					target: "#item-list", swap: "outerHTML"
				});
			};

			const restoreDirtyRows = (root) => {
				const body = root.querySelector("tbody");
				if (body) {
					for (const row of rowsToRestore) {
						const incoming = [...body.querySelectorAll("[data-item-row]")]
							.find((candidate) => candidate.dataset.itemRow === row.dataset.itemRow);
						if (incoming) incoming.replaceWith(row);
						else body.append(row);
					}
				}
				rowsToRestore = [];
				for (const form of formsToRestore) {
					const incoming = form.dataset.persist
						? [...root.querySelectorAll("form[data-persist]")].find((candidate) => candidate.dataset.persist === form.dataset.persist)
						: root.querySelector("form.client-query");
					incoming?.replaceWith(form);
				}
				formsToRestore = [];
				focusAfterSwap?.focus();
				focusAfterSwap = null;
			};

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
					formsToRestore = plan.forms;
					focusAfterSwap = plan.rows.some((row) => row.contains(active))
						|| plan.forms.some((form) => form.contains(active)) ? active : null;
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
		},
	});

	
	// listScreen: shared list behaviors — row tap-to-open, toolbar autosubmit,
	// filter condition sections, live-search status, menu placement, column
	// drag order, and bulk selection. Everything resolves from the document
	// per event, so a swapped list keeps working under one set of listeners.
	const listScreen = () => ({
		init() {
			if (window.__listScreenWired) return;
			window.__listScreenWired = true;

			const interactive = "a, button, input, select, textarea, label, summary, details";

			const rowOpenTarget = (event) => {
				const row = event.target.closest("[data-row-href]");
				if (!row || event.target.closest(interactive)) return null;
				if (window.getSelection && window.getSelection().toString() !== "") return null;
				return row.dataset.rowHref;
			};

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
			const syncCondition = (form) => {
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
			};

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
				panel.style.maxHeight = innerHeight - 2 * margin + "px";
				panel.style.overflowY = "auto";
				const rect = summary.getBoundingClientRect();
				const width = Math.min(panel.offsetWidth, innerWidth - 2 * margin);
				panel.style.width = width + "px";
				const height = panel.offsetHeight;
				const left = Math.max(margin, Math.min(rect.right - width, innerWidth - width - margin));
				let top = rect.bottom + 4;
				if (top + height > innerHeight - margin) top = Math.max(margin, rect.top - 4 - height);
				panel.style.left = left + "px";
				panel.style.top = top + "px";
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
			const syncBulk = () => {
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
			};
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
		},
	});

if (typeof module !== "undefined" && module.exports) module.exports = { notificationsLive, passkeyLogin, passkeyEnroll, itemSwapPlan, restoreFailedOptimisticRow, itemList, listScreen };
	if (typeof document === "undefined" || typeof window === "undefined") return;
	document.addEventListener("alpine:init", () => {
		window.Alpine.data("notificationsLive", notificationsLive);
		window.Alpine.data("passkeyLogin", passkeyLogin);
		window.Alpine.data("passkeyEnroll", passkeyEnroll);
		window.Alpine.data("itemList", itemList);
		window.Alpine.data("listScreen", listScreen);
	});
})();
