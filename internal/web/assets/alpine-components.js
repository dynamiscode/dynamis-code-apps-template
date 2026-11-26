(() => {
	"use strict";

	function createAction(tag, classes) {
		const action = document.createElement(tag);
		action.className = classes;
		return action;
	}

	const copyIntakeLink = async (field, clipboard, origin) => {
		const url = new URL(field.value, origin).href;
		try {
			await clipboard.writeText(url);
			return true;
		} catch (_) {
			field.value = url;
			field.focus();
			field.select();
			return false;
		}
	};

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
			notice.className = "error alert alert-error font-bold";
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
			// Every dropdown behaves the same way: one open at a time, closed by a tap outside or Escape,
			// and its panel placed inside the window instead of hanging off the edge.
			const menuSelector = "details.dropdown";
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


	// ---- Pure form evaluators (exported for the exhaustive tests) ----

	// Matches validateNewPassword in internal/identity/account.go.
	const MIN_PASSWORD = 12;

	// 0 means the server would reject it; 1..4 grow with length and variety.
	const passwordStrength = (value) => {
		if (typeof value !== "string" || value.length < MIN_PASSWORD) return 0;
		if (/^(.)\1+$/.test(value)) return 1;
		const classes = [/[a-z]/, /[A-Z]/, /[0-9]/, /[^A-Za-z0-9]/].filter((r) => r.test(value)).length;
		let score = 1;
		if (value.length >= 16) score++;
		if (classes >= 3) score++;
		if (value.length >= 20 && classes >= 2) score++;
		return Math.min(4, score);
	};

	// Authenticator codes: keep digits only, at most length. Matches
	// normalizeTOTPCode in internal/identity/mfa.go for any code it accepts.
	const maskDigits = (value, length) => String(value).replace(/\D/g, "").slice(0, length);

	// Recovery codes: upper case, no spaces. Matches normalizeRecoveryCode.
	const maskUpper = (value) => String(value).toUpperCase().replace(/\s+/g, "");

	// What to do with a stored draft when a page with its form loads.
	const draftAction = (stored, pageHasError) => {
		if (!stored) return "none";
		if (stored.submitted && !pageHasError) return "discard";
		return "restore";
	};

	const serialize = (entries) => JSON.stringify(entries);
	const isDirty = (initial, current) => serialize(initial) !== serialize(current);

	// Wizard: the next step index. Moving forward needs a valid current step.
	const wizardMove = (step, total, direction, currentValid) => {
		if (direction === "next") return currentValid && step < total - 1 ? step + 1 : step;
		if (direction === "back") return step > 0 ? step - 1 : step;
		return step;
	};

	// Realtime list refresh: never replace a list the user is editing.
	const refreshDecision = (editing) => editing ? "defer" : "refresh";

	// Optimistic removal: keep the row hidden only while the request is pending
	// or after it succeeded; show it again on any failure.
	const optimisticOutcome = (status) => {
		if (status === "pending") return "hidden";
		return typeof status === "number" && status >= 200 && status < 400 ? "removed" : "restored";
	};

	const isAuditFragment = (response, baseURL, firstElementID) =>
		response.ok && !response.redirected && new URL(response.url, baseURL).pathname === new URL(baseURL).pathname && firstElementID === "audit-table";

	// A lazy section places a fragment with the same id as its placeholder, from the same path.
	const isLazyFragment = (response, baseURL, firstElementID, expectedID) =>
		response.ok && !response.redirected && new URL(response.url, baseURL).pathname === new URL(baseURL).pathname && firstElementID === expectedID;

	// Inner SVG paths for the password reveal eye. `shown` picks the crossed-out
	// eye; the button's aria-label carries the meaning, so the glyph is decorative.
	const revealIconPaths = (shown) => shown
		? '<path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19M14.12 14.12a3 3 0 1 1-4.24-4.24"/><line x1="1" y1="1" x2="23" y2="23"/>'
		: '<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/>';

	const core = {
		MIN_PASSWORD, passwordStrength, maskDigits, maskUpper, draftAction, isDirty, wizardMove,
		refreshDecision, optimisticOutcome, isAuditFragment, revealIconPaths,
	};

	// ---- File uploader (app.js) ----

	class FileUploader {
		static PHASES = Object.freeze({
			IDLE: "IDLE",
			UPLOADING: "UPLOADING",
			SUCCESS: "SUCCESS",
			ERROR: "ERROR"
		});

		static PHASE_MESSAGES = Object.freeze({
			IDLE: "",
			UPLOADING: "Uploading file...",
			SUCCESS: "Upload complete.",
			ERROR: "Upload failed. Please try again."
		});

		constructor(form) {
			this.form = form;
			this.submitButton = form?.querySelector('button[type="submit"]') || form?.querySelector("button");
			this.fileInput = form?.querySelector('input[type="file"]');
			this.statusElement = form?.querySelector("[data-upload-status]")
				|| form?.querySelector('[role="status"]')
				|| form?.querySelector('[aria-live="polite"]');
			this.progressElement = form?.querySelector("[data-upload-progress]");
			this.phase = FileUploader.PHASES.IDLE;
			this.lastProgressAnnouncement = 0;
		}

		ensureStatusElement() {
			if (!this.statusElement && this.form) {
				this.statusElement = document.createElement("p");
				this.statusElement.setAttribute("role", "status");
				this.statusElement.setAttribute("aria-live", "polite");
				this.statusElement.className = "status-line";
				this.statusElement.setAttribute("data-upload-status", "");
				this.form.appendChild(this.statusElement);
			}
			return this.statusElement;
		}

		getPhaseMessage(phase) {
			if (this.form?.dataset) {
				const phaseKey = phase.charAt(0) + phase.slice(1).toLowerCase();
				const custom = this.form.dataset[`status${phaseKey}`] || this.form.dataset[`phase${phaseKey}`];
				if (custom) return custom;
			}
			return FileUploader.PHASE_MESSAGES[phase] || "";
		}

		isInFlight() {
			return this.phase === FileUploader.PHASES.UPLOADING ||
				this.phase === FileUploader.PHASES.SUCCESS;
		}

		transition(phase, message) {
			const previousPhase = this.phase;
			this.phase = phase;
			const isError = phase === FileUploader.PHASES.ERROR;
			const inFlight = this.isInFlight();

			if (this.progressElement) {
				if (phase === FileUploader.PHASES.UPLOADING && previousPhase !== phase) {
					this.progressElement.value = 0;
					this.progressElement.setAttribute("hidden", "");
					this.lastProgressAnnouncement = 0;
				} else if (phase !== FileUploader.PHASES.UPLOADING) {
					this.progressElement.setAttribute("hidden", "");
				}
			}

			if (this.submitButton) {
				this.submitButton.disabled = inFlight;
			}
			if (this.fileInput) {
				this.fileInput.disabled = inFlight;
			}

			const statusEl = this.ensureStatusElement();
			if (statusEl) {
				const text = message !== undefined ? message : this.getPhaseMessage(phase);
				statusEl.textContent = text;
				if (text) {
					statusEl.removeAttribute("hidden");
				} else {
					statusEl.setAttribute("hidden", "");
				}
				if (isError) {
					statusEl.classList.add("error");
					statusEl.classList.remove("status-line");
				} else {
					statusEl.classList.remove("error");
					statusEl.classList.add("status-line");
				}
			}
		}

		reportProgress(event) {
			if (!this.progressElement || !event.lengthComputable || event.total <= 0) return;
			const percent = Math.min(100, Math.max(0, Math.round((event.loaded / event.total) * 100)));
			this.progressElement.value = percent;
			this.progressElement.removeAttribute("hidden");
			const announcedPercent = Math.floor(percent / 25) * 25;
			if (announcedPercent > this.lastProgressAnnouncement) {
				this.lastProgressAnnouncement = announcedPercent;
				this.transition(FileUploader.PHASES.UPLOADING, this.getPhaseMessage(FileUploader.PHASES.UPLOADING) + " " + announcedPercent + "%");
			}
		}

		async handleSubmit(event) {
			if (event) event.preventDefault();
			if (this.isInFlight()) return;

			const file = this.fileInput?.files?.[0];
			if (!file) {
				this.transition(FileUploader.PHASES.ERROR, this.form?.dataset?.statusChooseFile || this.getPhaseMessage(FileUploader.PHASES.ERROR));
				return;
			}

			const csrf = this.form?.querySelector('input[name="csrf"]')?.value || "";
			const body = new FormData();
			body.append("csrf", csrf);
			body.append("file", file, file.name);

			try {
				this.transition(FileUploader.PHASES.UPLOADING);
				await new Promise((resolve, reject) => {
					const request = new XMLHttpRequest();
					request.open("POST", this.form.action);
					request.upload.addEventListener("progress", (progressEvent) => this.reportProgress(progressEvent));
					request.addEventListener("load", async () => {
						if (request.status >= 200 && request.status < 300) {
							resolve();
							return;
						}
						const message = await FileUploader.extractErrorMessage(request.responseText, this.getPhaseMessage(FileUploader.PHASES.ERROR));
						reject(message);
					});
					request.addEventListener("error", () => reject(this.getPhaseMessage(FileUploader.PHASES.ERROR)));
					request.addEventListener("abort", () => reject(this.getPhaseMessage(FileUploader.PHASES.ERROR)));
					request.send(body);
				});

				this.transition(FileUploader.PHASES.SUCCESS);
				if (this.form?.action && typeof window !== "undefined" && window.location) {
					window.location.assign(this.form.action);
				}
			} catch (message) {
				this.transition(FileUploader.PHASES.ERROR, typeof message === "string" ? message : this.getPhaseMessage(FileUploader.PHASES.ERROR));
			}
		}

		static async extractErrorMessage(text, defaultMsg) {
			try {
				if (text) {
					if (typeof DOMParser !== "undefined") {
						const doc = new DOMParser().parseFromString(text, "text/html");
						const alert = doc.querySelector('[role="alert"]');
						if (alert?.textContent?.trim()) {
							return alert.textContent.trim();
						}
					}
					try {
						const json = JSON.parse(text);
						if (json.detail) return json.detail;
						if (json.title) return json.title;
						if (json.error) return json.error;
					} catch (_) {}
				}
			} catch (_) {}
			return defaultMsg;
		}

		init() {
			if (!this.form) return;
			this.form.addEventListener("submit", (event) => this.handleSubmit(event));
		}
	}

	FileUploader.Phases = FileUploader.PHASES;
	FileUploader.PhaseMessages = FileUploader.PHASE_MESSAGES;

	if (typeof window !== "undefined") window.FileUploader = FileUploader;

	// ---- Progressive form enhancement (forms.js) ----

	// wireForms: toasts, inline validation with the error summary, busy state,
	// dirty tracking, drafts, the password meter and reveal toggle, wizards,
	// disclosure, lazy sections, and the htmx focus and busy restoration.
	// Every page works without it: forms post, the server validates, and errors
	// render as role="alert". This only adds feedback on top. The rules it
	// keeps are in the form-drafts, form-field-input, form-wizard and
	// async-feedback skills. The pure evaluators above decide; the DOM code
	// only applies their answers.
	const wireForms = () => {
		if (window.__formsWired) return;
		window.__formsWired = true;

		// The message catalog rides on a src-less script tag in the head.
		const script = document.querySelector("script[data-messages]");
		const msg = (name) => script?.getAttribute(`data-msg-${name}`) || "";
		const storage = (() => {
			try {
				const s = window.sessionStorage;
				s.setItem("dynamis:probe", "1");
				s.removeItem("dynamis:probe");
				return s;
			} catch (_) {
				return null;
			}
		})();
		let uid = 0;
		const idFor = (el, prefix) => el.id || (el.id = `${prefix}-${++uid}`);
		const checkable = (el) => el.type === "checkbox" || el.type === "radio";
		const controls = (root) => [...(root.elements || root.querySelectorAll("input, select, textarea"))].filter((el) => ["INPUT", "SELECT", "TEXTAREA"].includes(el.tagName));
		// Fields a draft may store: never secrets, hidden values, or files.
		const fieldsOf = (form, withSecrets) => controls(form).filter((el) => el.name && !["hidden", "file"].includes(el.type) && (withSecrets || !el.matches('input[type="password"], input[data-enhanced]')));
		const entryKey = (el) => (checkable(el) ? `${el.name}=${el.value}` : el.name);
		const entriesOf = (form, withSecrets) => fieldsOf(form, withSecrets).map((el) => [entryKey(el), el instanceof HTMLSelectElement && el.multiple ? [...el.selectedOptions].map((option) => option.value) : checkable(el) ? el.checked : el.value]);
		const visible = (el) => !el.closest("[hidden]");
		const pageHasError = () => !!document.querySelector('.error[role="alert"], .error-summary');

		// Toasts: one polite live region; Escape or the button dismisses.
		function toast(text, kind) {
			if (!text) return;
			let region = document.querySelector(".toasts");
			if (!region) {
				region = document.createElement("div");
				region.className = "toasts toast toast-bottom toast-end z-50";
				region.setAttribute("role", "status");
				region.setAttribute("aria-live", "polite");
				document.body.appendChild(region);
			}
			const item = document.createElement("p");
			item.className = `toast alert py-2 ${kind === "error" ? "alert-error" : "alert-success"}`;
			item.textContent = text;
			const close = createAction("button", "btn btn-outline btn-xs");
			close.type = "button";
			close.textContent = msg("dismiss");
			close.addEventListener("click", () => item.remove());
			item.appendChild(close);
			region.appendChild(item);
			setTimeout(() => item.remove(), 6000);
		}

		// Inline validation and the error summary.
		function fieldLabel(el) {
			const label = el.id && document.querySelector(`label[for="${el.id}"]`);
			return (label?.textContent || el.closest("fieldset")?.querySelector("legend")?.textContent || el.name).trim();
		}

		function setFieldError(el, message) {
			const errorId = `${idFor(el, "field")}-error`;
			let note = document.getElementById(errorId);
			const described = (el.getAttribute("aria-describedby") || "").split(" ").filter((x) => x && x !== errorId);
			if (message) {
				if (!note) {
					note = document.createElement("p");
					note.id = errorId;
					note.className = "field-error font-bold text-error";
					el.insertAdjacentElement("afterend", note);
				}
				note.textContent = message;
				el.setAttribute("aria-invalid", "true");
				described.push(errorId);
			} else {
				note?.remove();
				el.removeAttribute("aria-invalid");
			}
			if (described.length) el.setAttribute("aria-describedby", described.join(" "));
			else el.removeAttribute("aria-describedby");
		}

		function checkMatch(el) {
			const other = el.dataset.match && document.getElementById(el.dataset.match);
			if (other) el.setCustomValidity(el.value && el.value !== other.value ? msg("mismatch") : "");
		}

		function validateField(el) {
			if (el.dataset.trim !== undefined && el.value !== el.value.trim()) el.value = el.value.trim();
			checkMatch(el);
			const ok = el.checkValidity();
			setFieldError(el, ok ? "" : el.validationMessage);
			return ok;
		}

		function validateFields(fields) {
			return fields.filter((el) => el.willValidate && visible(el) && !validateField(el));
		}

		function showSummary(form, invalid) {
			form.querySelector(".error-summary")?.remove();
			if (!invalid.length) return;
			const box = document.createElement("div");
			box.className = "error-summary";
			box.setAttribute("role", "alert");
			box.tabIndex = -1;
			const title = document.createElement("p");
			title.textContent = msg("error-summary");
			const list = document.createElement("ul");
			for (const el of invalid) {
				const link = createAction("a", "rounded-field border border-base-300 px-1");
				link.href = `#${idFor(el, "field")}`;
				link.textContent = `${fieldLabel(el)}: ${el.validationMessage}`;
				link.addEventListener("click", (event) => {
					event.preventDefault();
					revealStep(el);
					el.focus();
				});
				const li = document.createElement("li");
				li.appendChild(link);
				list.appendChild(li);
			}
			box.append(title, list);
			form.prepend(box);
			box.focus();
		}

		// Busy state and double-submit prevention.
		function setBusy(form, busy) {
			if (busy) form.setAttribute("aria-busy", "true");
			else form.removeAttribute("aria-busy");
			form.dataset.submitting = busy ? "true" : "";
			for (const button of form.querySelectorAll("button:not([type=button])")) {
				if (busy) {
					button.dataset.wasDisabled = button.disabled ? "true" : "";
					button.disabled = true;
					if (!button.hasAttribute("aria-label")) {
						button.dataset.label = button.textContent;
						button.textContent = msg("saving");
						const spinner = document.createElement("span");
						spinner.className = "loading loading-spinner";
						button.prepend(spinner);
					}
				} else if (button.dataset.wasDisabled !== undefined) {
					button.disabled = button.dataset.wasDisabled === "true";
					if (button.dataset.label !== undefined) button.textContent = button.dataset.label;
					button.querySelector(".loading")?.remove();
					delete button.dataset.wasDisabled;
					delete button.dataset.label;
				}
			}
			if (!busy) updateDirty(form);
		}

		// Dirty tracking, enable-by-change, and the unsaved-changes warning.
		const initial = new WeakMap();
		const dirtyOnLoad = new WeakSet();
		function updateDirty(form) {
			if (!initial.has(form)) return;
			const dirty = isDirty(initial.get(form), entriesOf(form, true));
			if (dirty || !dirtyOnLoad.has(form)) form.dataset.dirty = dirty ? "true" : "";
			const status = form.querySelector("[data-dirty-status]");
			if (status) status.hidden = form.dataset.dirty !== "true";
			if (form.dataset.submitting === "true") return;
			for (const button of form.querySelectorAll("[data-requires-change]")) button.disabled = !dirty;
		}

		// Draft persistence (sessionStorage only; never passwords, hidden, or files).
		const draftKey = (form) => `dynamis:draft:${location.pathname}:${form.dataset.persist}`;
		function readDraft(form) {
			try {
				return JSON.parse(storage?.getItem(draftKey(form)) || "null");
			} catch (_) {
				return null;
			}
		}
		function writeDraft(form, submitted) {
			// While a submit is in flight the stored draft must stay marked submitted,
			// so the answer decides between discard (saved) and restore (rejected).
			if (!storage || (!submitted && form.dataset.submitting === "true")) return;
			const entries = entriesOf(form);
			const empty = entries.every(([, v]) => v === "" || v === false || (Array.isArray(v) && v.length === 0));
			try {
				if (empty && !submitted) storage.removeItem(draftKey(form));
				else storage.setItem(draftKey(form), JSON.stringify({ entries, submitted }));
			} catch (_) {}
		}
		function applyDraft(form, announce) {
			const stored = readDraft(form);
			const action = draftAction(stored, pageHasError());
			if (action === "discard") storage?.removeItem(draftKey(form));
			if (action !== "restore") return;
			const saved = new Map(stored.entries);
			for (const el of fieldsOf(form)) {
				if (!saved.has(entryKey(el))) continue;
				if (checkable(el)) el.checked = saved.get(entryKey(el));
				else if (el instanceof HTMLSelectElement && el.multiple) {
					const selected = new Set(saved.get(entryKey(el)));
					for (const option of el.options) option.selected = selected.has(option.value);
				} else el.value = saved.get(entryKey(el));
			}
			writeDraft(form, false);
			if (!announce) return;
			const note = document.createElement("p");
			note.className = "draft-notice alert alert-info py-2 text-sm";
			note.setAttribute("role", "status");
			note.textContent = `${msg("draft-restored")} `;
			const discard = createAction("button", "btn btn-outline btn-xs");
			discard.type = "button";
			discard.textContent = msg("discard-draft");
			discard.addEventListener("click", () => {
				storage?.removeItem(draftKey(form));
				form.reset();
				for (const el of fieldsOf(form)) {
					if (checkable(el)) continue;
					if (el instanceof HTMLSelectElement && el.multiple) {
						for (const option of el.options) option.selected = option.defaultSelected;
					} else el.value = el.defaultValue;
				}
				note.remove();
				updateDirty(form);
			});
			note.appendChild(discard);
			form.prepend(note);
		}

		// Password strength meter and reveal toggle.
		function enhancePassword(el) {
			if (el.dataset.enhanced) return;
			el.dataset.enhanced = "true";
			const toggle = createAction("button", "btn btn-outline btn-xs reveal");
			toggle.type = "button";
			toggle.setAttribute("aria-controls", idFor(el, "password"));
			toggle.setAttribute("aria-pressed", "false");
			toggle.setAttribute("aria-label", msg("show-password"));
			const eye = document.createElementNS("http://www.w3.org/2000/svg", "svg");
			eye.setAttribute("class", "reveal-eye");
			eye.setAttribute("width", "16");
			eye.setAttribute("height", "16");
			eye.setAttribute("viewBox", "0 0 24 24");
			eye.setAttribute("fill", "none");
			eye.setAttribute("stroke", "currentColor");
			eye.setAttribute("stroke-width", "1.75");
			eye.setAttribute("stroke-linecap", "round");
			eye.setAttribute("stroke-linejoin", "round");
			eye.setAttribute("aria-hidden", "true");
			eye.setAttribute("focusable", "false");
			eye.innerHTML = revealIconPaths(false);
			toggle.append(eye);
			toggle.addEventListener("click", () => {
				const show = el.type === "password";
				el.type = show ? "text" : "password";
				toggle.setAttribute("aria-pressed", String(show));
				toggle.setAttribute("aria-label", msg(show ? "hide-password" : "show-password"));
				eye.innerHTML = revealIconPaths(show);
				el.focus();
			});
			// shortcut: every password input in the templ trees is a bare .input, never an icon label wrap
			const box = document.createElement("span");
			box.className = "reveal-box";
			el.replaceWith(box);
			box.append(el, toggle);
			if (el.dataset.strength === undefined) return;
			const wrap = document.createElement("p");
			wrap.className = "strength";
			const meter = document.createElement("meter");
			Object.assign(meter, { min: 0, max: 4, low: 2, high: 3, optimum: 4, value: 0 });
			meter.setAttribute("aria-label", msg("strength"));
			const text = document.createElement("span");
			text.setAttribute("aria-live", "polite");
			wrap.append(meter, text);
			box.insertAdjacentElement("afterend", wrap);
			const update = () => {
				const score = passwordStrength(el.value);
				meter.value = score;
				text.textContent = el.value ? msg(`strength-${score}`) : "";
			};
			el.addEventListener("input", update);
			update();
		}

		// Wizard steps and progressive disclosure.
		function revealStep(el) {
			const form = el.form || el.closest("form");
			if (!form?._wizard) return;
			const index = form._wizard.steps.findIndex((step) => step.contains(el));
			if (index >= 0) form._wizard.show(index, false);
		}

		function enhanceWizard(form) {
			const steps = [...form.querySelectorAll("fieldset[data-step]")];
			if (steps.length < 2 || form._wizard) return;
			const submit = form.querySelector('button[type="submit"]');
			const nav = document.createElement("div");
			nav.className = "wizard-nav mt-4 flex gap-2";
			const status = document.createElement("p");
			status.className = "wizard-status text-base-content/60 font-bold";
			status.setAttribute("aria-live", "polite");
			const back = Object.assign(createAction("button", "btn btn-outline btn-sm"), { type: "button", textContent: msg("back") });
			const next = Object.assign(createAction("button", "btn btn-primary btn-sm"), { type: "button", textContent: msg("next") });
			nav.append(back, next);
			form.insertBefore(status, steps[0]);
			submit.insertAdjacentElement("beforebegin", nav);
			let current = 0;
			const show = (index, focus) => {
				current = index;
				steps.forEach((step, i) => { step.hidden = i !== index; });
				const last = index === steps.length - 1;
				back.hidden = index === 0;
				next.hidden = last;
				submit.hidden = !last;
				status.textContent = msg("step-of").replace("{step}", index + 1).replace("{total}", steps.length);
				if (focus) {
					const legend = steps[index].querySelector("legend");
					legend.tabIndex = -1;
					legend.focus();
				}
			};
			const stepValid = () => {
				const invalid = validateFields(controls(steps[current]));
				if (invalid.length) invalid[0].focus();
				return invalid.length === 0;
			};
			next.addEventListener("click", () => {
				const target = wizardMove(current, steps.length, "next", stepValid());
				if (target !== current) show(target, true);
			});
			back.addEventListener("click", () => show(wizardMove(current, steps.length, "back", true), true));
			form._wizard = { steps, show, isLast: () => current === steps.length - 1, advance: () => next.click() };
			show(0, false);
		}

		function enhanceDisclosure(section) {
			const [id, value] = section.dataset.showWhen.split("=");
			const control = document.getElementById(id);
			if (!control || section.dataset.enhanced) return;
			section.dataset.enhanced = "true";
			const apply = () => { section.hidden = control.value !== value; };
			control.addEventListener("change", apply);
			apply();
		}

		// Lazy-loaded page sections.
		async function loadLazy(box) {
			if (box.dataset.loading) return;
			box.dataset.loading = "true";
			try {
				const response = await fetch(box.dataset.lazySrc, { headers: { "HX-Request": "true" }, credentials: "same-origin" });
				const responseURL = new URL(response.url, location.href);
				if (!response.ok || response.redirected) {
					if (response.redirected && responseURL.pathname === "/login") box.querySelector("a")?.setAttribute("href", response.url);
					throw new Error(String(response.status));
				}
				const doc = new DOMParser().parseFromString(await response.text(), "text/html");
				const fresh = doc.body.firstElementChild;
				if (!isAuditFragment(response, location.href, fresh?.id) && !isLazyFragment(response, location.href, fresh?.id, box.id)) throw new Error("invalid fragment");
				// htmx only binds attributes it swaps itself, so the fragment is processed here, or its sort links stay plain.
				const node = document.importNode(fresh, true);
				box.replaceWith(node);
				window.htmx?.process(node);
				document.dispatchEvent(new CustomEvent("list:results-changed"));
			} catch (_) {
				// The results are already shown when the failure came after the swap, so there is nothing to retry.
				if (!box.isConnected) return;
				// A failed section shows its message and a retry control, not the loading state.
				delete box.dataset.loading;
				box.removeAttribute("aria-busy");
				box.querySelector(".loading")?.classList.remove("loading");
				const status = box.querySelector('[role="status"]');
				if (status) status.textContent = msg("load-failed");
				if (box.querySelector(".lazy-retry")) return;
				const retry = createAction("button", "btn btn-outline btn-xs lazy-retry");
				retry.type = "button";
				retry.textContent = msg("retry");
				retry.addEventListener("click", () => {
					retry.remove();
					box.setAttribute("aria-busy", "true");
					if (status) status.textContent = msg("loading");
					loadLazy(box);
				});
				box.append(retry);
			}
		}

		// Enhance every form in a subtree; runs on load and after each htmx swap.
		function enhance(root, announce) {
			for (const form of root.querySelectorAll("form")) {
				if (form.dataset.enhanced) continue;
				form.dataset.enhanced = "true";
				form.noValidate = true;
				if (form.matches("[data-dirty-track], [data-unsaved-warning]")) {
					initial.set(form, entriesOf(form, true));
					if (form.hasAttribute("data-dirty-on-load")) {
						dirtyOnLoad.add(form);
						form.dataset.dirty = "true";
					}
					updateDirty(form);
				}
				if (form.dataset.persist) applyDraft(form, announce);
				if (form.matches("[data-wizard]")) enhanceWizard(form);
			}
			root.querySelectorAll('input[type="password"]').forEach(enhancePassword);
			root.querySelectorAll("[data-show-when]").forEach(enhanceDisclosure);
			root.querySelectorAll("[data-lazy-src]").forEach(loadLazy);
			for (const el of root.querySelectorAll('[data-default="timezone"]')) {
				if (!el.value) {
					try { el.value = Intl.DateTimeFormat().resolvedOptions().timeZone || ""; } catch (_) {}
				}
			}
		}

		// ---- Event handlers ----
		document.addEventListener("click", async (event) => {
			const button = event.target instanceof Element ? event.target.closest("button[data-copy-target]") : null;
			if (!button) return;
			const field = document.getElementById(button.dataset.copyTarget);
			if (!(field instanceof HTMLInputElement)) return;
			if (await copyIntakeLink(field, navigator.clipboard, window.location.origin)) {
				const status = button.parentElement.querySelector("[data-copy-status]");
				if (status) status.textContent = button.dataset.copySuccess;
			}
		});

		document.addEventListener("input", (event) => {
			const el = event.target;
			if (!(el instanceof Element) || !el.form) return;
			if (el.dataset.mask === "digits") el.value = maskDigits(el.value, Number(el.dataset.maskLength) || 6);
			if (el.dataset.mask === "upper") el.value = maskUpper(el.value);
			if (el.getAttribute("aria-invalid") === "true") validateField(el);
			for (const other of el.form.querySelectorAll(`[data-match="${el.id}"]`)) if (other.value) validateField(other);
			updateDirty(el.form);
			if (el.form.dataset.persist) writeDraft(el.form, false);
		});

		document.addEventListener("change", (event) => {
			const el = event.target;
			if (!(el instanceof Element) || !el.form) return;
			updateDirty(el.form);
			if (el.form.dataset.persist) writeDraft(el.form, false);
			// Autosave: a select that saves its row as soon as it changes.
			const button = el.dataset.autosave && el.form.querySelector(`button[value="${el.dataset.autosave}"]`);
			if (button && el.form.checkValidity()) el.form.requestSubmit(button);
		});

		// Inline validation after the user leaves a field they touched.
		document.addEventListener("focusout", (event) => {
			const el = event.target;
			if (el instanceof Element && el.form && el.willValidate && el.value !== "") validateField(el);
		});

		// Confirmation dialog, then optimistic removal.
		document.addEventListener("click", (event) => {
			const button = event.target instanceof Element ? event.target.closest("button[data-confirm]") : null;
			if (!button) return;
			if (!window.confirm(button.dataset.confirm)) {
				event.preventDefault();
				event.stopImmediatePropagation();
				return;
			}
			if (button.dataset.optimistic === "remove" && button.form?.hasAttribute("hx-post")) {
				const row = button.closest("li, tr");
				if (row) {
					row.dataset.optimistic = optimisticOutcome("pending");
					button.form._optimisticRow = row;
				}
			}
		}, true);

		// Submit: forgiving format, validation with summary, wizard steps, busy state.
		document.addEventListener("submit", (event) => {
			const form = event.target;
			if (!(form instanceof HTMLFormElement)) return;
			if (form.dataset.submitting === "true") {
				event.preventDefault();
				event.stopImmediatePropagation();
				return;
			}
			if (form._wizard && !form._wizard.isLast()) {
				event.preventDefault();
				event.stopImmediatePropagation();
				form._wizard.advance();
				return;
			}
			const invalid = validateFields(controls(form));
			if (invalid.length) {
				event.preventDefault();
				event.stopImmediatePropagation();
				if (form._optimisticRow) {
					delete form._optimisticRow.dataset.optimistic;
					form._optimisticRow = null;
				}
				revealStep(invalid[0]);
				showSummary(form, invalid);
				return;
			}
			form.querySelector(".error-summary")?.remove();
			for (const el of form.querySelectorAll("input[data-enhanced]")) el.type = "password";
			if (form.dataset.persist) writeDraft(form, true);
			// The file upload form runs its own state machine (file-upload-form skill).
			if (form.matches("[data-file-upload]")) return;
			// Block a second submit now; disable the buttons after the browser and htmx
			// have read the submitter's value.
			form.dataset.submitting = "true";
			setTimeout(() => setBusy(form, true), 0);
		}, true);

		window.addEventListener("beforeunload", (event) => {
			const dirty = [...document.querySelectorAll("form[data-unsaved-warning]")]
				.some((form) => form.dataset.dirty === "true" && form.dataset.submitting !== "true");
			if (dirty) {
				event.preventDefault();
				event.returnValue = "";
			}
		});

		// A page restored from the back/forward cache must not stay busy.
		window.addEventListener("pageshow", (event) => {
			if (event.persisted) document.querySelectorAll('form[aria-busy="true"]').forEach((form) => setBusy(form, false));
		});

		// Keyboard: Escape closes an open menu or the newest toast.
		document.addEventListener("keydown", (event) => {
			if (event.key !== "Escape") return;
			const open = event.target instanceof Element ? event.target.closest("details[open]") : null;
			if (open) {
				open.open = false;
				open.querySelector("summary")?.focus();
				return;
			}
			document.querySelector(".toasts .toast:last-child")?.remove();
		});

		// htmx: remember focus across swaps, restore busy forms, revert optimistic rows.
		let focusedId = "";
		document.addEventListener("htmx:before:request", () => {
			const active = document.activeElement;
			focusedId = active?.id || active?.form?.querySelector('input:not([type="hidden"]), select, textarea')?.id || "";
		});
		document.addEventListener("htmx:finally:request", (event) => {
			const ctx = event.detail?.ctx;
			const form = ctx?.sourceElement?.closest?.("form") || ctx?.sourceElement;
			const status = ctx?.response?.status;
			if (form instanceof HTMLFormElement) {
				if (form.isConnected) setBusy(form, false);
				const row = form._optimisticRow;
				if (row) {
					if (optimisticOutcome(status) === "restored") delete row.dataset.optimistic;
					form._optimisticRow = null;
				}
			}
			if (!(typeof status === "number" && status < 500)) toast(msg("request-failed"), "error");
		});
		document.addEventListener("htmx:after:swap", (event) => {
			enhance(document, false);
			wireCatalogDialog(document);
			wireClientDialog(document);
			wireEngagementDialog(document);
			wireDetailDialogs(document);
			wireDetailSplit(document);
			wireInlineTitles(document);
			wireConfirmSelects(document);
			const alert = document.querySelector('[role="alert"].error');
			const again = focusedId && document.getElementById(focusedId);
			if (alert) {
				alert.tabIndex = -1;
				alert.focus();
			} else if (again) {
				again.focus();
			} else if (event.detail?.ctx?.target?.id === "engagement-client-results" && (focusedId === "engagement-client-prev" || focusedId === "engagement-client-next")) {
				document.getElementById("client-search")?.focus();
			}
		});
		document.addEventListener("app:toast", (event) => toast(event.detail?.value));

		// Realtime list refreshes ask before replacing controls the user is editing.
		window.dynamisForms = Object.assign({}, core, {
			isEditing(root) {
				const active = document.activeElement;
				if (active && root.contains(active)) return true;
				if ([...root.querySelectorAll("details")].some((details) => details.open && details.dataset.userToggled === "true")) return true;
				return [...root.querySelectorAll("form")].some((form) => form.dataset.dirty === "true" || form.dataset.submitting === "true");
			},
			toast,
		});

		enhance(document, true);
		// Full-page errors: move focus to the first error so it is announced.
		const firstError = document.querySelector('.error[role="alert"]');
		if (firstError && !document.querySelector("[autofocus]")) {
			firstError.tabIndex = -1;
			firstError.focus();
		}
	};

	// ---- Realtime list refresh over SSE (app.js) ----

	// wireRealtime: connects the list page's event stream, defers a refresh
	// while the user is editing, and keeps the status line current. htmx
	// loads later in document order, so this waits for it.
	const wireRealtime = () => {
		if (window.__realtimeWired) return;
		const list = document.querySelector("#client-list[data-events]");
		if (!list || !window.EventSource) return;
		if (!window.htmx) { setTimeout(wireRealtime, 25); return; }
		window.__realtimeWired = true;
		if (window.htmx) htmx.config.noSwap = [204, 304, "4xx", "5xx"];
		const listID = list.id;
		const messages = { connecting: list.dataset.liveConnecting, connected: list.dataset.liveConnected, unavailable: list.dataset.liveUnavailable, reconnecting: list.dataset.liveReconnecting };
		let connection = messages.connecting, waiting = false, refreshing = false, pending = false, activeContext = null, cancelledContext = null;
		const current = () => document.querySelector("#" + listID);
		const status = () => document.querySelector("#realtime-status");
		const showStatus = () => { const node = status(); if (node) node.textContent = waiting ? (current()?.dataset.liveWaiting || connection) : connection; };
		const events = new EventSource(list.dataset.events);
		events.addEventListener("open", () => { connection = messages.connected; showStatus(); });
		events.addEventListener("error", () => { connection = events.readyState === EventSource.CLOSED ? messages.unavailable : messages.reconnecting; showStatus(); });
		const refresh = () => {
			const node = current();
			if (!node) return;
			if (window.dynamisForms?.refreshDecision(window.dynamisForms.isEditing(node)) === "defer") { waiting = true; showStatus(); return; }
			waiting = false; showStatus();
			if (refreshing) { pending = true; return; }
			refreshing = true;
			window.htmx?.ajax("GET", location.pathname + location.search, { target: "#" + listID, swap: "outerHTML" });
		};
		const retryWaiting = () => setTimeout(() => { if (waiting) refresh(); }, 0);
		const finish = (ctx) => {
			if (ctx !== activeContext) return;
			activeContext = null; cancelledContext = null; refreshing = false;
			if (pending) { pending = false; refresh(); }
		};
		document.addEventListener("htmx:before:request", (event) => {
			const ctx = event.detail?.ctx;
			if (ctx?.target?.id === listID && ctx.request?.method?.toLowerCase() === "get") { activeContext = ctx; refreshing = true; }
		});
		document.addEventListener("htmx:before:swap", (event) => {
			const ctx = event.detail?.ctx;
			if (ctx?.target?.id !== listID || ctx.request?.method?.toLowerCase() !== "get") return;
			const node = current();
			if (node && window.dynamisForms?.refreshDecision(window.dynamisForms.isEditing(node)) === "defer") { event.preventDefault(); waiting = true; cancelledContext = ctx; showStatus(); }
		});
		document.addEventListener("htmx:after:swap", (event) => {
			const ctx = event.detail?.ctx;
			if (ctx?.target?.id === listID) { showStatus(); finish(ctx); }
		});
		document.addEventListener("htmx:after:request", (event) => {
			const ctx = event.detail?.ctx;
			const status = ctx?.response?.status;
			if (ctx === cancelledContext || event.detail?.failed || status >= 400 || status === 204 || status === 304) finish(ctx);
		});
		for (const name of ["htmx:sendError", "htmx:timeout"]) { document.addEventListener(name, (event) => finish(event.detail?.ctx)); }
		document.addEventListener("htmx:finally:request", (event) => {
			const ctx = event.detail?.ctx;
			if (ctx === cancelledContext || String(ctx?.status || "").startsWith("error:")) finish(ctx);
		});
		document.addEventListener("click", (event) => {
			const summary = event.target instanceof Element ? event.target.closest("summary") : null;
			if (summary?.parentElement?.matches("details")) summary.parentElement.dataset.userToggled = "true";
		});
		document.addEventListener("toggle", (event) => {
			if (event.target instanceof HTMLDetailsElement && event.target.dataset.userToggled) retryWaiting();
		}, true);
		for (const name of ["client.changed", "resync"]) { events.addEventListener(name, refresh); }
		for (const name of ["focusout", "input", "change", "htmx:finally:request"]) { document.addEventListener(name, retryWaiting); }
		events.addEventListener("close", () => events.close());
	};

	// ---- File upload mount (app.js) ----

	const wireFileUploader = () => {
		if (window.__uploaderWired) return;
		window.__uploaderWired = true;
		const fileUploadForm = document.querySelector("form[data-file-upload]");
		if (fileUploadForm) {
			const uploader = new FileUploader(fileUploadForm);
			uploader.init();
		}
	};

	// ---- WebMCP browser tools (app.js) ----

	const wireWebMCP = () => {
		if (window.__webmcpWired) return;
		window.__webmcpWired = true;
		const context = document.modelContext;
		if (!context || typeof context.registerTool !== "function" || !document.querySelector("[data-webmcp-page]")) return;
		const idSchema = { type: "string", pattern: "^[0-9a-f]{32}$" };
		// Member, invitation, and token IDs are short IDs over the ADR 0009 alphabet.
		const shortIdSchema = { type: "string", pattern: "^[2-9a-hjkmnp-z]{4,}$" };
		const textSchema = (maxLength) => ({ type: "string", minLength: 1, maxLength });
		const schemas = {
			"workspace-create-v1": { type: "object", properties: { name: textSchema(120) }, required: ["name"], additionalProperties: false },
			"member-role-update-v1": { type: "object", properties: { memberId: shortIdSchema, role: { type: "string", enum: ["admin", "member", "viewer"] } }, required: ["memberId", "role"], additionalProperties: false },
			"member-remove-v1": { type: "object", properties: { memberId: shortIdSchema }, required: ["memberId"], additionalProperties: false },
			"ownership-transfer-v1": { type: "object", properties: { memberId: shortIdSchema }, required: ["memberId"], additionalProperties: false },
			"invitation-revoke-v1": { type: "object", properties: { invitationId: shortIdSchema }, required: ["invitationId"], additionalProperties: false },
			"token-revoke-v1": { type: "object", properties: { tokenId: shortIdSchema }, required: ["tokenId"], additionalProperties: false },
			"session-revoke-v1": { type: "object", properties: { sessionId: idSchema }, required: ["sessionId"], additionalProperties: false },
			"workspace-export-v1": { type: "object", properties: {}, additionalProperties: false }
		};
		const actions = {
			"workspace-create-v1": "workspace-create", "member-role-update-v1": "member-role-update", "member-remove-v1": "member-remove",
			"ownership-transfer-v1": "ownership-transfer", "invitation-revoke-v1": "invitation-revoke", "token-revoke-v1": "token-revoke",
			"session-revoke-v1": "session-revoke"
		};
		const object = (value) => value && typeof value === "object" && !Array.isArray(value);
		const text = (value, maxLength) => typeof value === "string" && value.length > 0 && value.length <= maxLength;
		const validId = (value) => typeof value === "string" && /^[0-9a-f]{32}$/.test(value);
		const validShortId = (value) => typeof value === "string" && new RegExp(shortIdSchema.pattern).test(value);
		const result = (tool, status) => ({ tool, status });
		const formFor = (action, id) => [...document.querySelectorAll("form[data-webmcp-actions]")]
			.find((form) => form.getClientRects().length > 0 && form.dataset.webmcpActions.split(" ").includes(action) && (!id || form.dataset.webmcpId === id));
		const control = (form, name) => [...form.elements].find((element) => element.name === name && element.type !== "hidden" && !element.disabled);
		const prepare = (tool, form, fields, button) => {
			if (!form) return result(tool, "resource-not-found");
			for (const [name, value] of Object.entries(fields || {})) {
				const element = control(form, name);
				if (!element) return result(tool, "visible-control-not-found");
				element.value = value;
				// Tell the form code the field changed, so dirty tracking enables the Save button.
				element.dispatchEvent(new Event("input", { bubbles: true }));
			}
			const target = button ? form.querySelector(`button[value="${button}"]`) : control(form, Object.keys(fields || {})[0]);
			const focusTarget = target || form.querySelector("button");
			// Row actions can sit in a closed row menu. Open the menus around the prepared control so it can take focus.
			for (let menu = focusTarget?.closest("details"); menu; menu = menu.parentElement?.closest("details")) menu.open = true;
			focusTarget?.focus();
			return result(tool, "ready-for-user-submission");
		};
		const execute = (name, input) => {
			if (!object(input)) return result(name, "invalid-input");
			if (name === "workspace-create-v1") return text(input.name, 120) ? prepare(name, formFor(actions[name]), { name: input.name }) : result(name, "invalid-input");
			const idField = { "member-role-update-v1": "memberId", "member-remove-v1": "memberId", "ownership-transfer-v1": "memberId", "invitation-revoke-v1": "invitationId", "token-revoke-v1": "tokenId", "session-revoke-v1": "sessionId" }[name];
			if (idField) {
				const idValid = name === "session-revoke-v1" ? validId : validShortId;
				if (!idValid(input[idField])) return result(name, "invalid-input");
				if (name === "member-role-update-v1" && !["admin", "member", "viewer"].includes(input.role)) return result(name, "invalid-input");
				const button = name === "member-role-update-v1" ? "role" : name === "member-remove-v1" ? "remove" : name === "ownership-transfer-v1" ? "transfer" : "revoke";
				return prepare(name, formFor(actions[name], input[idField]), name === "member-role-update-v1" ? { role: input.role } : {}, button);
			}
			if (name === "workspace-export-v1") {
				const link = document.querySelector("[data-webmcp-export]");
				if (!link) return result(name, "resource-not-found");
				link.focus();
				return result(name, "ready-for-user-activation");
			}
			return result(name, "invalid-input");
		};
		for (const [name, schema] of Object.entries(schemas)) {
			if (name === "workspace-export-v1" && !document.querySelector("[data-webmcp-export]")) continue;
			if (name !== "workspace-export-v1" && ![...document.querySelectorAll("form[data-webmcp-actions]")].some((form) => form.dataset.webmcpActions.split(" ").includes(actions[name]))) continue;
			try {
				Promise.resolve(context.registerTool({ name, description: "Prepare a visible browser control; the user completes the existing flow.", inputSchema: schema, annotations: { readOnlyHint: name === "workspace-export-v1" }, execute: (input) => execute(name, input) })).catch(() => {});
			} catch (_) {}
		}
	};

	const wireDetailSplit = (root) => {
		for (const split of root.querySelectorAll("[data-detail-split]")) {
			const divider = split.querySelector("[data-detail-divider]");
			const range = split.querySelector("[data-detail-range]");
			if (!divider || split.dataset.detailSplitWired) continue;
			split.dataset.detailSplitWired = "true";
			let sidebar = 384;
			let drag = null;
			const controls = [divider];
			const desktop = window.matchMedia("(min-width: 1024px)");
			const stopDrag = () => {
				if (!drag) return;
				const pointerId = drag.id;
				drag = null;
				if (divider.hasPointerCapture(pointerId)) divider.releasePointerCapture(pointerId);
			};
			const update = () => {
				const width = Math.floor(split.clientWidth);
				const active = desktop.matches && width >= 616;
				for (const control of controls) control.hidden = !active;
				divider.tabIndex = active ? 0 : -1;
				if (!active) {
					if (range) range.hidden = true;
					stopDrag();
					return;
				}
				sidebar = Math.max(280, Math.min(width - 336, Math.round(sidebar)));
				split.style.setProperty("--detail-sidebar-width", `${sidebar}px`);
				divider.setAttribute("aria-valuemin", "320");
				divider.setAttribute("aria-valuemax", String(width - 296));
				divider.setAttribute("aria-valuenow", String(width - 16 - sidebar));
				if (range) {
					range.min = "320";
					range.max = String(width - 296);
					range.value = String(width - 16 - sidebar);
				}
			};
			divider.addEventListener("pointerdown", (event) => {
				if (divider.hidden || drag || !event.isPrimary || event.button !== 0) return;
				drag = { id: event.pointerId, x: event.clientX, sidebar, moved: false };
				divider.setPointerCapture(event.pointerId);
				event.preventDefault();
			});
			divider.addEventListener("pointermove", (event) => {
				if (!drag || event.pointerId !== drag.id) return;
				if (Math.abs(event.clientX - drag.x) > 3) drag.moved = true;
				sidebar = drag.sidebar - (event.clientX - drag.x);
				update();
			});
			divider.addEventListener("pointerup", (event) => {
				if (!drag || event.pointerId !== drag.id) return;
				const openRange = !drag.moved && range;
				stopDrag();
				if (openRange) {
					range.hidden = false;
					range.focus();
				}
			});
			for (const type of ["pointercancel", "lostpointercapture"]) {
				divider.addEventListener(type, (event) => {
					if (drag && event.pointerId === drag.id) stopDrag();
				});
			}
			divider.addEventListener("keydown", (event) => {
				if (divider.hidden) return;
				if (event.key === "ArrowLeft") sidebar += 10;
				else if (event.key === "ArrowRight") sidebar -= 10;
				else if (event.key === "Home") sidebar = split.clientWidth - 336;
				else if (event.key === "End") sidebar = 280;
				else return;
				event.preventDefault();
				update();
			});
			range?.addEventListener("input", () => {
				sidebar = split.clientWidth - 16 - Number(range.value);
				update();
			});
			range?.addEventListener("blur", () => { range.hidden = true; });
			range?.addEventListener("keydown", (event) => {
				if (event.key === "Escape") {
					range.hidden = true;
					divider.focus();
				}
			});
			const resize = () => {
				if (split.isConnected === false) {
					window.removeEventListener("resize", resize);
					observer?.disconnect();
					return;
				}
				update();
			};
			const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(resize);
			observer?.observe(split);
			window.addEventListener("resize", resize);
			update();
		}
	};

	const wireTimelineDialog = (root) => {
		const composer = root.querySelector("[data-timeline-composer]");
		const dialog = root.querySelector("[data-timeline-dialog]");
		const open = root.querySelector("[data-timeline-open]");
		const content = root.querySelector("[data-timeline-modal-content]");
		const close = composer?.querySelector("[data-timeline-close]");
		if (!composer || !dialog || !open || !content || !close || typeof dialog.showModal !== "function") return;
		content.append(composer);
		open.hidden = false;
		close.hidden = false;
		open.addEventListener("click", () => dialog.showModal());
		close.addEventListener("click", () => dialog.close());
		dialog.addEventListener("close", () => open.focus());
		if (composer.hasAttribute("data-timeline-invalid")) {
			root.querySelector("[data-timeline-error-summary]")?.remove();
			dialog.showModal();
		}
	};

	const wireCatalogDialog = (root) => {
		const catalogDialog = root.querySelector("[data-catalog-dialog]");
		const catalogOpen = root.querySelector("[data-catalog-create]");
		const catalogCancel = root.querySelector("[data-catalog-cancel]");
		if (!catalogDialog || !catalogOpen || !catalogCancel || catalogDialog.dataset.bound || typeof catalogDialog.showModal !== "function") return;
		catalogDialog.dataset.bound = "true";
		const initiallyOpen = catalogDialog.open;
		if (initiallyOpen) catalogDialog.removeAttribute("open");
		catalogOpen.addEventListener("click", (event) => { event.preventDefault(); catalogDialog.showModal(); });
		catalogCancel.addEventListener("click", (event) => { event.preventDefault(); catalogDialog.close(); });
		catalogDialog.addEventListener("close", () => catalogOpen.focus());
		if (initiallyOpen) catalogDialog.showModal();
	};
	const wireClientDialog = (root) => {
		const dialog = root.querySelector("[data-client-dialog]");
		if (!dialog || dialog.dataset.bound || typeof dialog.showModal !== "function") return;
		dialog.dataset.bound = "true";
		if (!root._clientDialogBound) {
			root._clientDialogBound = true;
			root.addEventListener("click", (event) => {
				const current = root.querySelector("[data-client-dialog]");
				const open = event.target.closest("[data-client-create]");
				if (open && current) {
					event.preventDefault();
					root._clientDialogOpener = open;
					current.showModal();
					current.querySelector('input[name="display_name"]')?.focus();
				} else if (current && event.target.closest("[data-client-close]")) {
					current.close();
				}
			});
		}
		dialog.addEventListener("click", (event) => { if (event.target === dialog) dialog.close(); });
		dialog.addEventListener("close", () => root._clientDialogOpener?.focus());
	};


	const wireEngagementDialog = (root) => {
		const dialog = root.querySelector("[data-engagement-dialog]");
		const cancel = dialog?.querySelector("[data-engagement-cancel]");
		if (!dialog || !cancel || dialog.dataset.bound || typeof dialog.showModal !== "function") return;
		dialog.dataset.bound = "true";
		cancel.addEventListener("click", (event) => { event.preventDefault(); dialog.close(); });
		dialog.addEventListener("click", (event) => { if (event.target === dialog) dialog.close(); });
		dialog.addEventListener("close", () => root.querySelector("[data-engagement-create]")?.focus());
		dialog.showModal();
	};

	const wireDetailDialogs = (root) => {
		for (const details of root.querySelectorAll("[data-detail-editor]")) {
			const dialog = details.querySelector("[data-detail-dialog]");
			const summary = details.querySelector("summary");
			const form = dialog?.querySelector("form");
			const cancel = dialog?.querySelector("[data-detail-cancel]");
			if (!dialog || !summary || !form || !cancel || dialog.dataset.bound || typeof dialog.showModal !== "function") continue;
			dialog.dataset.bound = "true";
			const initiallyOpen = details.open;
			dialog.removeAttribute("open");
			dialog.classList.remove("relative");
			dialog.classList.add("modal");
			details.open = false;
			cancel.hidden = false;
			const fallbackCancel = dialog.querySelector("[data-detail-fallback-cancel]");
			if (fallbackCancel) fallbackCancel.hidden = true;
			const values = () => JSON.stringify([...form.elements].filter((field) => field.name && !["hidden", "submit", "button"].includes(field.type)).map((field) =>
				[field.name, field.type === "checkbox" || field.type === "radio" ? field.checked : field.value]));
			const initial = values();
			const open = () => {
				details.open = true;
				dialog.showModal();
				dialog.querySelector('[role="alert"], [aria-invalid="true"]')?.focus();
			};
			const dismiss = () => {
				if ((details.hasAttribute("data-detail-dirty-on-load") || values() !== initial) && !window.confirm(details.dataset.detailDiscardMessage)) return;
				dialog.close();
			};
			summary.addEventListener("click", (event) => { event.preventDefault(); open(); });
			cancel.addEventListener("click", (event) => { event.preventDefault(); dismiss(); });
			dialog.addEventListener("cancel", (event) => { event.preventDefault(); dismiss(); });
			dialog.addEventListener("keydown", (event) => { if (event.key === "Escape") event.stopPropagation(); });
			dialog.addEventListener("close", () => { details.open = false; summary.focus(); });
			if (initiallyOpen) open();
		}
	};

	const wireConfirmSelects = (root) => {
		for (const form of root.querySelectorAll("form[data-confirm-select]")) {
			const select = form.querySelector("select");
			const dialog = form.querySelector("[data-confirm-select-dialog]");
			const fallback = form.querySelector("[data-confirm-select-fallback]");
			const value = dialog?.querySelector("[data-confirm-select-value]");
			const cancel = dialog?.querySelector("[data-confirm-select-cancel]");
			const accept = dialog?.querySelector("[data-confirm-select-accept]");
			if (!select || !dialog || !fallback || !value || !cancel || !accept || form.dataset.confirmSelectBound || typeof dialog.showModal !== "function") continue;
			form.dataset.confirmSelectBound = "true";
			fallback.hidden = true;
			const previous = select.selectedIndex;
			const previousValue = select.value;
			let confirmed = false;
			select.addEventListener("change", () => {
				if (select.value === previousValue) return;
				value.textContent = select.selectedOptions[0]?.textContent.trim() || "";
				confirmed = false;
				dialog.showModal();
				cancel.focus();
			});
			cancel.addEventListener("click", () => dialog.close());
			dialog.addEventListener("click", (event) => { if (event.target === dialog) dialog.close(); });
			dialog.addEventListener("close", () => {
				if (!confirmed) {
					select.selectedIndex = previous;
					select.dispatchEvent(new Event("input", { bubbles: true }));
				}
				select.focus();
			});
			accept.addEventListener("click", () => {
				confirmed = true;
				dialog.close();
				form.requestSubmit();
			});
		}
	};

	const wireInlineTitles = (root) => {
		for (const details of root.querySelectorAll("[data-inline-title]")) {
			const summary = details.querySelector("summary");
			const input = details.querySelector("[data-inline-input]");
			const form = details.querySelector("form");
			const cancel = details.querySelector("[data-inline-cancel]");
			if (!summary || !input || !form || !cancel || details.dataset.inlineBound) continue;
			details.dataset.inlineBound = "true";
			let opening = false;
			const dismiss = (restoreFocus = true) => {
				if (details.hasAttribute("data-inline-dirty-on-load")) {
					form.dataset.submitting = "true";
					window.location.assign(cancel.href);
					return;
				}
				input.value = details.dataset.inlineSaved;
				details.removeAttribute("data-inline-dirty-on-load");
				input.dispatchEvent(new Event("input", { bubbles: true }));
				form.dataset.dirty = "";
				details.open = false;
				if (restoreFocus) summary.focus();
			};
			summary.addEventListener("click", (event) => {
				if (!details.open) { opening = true; return; }
				event.preventDefault();
				dismiss();
			});
			details.addEventListener("toggle", () => {
				if (details.open && opening) { input.focus(); input.select(); }
				opening = false;
			});
			cancel.addEventListener("click", (event) => { event.preventDefault(); dismiss(); });
			details.ownerDocument.addEventListener("click", (event) => {
				if (details.open && !details.contains(event.target) && form.dataset.submitting !== "true") dismiss(false);
			});
			details.addEventListener("keydown", (event) => {
				const multiline = input.hasAttribute("data-inline-multiline");
				if (event.key === "Enter" && event.target === input && !event.isComposing &&
					(multiline ? (event.ctrlKey || event.metaKey) && !event.shiftKey : !event.shiftKey)) {
					event.preventDefault();
					if (!details.hasAttribute("data-inline-explicit-retry")) {
						const confirm = form.querySelector("button[type=submit][data-confirm]");
						if (confirm) confirm.click();
						else form.requestSubmit();
					}
					return;
				}
				if (event.key !== "Escape" || !details.open || event.isComposing) return;
				event.preventDefault();
				event.stopPropagation();
				dismiss();
			});
		}
	};


	const engagementClientPicker = () => ({
		init() {
			const root = this.$root;
			const picker = root.querySelector("[data-client-picker]");
			const selected = root.querySelector("[data-selected-client]");
			const current = root.querySelector("[data-client-current]");
			if (!picker || !selected || !current) return;
			root.addEventListener("click", (event) => {
				const option = event.target.closest("[data-client-option]");
				if (!option || !picker.contains(option)) return;
				event.preventDefault();
				selected.value = option.dataset.clientId;
				current.textContent = option.textContent.trim();
				picker.open = false;
				picker.querySelector("summary")?.focus();
			});
		},
	});

	const engagementNotesEditor = () => ({
		init() {
			const root = this.$root;
			const textarea = root.querySelector("textarea");
			const controls = root.querySelector("[data-notes-controls]");
			const tools = root.querySelector("[data-notes-tools]");
			const writePanel = root.querySelector("[data-notes-write-panel]");
			const output = root.querySelector("[data-notes-output]");
			const write = root.querySelector("[data-notes-write]");
			const preview = root.querySelector("[data-notes-preview]");
			const count = root.querySelector("[data-notes-count]");
			const form = root.closest("form");
			if (!textarea || !controls || !tools || !writePanel || !output || !write || !preview || !count || !form) return;
			let revision = 0;
			const updateCount = () => { count.textContent = `${root.dataset.countLabel}: ${textarea.value.length} / ${textarea.maxLength}`; };
			const showWrite = () => {
				revision++;
				writePanel.hidden = false;
				output.hidden = true;
				output.removeAttribute("aria-busy");
				tools.hidden = false;
				write.setAttribute("aria-selected", "true");
				preview.setAttribute("aria-selected", "false");
				write.tabIndex = 0;
				preview.tabIndex = -1;
				write.classList.add("tab-active");
				preview.classList.remove("tab-active");
			};
			textarea.addEventListener("input", () => { updateCount(); if (!output.hidden) showWrite(); });
			tools.addEventListener("click", (event) => {
				const button = event.target.closest("[data-markdown]");
				if (!button) return;
				const formats = { bold: ["**", "**"], italic: ["*", "*"], heading: ["## ", ""], list: ["- ", ""], link: ["[", "](https://)"], code: ["`", "`"] };
				const format = formats[button.dataset.markdown];
				if (!format) return;
				const start = textarea.selectionStart;
				const end = textarea.selectionEnd;
				const lineFormat = button.dataset.markdown === "heading" || button.dataset.markdown === "list";
				const replaceStart = lineFormat ? (start === 0 ? 0 : textarea.value.lastIndexOf("\n", start - 1) + 1) : start;
				const selected = textarea.value.slice(replaceStart, end);
				const replacement = format[0] + (lineFormat ? selected.replace(/\n(?!$)/g, `\n${format[0]}`) : selected) + format[1];
				if (textarea.value.length - (end - replaceStart) + replacement.length > textarea.maxLength) return;
				textarea.setRangeText(replacement, replaceStart, end, "select");
				if (lineFormat) {
					const selectionStart = start + format[0].length;
					const selectionEnd = end + replacement.length - selected.length;
					textarea.setSelectionRange(selectionStart, selectionEnd);
				} else {
					textarea.setSelectionRange(start + format[0].length, start + format[0].length + selected.length);
				}
				textarea.dispatchEvent(new Event("input", { bubbles: true }));
				textarea.focus();
			});
			write.addEventListener("click", showWrite);
			preview.addEventListener("click", async () => {
				const current = ++revision;
				writePanel.hidden = true;
				output.hidden = false;
				tools.hidden = true;
				write.setAttribute("aria-selected", "false");
				preview.setAttribute("aria-selected", "true");
				write.tabIndex = -1;
				preview.tabIndex = 0;
				write.classList.remove("tab-active");
				preview.classList.add("tab-active");
				output.textContent = root.dataset.previewLoading;
				output.setAttribute("aria-busy", "true");
				const body = new FormData();
				body.set("csrf", form.querySelector('[name="csrf"]').value);
				body.set("notes", textarea.value);
				try {
					const response = await fetch(root.dataset.previewUrl, { method: "POST", body });
					if (!response.ok || response.redirected || !response.headers.get("content-type")?.startsWith("text/html")) throw new Error("preview");
					const html = await response.text();
					if (current !== revision) return;
					if (html) output.innerHTML = html;
					else output.textContent = root.dataset.previewEmpty;
				} catch (_) {
					if (current !== revision) return;
					output.textContent = root.dataset.previewFailed;
				} finally {
					if (current === revision) output.removeAttribute("aria-busy");
				}
			});
			for (const tab of [write, preview]) {
				tab.addEventListener("keydown", (event) => {
					if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
					event.preventDefault();
					const target = event.key === "Home" ? write : event.key === "End" ? preview : tab === write ? preview : write;
					target.click();
					target.focus();
				});
			}
			writePanel.setAttribute("role", "tabpanel");
			writePanel.setAttribute("aria-labelledby", "engagement-notes-write-tab");
			updateCount();
			count.hidden = false;
			controls.hidden = false;
		},
	});

	const wireClientDeleteDialog = (root) => {
		const dialog = root.querySelector("[data-client-delete-dialog]");
		if (!dialog || dialog.dataset.bound || typeof dialog.showModal !== "function") return;
		dialog.dataset.bound = "true";
		const warning = dialog.querySelector("[data-client-delete-warning]");
		const names = dialog.querySelector("[data-client-delete-names]");
		const cancel = dialog.querySelector("[data-client-delete-cancel]");
		const confirm = dialog.querySelector("[data-client-delete-confirm]");
		let pending;
		root.addEventListener("click", (event) => {
			const button = event.target.closest?.("button[data-client-delete]");
			if (!button || button.disabled || !button.form) return;
			event.preventDefault();
			event.stopImmediatePropagation();
			pending = button;
			warning.textContent = button.dataset.confirm;
			names.replaceChildren();
			const selected = button.hasAttribute("data-client-bulk")
				? [...button.form.elements].filter((input) => input.name === "client_id" && input.checked)
				: [];
			const labels = button.hasAttribute("data-client-bulk")
				? [...new Map(selected.map((input) => [input.value, input.getAttribute("aria-label")])).values()]
				: [button.dataset.clientName];
			for (const label of labels) {
				const item = root.createElement("li");
				item.textContent = label;
				names.append(item);
			}
			dialog.showModal();
			cancel.focus();
		}, true);
		cancel.addEventListener("click", () => dialog.close());
		dialog.addEventListener("close", () => pending?.focus());
		confirm.addEventListener("click", () => {
			if (!pending || pending.form.dataset.submitting === "true") return;
			const form = pending.form;
			const input = root.createElement("input");
			input.type = "hidden";
			input.name = "confirm_delete";
			input.value = "yes";
			form.append(input);
			dialog.close();
			form.requestSubmit(pending);
		});
	};

	// pageBehaviors: the per-page mount. Every shell body carries it; each
	// wire function self-guards, so pages without a behavior pay nothing.
	const pageBehaviors = () => ({
		init() {
			wireTimelineDialog(document);
			wireCatalogDialog(document);
			wireClientDialog(document);
			wireClientDeleteDialog(document);
			wireDetailDialogs(document);
			wireDetailSplit(document);
			wireInlineTitles(document);
			wireForms();
			wireConfirmSelects(document);
			wireRealtime();
			wireFileUploader();
			wireWebMCP();
		},
		toggleDrawer() {
			const box = document.getElementById("app-drawer");
			if (box) box.checked = !box.checked;
		},
	});

if (typeof module !== "undefined" && module.exports) module.exports = { core, FileUploader, wireForms, wireRealtime, wireTimelineDialog, wireCatalogDialog, wireClientDialog, wireClientDeleteDialog, wireEngagementDialog, wireDetailDialogs, wireDetailSplit, wireInlineTitles, wireConfirmSelects, engagementClientPicker, engagementNotesEditor, notificationsLive, passkeyLogin, passkeyEnroll, listScreen, pageBehaviors, copyIntakeLink };
	if (typeof document === "undefined" || typeof window === "undefined") return;
	// The pinned HTMX runtime dispatches this before reading or parsing the body.
	// Keep downloads, EventSource, API data and external redirects out of swaps.
	document.addEventListener("htmx:before:response", (event) => {
		const response = event.detail?.ctx?.response;
		const clientLocation = response?.headers?.get("X-Client-Location");
		if (clientLocation && event.detail?.ctx?.sourceElement?.closest?.("#client-create-content")) {
			location.assign(clientLocation);
			event.preventDefault();
			return;
		}
		if ([204, 205, 304].includes(response?.status)) return;
		const mediaType = response?.headers?.get("Content-Type")?.trim() ?? "";
		const disposition = response?.headers?.get("Content-Disposition");
		const local = response?.raw?.url?.startsWith(location.origin + "/");
		if (!local || disposition != null || !/^text\/html(?:[ \t]*;[ \t]*charset[ \t]*=[ \t]*(?:utf-8|"utf-8"))?$/i.test(mediaType)) event.preventDefault();
	}, true);
	document.addEventListener("alpine:init", () => {
		window.Alpine.data("notificationsLive", notificationsLive);
		window.Alpine.data("passkeyLogin", passkeyLogin);
		window.Alpine.data("passkeyEnroll", passkeyEnroll);
		window.Alpine.data("listScreen", listScreen);
		window.Alpine.data("engagementClientPicker", engagementClientPicker);
		window.Alpine.data("engagementNotesEditor", engagementNotesEditor);
		window.Alpine.data("pageBehaviors", pageBehaviors);
	});
})();
