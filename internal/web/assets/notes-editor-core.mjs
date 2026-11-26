import { safeLinkURL, safeVisualSource } from "./notes-editor-safety.mjs";

export function wireNotesEditor(root, create) {
	const details = root;
	const form = root.querySelector("[data-notes-form]");
	const source = root.querySelector("[data-notes-source]");
	const rich = root.querySelector("[data-notes-rich]");
	const toolbar = root.querySelector("[data-notes-toolbar]");
	const warning = root.querySelector("[data-notes-warning]");
	const cancel = root.querySelector("[data-notes-cancel]");
	const summary = root.querySelector("summary");
	const sourceMode = root.querySelector("[data-notes-source-mode]");
	const link = root.querySelector("[data-notes-link]");
	const linkPanel = root.querySelector("[data-notes-link-panel]");
	const linkURL = root.querySelector("[data-notes-link-url]");
	const linkError = root.querySelector("[data-notes-link-error]");
	const linkApply = root.querySelector("[data-notes-link-apply]");
	const linkCancel = root.querySelector("[data-notes-link-cancel]");
	if (!form || !source || !rich || !toolbar || !warning || !cancel || !summary || !sourceMode || !link || !linkPanel || !linkURL || !linkError || !linkApply || !linkCancel) return;
	let editor;
	let original;
	let changed = false;
	const buttons = [...toolbar.querySelectorAll("[data-notes-format]")];
	const notify = () => source.dispatchEvent(new Event("input", { bubbles: true }));
	const sync = () => {
		if (editor && changed) {
			source.value = editor.getMarkdown();
			notify();
		}
	};
	const closeLink = () => {
		linkPanel.hidden = true;
		link.setAttribute("aria-expanded", "false");
		linkError.hidden = true;
		linkURL.removeAttribute("aria-invalid");
	};
	const useSource = (showWarning) => {
		sync();
		if (editor) { editor.destroy(); editor = undefined; }
		rich.hidden = true;
		toolbar.hidden = false;
		for (const button of buttons) { button.disabled = true; button.setAttribute("aria-pressed", "false"); }
		link.disabled = true;
		sourceMode.setAttribute("aria-pressed", "true");
		closeLink();
		source.hidden = false;
		warning.hidden = !showWarning;
	};
	const renderButtons = () => {
		for (const button of buttons) {
			const format = button.dataset.notesFormat;
			const active = format === "list" ? editor.state.isActive.bulletList() : editor.state.isActive.mark(format);
			button.setAttribute("aria-pressed", String(active));
		}
		link.disabled = !editor.state.can.setLink();
	};
	const open = () => {
		original = source.value;
		changed = false;
		if (!safeVisualSource(original)) { useSource(true); source.focus(); return; }
		try {
			editor = create({ element: rich, markdown: original, ariaLabel: source.labels?.[0]?.textContent || "" });
			if (editor.getMarkdown() !== original) { useSource(true); source.focus(); return; }
			editor.subscribe("update", () => { changed = true; sync(); });
			editor.subscribe("state", renderButtons);
			source.hidden = true;
			rich.hidden = false;
			for (const button of buttons) button.disabled = false;
			sourceMode.setAttribute("aria-pressed", "false");
			toolbar.hidden = false;
			warning.hidden = true;
			renderButtons();
			editor.focus();
		} catch (_) {
			useSource(true);
			source.focus();
		}
	};
	const dismiss = (focus = true) => {
		if (details.hasAttribute("data-notes-draft")) {
			form.dataset.submitting = "true";
			window.location.assign(cancel.href);
			return;
		}
		useSource(false);
		source.value = details.dataset.notesSaved;
		notify();
		form.dataset.dirty = "";
		details.open = false;
		if (focus) summary.focus();
	};
	summary.addEventListener("click", () => { if (!details.open) queueMicrotask(open); });
	cancel.addEventListener("click", (event) => { event.preventDefault(); dismiss(); });
	root.ownerDocument.addEventListener("click", (event) => {
		if (details.open && !details.contains(event.target) && form.dataset.submitting !== "true") dismiss(false);
	});
	root.addEventListener("keydown", (event) => {
		if (event.key !== "Escape" || event.isComposing || !details.open) return;
		event.preventDefault();
		event.stopPropagation();
		if (!linkPanel.hidden) { closeLink(); editor?.focus(); return; }
		dismiss();
	});
	toolbar.addEventListener("mousedown", (event) => {
		if (event.target.closest("[data-notes-format], [data-notes-link]")) event.preventDefault();
	});
	toolbar.addEventListener("click", (event) => {
		const button = event.target.closest("[data-notes-format]");
		if (!button || !editor) return;
		if (button.dataset.notesFormat === "list") editor.commands.toggleBulletList();
		else editor.commands.toggleMark(button.dataset.notesFormat);
	});
	sourceMode.addEventListener("click", () => {
		if (editor) { useSource(false); source.focus(); }
		else open();
	});
	link.addEventListener("click", () => {
		if (!editor || !editor.state.can.setLink()) return;
		linkPanel.hidden = false;
		link.setAttribute("aria-expanded", "true");
		linkURL.value = "";
		linkError.hidden = true;
		linkURL.removeAttribute("aria-invalid");
		linkURL.focus();
	});
	linkApply.addEventListener("click", () => {
		if (!editor) return;
		if (!safeLinkURL(linkURL.value) || !editor.commands.setLink(linkURL.value)) {
			linkError.hidden = false;
			linkURL.setAttribute("aria-invalid", "true");
			linkURL.focus();
			return;
		}
		closeLink();
		editor.focus();
	});
	linkCancel.addEventListener("click", () => { closeLink(); editor?.focus(); });
	linkURL.addEventListener("keydown", (event) => {
		if (event.key !== "Enter" || event.isComposing) return;
		event.preventDefault();
		linkApply.click();
	});
	form.addEventListener("submit", (event) => {
		if (!editor) return;
		sync();
		let probe;
		let stable = false;
		try {
			probe = create({ element: root.ownerDocument.createElement("div"), markdown: source.value });
			stable = probe.getMarkdown() === source.value;
		} catch (_) {
			stable = false;
		} finally {
			try { probe?.destroy(); } catch (_) { stable = false; }
		}
		if (!stable || (source.maxLength >= 0 && source.value.length > source.maxLength)) {
			event.preventDefault();
			event.stopPropagation();
			useSource(true);
			source.focus();
		}
	});
	if (details.open) open();
}
