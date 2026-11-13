// forms.js: progressive enhancement for every server-rendered form.
//
// Every page works without this file: forms post, the server validates, and
// errors render as role="alert". This file only adds feedback on top. The rules
// it must keep are in the form-drafts, form-field-input, form-wizard and
// async-feedback skills. The pure evaluators below decide; the DOM code only
// applies their answers.
(() => {
	"use strict";

	// Matches validateNewPassword in internal/identity/account.go.
	const MIN_PASSWORD = 12;

	// ---- Pure evaluators (exported for the exhaustive tests) ----

	// 0 means the server would reject it; 1..4 grow with length and variety.
	function passwordStrength(value) {
		if (typeof value !== "string" || value.length < MIN_PASSWORD) return 0;
		if (/^(.)\1+$/.test(value)) return 1;
		const classes = [/[a-z]/, /[A-Z]/, /[0-9]/, /[^A-Za-z0-9]/].filter((r) => r.test(value)).length;
		let score = 1;
		if (value.length >= 16) score++;
		if (classes >= 3) score++;
		if (value.length >= 20 && classes >= 2) score++;
		return Math.min(4, score);
	}

	// Authenticator codes: keep digits only, at most `length`. Matches
	// normalizeTOTPCode in internal/identity/mfa.go for any code it accepts.
	function maskDigits(value, length) {
		return String(value).replace(/\D/g, "").slice(0, length);
	}

	// Recovery codes: upper case, no spaces. Matches normalizeRecoveryCode.
	function maskUpper(value) {
		return String(value).toUpperCase().replace(/\s+/g, "");
	}

	// What to do with a stored draft when a page with its form loads.
	function draftAction(stored, pageHasError) {
		if (!stored) return "none";
		if (stored.submitted && !pageHasError) return "discard";
		return "restore";
	}

	function serialize(entries) {
		return JSON.stringify(entries);
	}

	function isDirty(initial, current) {
		return serialize(initial) !== serialize(current);
	}

	// Wizard: the next step index. Moving forward needs a valid current step.
	function wizardMove(step, total, direction, currentValid) {
		if (direction === "next") return currentValid && step < total - 1 ? step + 1 : step;
		if (direction === "back") return step > 0 ? step - 1 : step;
		return step;
	}

	// Realtime list refresh: never replace a list the user is editing.
	function refreshDecision(editing) {
		return editing ? "defer" : "refresh";
	}

	// Optimistic removal: keep the row hidden only while the request is pending
	// or after it succeeded; show it again on any failure.
	function optimisticOutcome(status) {
		if (status === "pending") return "hidden";
		return typeof status === "number" && status >= 200 && status < 400 ? "removed" : "restored";
	}

	const core = {
		MIN_PASSWORD, passwordStrength, maskDigits, maskUpper, draftAction, isDirty, wizardMove,
		refreshDecision, optimisticOutcome,
	};
	if (typeof module !== "undefined" && module.exports) module.exports = core;
	if (typeof document === "undefined" || typeof window === "undefined") return;

	// ---- DOM wiring ----

	const script = document.currentScript;
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
	const entriesOf = (form, withSecrets) => fieldsOf(form, withSecrets).map((el) => [entryKey(el), checkable(el) ? el.checked : el.value]);
	const visible = (el) => !el.closest("[hidden]");
	const pageHasError = () => !!document.querySelector('.error[role="alert"], .error-summary');

	// Toasts: one polite live region; Escape or the button dismisses.
	function toast(text, kind) {
		if (!text) return;
		let region = document.querySelector(".toasts");
		if (!region) {
			region = document.createElement("div");
			region.className = "toasts";
			region.setAttribute("role", "status");
			region.setAttribute("aria-live", "polite");
			document.body.appendChild(region);
		}
		const item = document.createElement("p");
		item.className = `toast${kind === "error" ? " error" : ""}`;
		item.textContent = text;
		const close = document.createElement("button");
		close.type = "button";
		close.className = "secondary";
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
				note.className = "field-error";
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
			const link = document.createElement("a");
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
				}
			} else if (button.dataset.wasDisabled !== undefined) {
				button.disabled = button.dataset.wasDisabled === "true";
				if (button.dataset.label !== undefined) button.textContent = button.dataset.label;
				delete button.dataset.wasDisabled;
				delete button.dataset.label;
			}
		}
		if (!busy) updateDirty(form);
	}

	// Dirty tracking, enable-by-change, and the unsaved-changes warning.
	const initial = new WeakMap();
	function updateDirty(form) {
		if (!initial.has(form)) return;
		const dirty = isDirty(initial.get(form), entriesOf(form, true));
		form.dataset.dirty = dirty ? "true" : "";
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
		const empty = entries.every(([, v]) => v === "" || v === false);
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
			else el.value = saved.get(entryKey(el));
		}
		writeDraft(form, false);
		if (!announce) return;
		const note = document.createElement("p");
		note.className = "draft-notice";
		note.setAttribute("role", "status");
		note.textContent = `${msg("draft-restored")} `;
		const discard = document.createElement("button");
		discard.type = "button";
		discard.className = "secondary";
		discard.textContent = msg("discard-draft");
		discard.addEventListener("click", () => {
			storage?.removeItem(draftKey(form));
			form.reset();
			for (const el of fieldsOf(form)) if (!checkable(el)) el.value = el.defaultValue;
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
		const toggle = document.createElement("button");
		toggle.type = "button";
		toggle.className = "secondary reveal";
		toggle.setAttribute("aria-controls", idFor(el, "password"));
		toggle.setAttribute("aria-pressed", "false");
		toggle.textContent = msg("show-password");
		toggle.addEventListener("click", () => {
			const show = el.type === "password";
			el.type = show ? "text" : "password";
			toggle.setAttribute("aria-pressed", String(show));
			toggle.textContent = msg(show ? "hide-password" : "show-password");
			el.focus();
		});
		el.insertAdjacentElement("afterend", toggle);
		if (el.dataset.strength === undefined) return;
		const wrap = document.createElement("p");
		wrap.className = "strength";
		const meter = document.createElement("meter");
		Object.assign(meter, { min: 0, max: 4, low: 2, high: 3, optimum: 4, value: 0 });
		meter.setAttribute("aria-label", msg("strength"));
		const text = document.createElement("span");
		text.setAttribute("aria-live", "polite");
		wrap.append(meter, text);
		toggle.insertAdjacentElement("afterend", wrap);
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
		nav.className = "wizard-nav";
		const status = document.createElement("p");
		status.className = "wizard-status";
		status.setAttribute("aria-live", "polite");
		const back = Object.assign(document.createElement("button"), { type: "button", className: "secondary", textContent: msg("back") });
		const next = Object.assign(document.createElement("button"), { type: "button", textContent: msg("next") });
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
			if (!response.ok) throw new Error(String(response.status));
			const doc = new DOMParser().parseFromString(await response.text(), "text/html");
			const fresh = doc.body.firstElementChild;
			if (!fresh) throw new Error("empty");
			box.replaceWith(document.importNode(fresh, true));
		} catch (_) {
			box.removeAttribute("aria-busy");
			const status = box.querySelector('[role="status"]');
			if (status) status.textContent = msg("load-failed");
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
			const row = button.closest("li");
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
		const alert = document.querySelector('[role="alert"].error');
		const again = focusedId && document.getElementById(focusedId);
		if (alert) {
			alert.tabIndex = -1;
			alert.focus();
		} else if (again) {
			again.focus();
		}
	});
	document.addEventListener("app:toast", (event) => toast(event.detail?.value));

	// The realtime item list asks before replacing rows the user is editing.
	window.dynamisForms = Object.assign({}, core, {
		isEditing(root) {
			const active = document.activeElement;
			const typing = active && root.contains(active) && ["INPUT", "SELECT", "TEXTAREA"].includes(active.tagName);
			return typing || [...root.querySelectorAll("form")].some((form) => form.dataset.dirty === "true" || form.dataset.submitting === "true");
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
})();
