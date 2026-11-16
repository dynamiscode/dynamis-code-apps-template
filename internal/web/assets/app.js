if(window.htmx)htmx.config.noSwap=[204,304,"4xx","5xx"];
const list=document.querySelector("#item-list[data-events],#client-list[data-events]");
const realtimeStatus=document.querySelector("#realtime-status");
if(list&&window.EventSource){const messages={connecting:list.dataset.liveConnecting,connected:list.dataset.liveConnected,unavailable:list.dataset.liveUnavailable,reconnecting:list.dataset.liveReconnecting};if(realtimeStatus)realtimeStatus.textContent=messages.connecting;const events=new EventSource(list.dataset.events);events.addEventListener("open",()=>{if(realtimeStatus)realtimeStatus.textContent=messages.connected});events.addEventListener("error",()=>{if(realtimeStatus)realtimeStatus.textContent=events.readyState===EventSource.CLOSED?messages.unavailable:messages.reconnecting});let waiting=false;const refresh=()=>{const current=document.querySelector("#"+list.id);if(current&&window.dynamisForms?.refreshDecision(window.dynamisForms.isEditing(current))==="defer"){waiting=true;if(realtimeStatus)realtimeStatus.textContent=list.dataset.liveWaiting||messages.connected;return}waiting=false;if(realtimeStatus)realtimeStatus.textContent=messages.connected;window.htmx?.ajax("GET",location.pathname+location.search,{target:"#"+list.id,swap:"outerHTML"})};for(const name of[list.id==="client-list"?"client.changed":"item.changed","resync"]){events.addEventListener(name,refresh)}document.addEventListener("focusout",()=>setTimeout(()=>{if(waiting)refresh()},0));document.addEventListener("htmx:finally:request",()=>setTimeout(()=>{if(waiting)refresh()},0));events.addEventListener("close",()=>events.close())}
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
			this.statusElement.className = "status";
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
				statusEl.classList.remove("status");
			} else {
				statusEl.classList.remove("error");
				statusEl.classList.add("status");
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
			this.transition(FileUploader.PHASES.UPLOADING, `${this.getPhaseMessage(FileUploader.PHASES.UPLOADING)} ${announcedPercent}%`);
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

if (typeof window !== "undefined") {
	window.FileUploader = FileUploader;
}
if (typeof module !== "undefined" && module.exports) {
	module.exports = { FileUploader };
}

const fileUploadForm = document.querySelector("form[data-file-upload]");
if (fileUploadForm) {
	const uploader = new FileUploader(fileUploadForm);
	uploader.init();
}

(() => {
	const context = document.modelContext;
	if (!context || typeof context.registerTool !== "function" || !document.querySelector("[data-webmcp-page]")) return;
	const idSchema = {type: "string", pattern: "^[0-9a-f]{32}$"};
	// Member, invitation, and token IDs are short IDs over the ADR 0009 alphabet.
	const shortIdSchema = {type: "string", pattern: "^[2-9a-hjkmnp-z]{4,}$"};
	const textSchema = (maxLength) => ({type: "string", minLength: 1, maxLength});
	const schemas = {
		"workspace-create-v1": {type: "object", properties: {name: textSchema(120)}, required: ["name"], additionalProperties: false},
		"item-create-v1": {type: "object", properties: {title: textSchema(200)}, required: ["title"], additionalProperties: false},
		"item-update-v1": {type: "object", properties: {itemId: shortIdSchema, title: textSchema(200), status: {type: "string", enum: ["active", "complete"]}}, required: ["itemId", "title", "status"], additionalProperties: false},
		"item-delete-v1": {type: "object", properties: {itemId: shortIdSchema}, required: ["itemId"], additionalProperties: false},
		"member-role-update-v1": {type: "object", properties: {memberId: shortIdSchema, role: {type: "string", enum: ["admin", "member", "viewer"]}}, required: ["memberId", "role"], additionalProperties: false},
		"member-remove-v1": {type: "object", properties: {memberId: shortIdSchema}, required: ["memberId"], additionalProperties: false},
		"ownership-transfer-v1": {type: "object", properties: {memberId: shortIdSchema}, required: ["memberId"], additionalProperties: false},
		"invitation-revoke-v1": {type: "object", properties: {invitationId: shortIdSchema}, required: ["invitationId"], additionalProperties: false},
		"token-revoke-v1": {type: "object", properties: {tokenId: shortIdSchema}, required: ["tokenId"], additionalProperties: false},
		"session-revoke-v1": {type: "object", properties: {sessionId: idSchema}, required: ["sessionId"], additionalProperties: false},
		"workspace-export-v1": {type: "object", properties: {}, additionalProperties: false}
	};
	const actions = {
		"workspace-create-v1": "workspace-create", "item-create-v1": "item-create", "item-update-v1": "item-update",
		"item-delete-v1": "item-delete", "member-role-update-v1": "member-role-update", "member-remove-v1": "member-remove",
		"ownership-transfer-v1": "ownership-transfer", "invitation-revoke-v1": "invitation-revoke", "token-revoke-v1": "token-revoke",
		"session-revoke-v1": "session-revoke"
	};
	const object = (value) => value && typeof value === "object" && !Array.isArray(value);
	const text = (value, maxLength) => typeof value === "string" && value.length > 0 && value.length <= maxLength;
	const validId = (value) => typeof value === "string" && /^[0-9a-f]{32}$/.test(value);
	const validShortId = (value) => typeof value === "string" && new RegExp(shortIdSchema.pattern).test(value);
	const result = (tool, status) => ({tool, status});
	const formFor = (action, id) => [...document.querySelectorAll("form[data-webmcp-actions]")]
		.find((form) => form.getClientRects().length > 0 && form.dataset.webmcpActions.split(" ").includes(action) && (!id || form.dataset.webmcpId === id));
	const control = (form, name) => [...form.elements].find((element) => element.name === name && element.type !== "hidden" && !element.disabled);
	const prepare = (tool, form, fields, button) => {
		if (!form) return result(tool, "resource-not-found");
		for (const [name, value] of Object.entries(fields || {})) {
			const element = control(form, name);
			if (!element) return result(tool, "visible-control-not-found");
			element.value = value;
			// Tell forms.js the field changed, so dirty tracking enables the Save button.
			element.dispatchEvent(new Event("input", {bubbles: true}));
		}
		const target = button ? form.querySelector(`button[value="${button}"]`) : control(form, Object.keys(fields || {})[0]);
		(target || form.querySelector("button"))?.focus();
		return result(tool, "ready-for-user-submission");
	};
	const execute = (name, input) => {
		if (!object(input)) return result(name, "invalid-input");
		if (name === "workspace-create-v1") return text(input.name, 120) ? prepare(name, formFor(actions[name]), {name: input.name}) : result(name, "invalid-input");
		if (name === "item-create-v1") return text(input.title, 200) ? prepare(name, formFor(actions[name]), {title: input.title}) : result(name, "invalid-input");
		if (name === "item-update-v1") return validShortId(input.itemId) && text(input.title, 200) && ["active", "complete"].includes(input.status)
			? prepare(name, formFor(actions[name], input.itemId), {title: input.title, status: input.status}, "update") : result(name, "invalid-input");
		if (name === "item-delete-v1") return validShortId(input.itemId) ? prepare(name, formFor(actions[name], input.itemId), {}, "delete") : result(name, "invalid-input");
		const idField = {"member-role-update-v1": "memberId", "member-remove-v1": "memberId", "ownership-transfer-v1": "memberId", "invitation-revoke-v1": "invitationId", "token-revoke-v1": "tokenId", "session-revoke-v1": "sessionId"}[name];
		if (idField) {
			const idValid = name === "session-revoke-v1" ? validId : validShortId;
			if (!idValid(input[idField])) return result(name, "invalid-input");
			if (name === "member-role-update-v1" && !["admin", "member", "viewer"].includes(input.role)) return result(name, "invalid-input");
			const button = name === "member-role-update-v1" ? "role" : name === "member-remove-v1" ? "remove" : name === "ownership-transfer-v1" ? "transfer" : "revoke";
			return prepare(name, formFor(actions[name], input[idField]), name === "member-role-update-v1" ? {role: input.role} : {}, button);
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
		if (name !== "workspace-export-v1" && !formFor(actions[name])) continue;
		try {
			Promise.resolve(context.registerTool({name, description: "Prepare a visible browser control; the user completes the existing flow.", inputSchema: schema, annotations: {readOnlyHint: name === "workspace-export-v1"}, execute: (input) => execute(name, input)})).catch(() => {});
		} catch (_) {}
	}
})();
