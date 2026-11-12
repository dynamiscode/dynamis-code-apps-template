if(window.htmx)htmx.config.noSwap=[204,304,"4xx","5xx"];
document.addEventListener("htmx:after:swap",()=>document.querySelector('[role="alert"]')?.focus());
const list=document.querySelector("#item-list[data-events]");
const realtimeStatus=document.querySelector("#realtime-status");
if(list&&window.EventSource){const messages={connecting:list.dataset.liveConnecting,connected:list.dataset.liveConnected,unavailable:list.dataset.liveUnavailable,reconnecting:list.dataset.liveReconnecting};if(realtimeStatus)realtimeStatus.textContent=messages.connecting;const events=new EventSource(list.dataset.events);events.addEventListener("open",()=>{if(realtimeStatus)realtimeStatus.textContent=messages.connected});events.addEventListener("error",()=>{if(realtimeStatus)realtimeStatus.textContent=events.readyState===EventSource.CLOSED?messages.unavailable:messages.reconnecting});for(const name of["item.changed","resync"]){events.addEventListener(name,()=>window.htmx?.ajax("GET",location.pathname,{target:"#item-list",swap:"outerHTML"}))}events.addEventListener("close",()=>events.close())}
document.addEventListener("click",event=>{const button=event.target instanceof Element?event.target.closest("button[data-confirm]"):null;if(button&&!window.confirm(button.dataset.confirm))event.preventDefault()});
class PresignedUploader {
	static PHASES = Object.freeze({
		IDLE: "IDLE",
		INITIATING: "INITIATING",
		UPLOADING: "UPLOADING",
		COMPLETING: "COMPLETING",
		SUCCESS: "SUCCESS",
		ERROR: "ERROR"
	});

	static PHASE_MESSAGES = Object.freeze({
		IDLE: "",
		INITIATING: "Initiating upload...",
		UPLOADING: "Uploading file...",
		COMPLETING: "Completing upload...",
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
		this.phase = PresignedUploader.PHASES.IDLE;
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
		return PresignedUploader.PHASE_MESSAGES[phase] || "";
	}

	isInFlight() {
		return this.phase === PresignedUploader.PHASES.INITIATING ||
			this.phase === PresignedUploader.PHASES.UPLOADING ||
			this.phase === PresignedUploader.PHASES.COMPLETING ||
			this.phase === PresignedUploader.PHASES.SUCCESS;
	}

	transition(phase, message) {
		this.phase = phase;
		const isError = phase === PresignedUploader.PHASES.ERROR;
		const inFlight = this.isInFlight();

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

	async handleSubmit(event) {
		if (event) event.preventDefault();
		if (this.isInFlight()) return;

		const file = this.fileInput?.files?.[0];
		if (!file) {
			this.transition(PresignedUploader.PHASES.ERROR, "Choose a file to upload.");
			return;
		}

		const csrf = this.form?.querySelector('input[name="csrf"]')?.value || "";
		const initiateUrl = this.form?.dataset?.initiate || (this.form?.action ? `${this.form.action}/initiate` : "");

		try {
			this.transition(PresignedUploader.PHASES.INITIATING);
			const initiate = await fetch(initiateUrl, {
				method: "POST",
				headers: { "Content-Type": "application/x-www-form-urlencoded" },
				body: new URLSearchParams({
					csrf,
					originalName: file.name,
					size: String(file.size),
					contentType: file.type || "application/octet-stream"
				}),
				credentials: "same-origin"
			});
			if (!initiate.ok) {
				const errMsg = await PresignedUploader.extractErrorMessage(initiate, "Failed to initiate file upload. Please try again.");
				throw new Error(errMsg);
			}
			const upload = await initiate.json();
			if (!upload || !upload.uploadUrl || !upload.completeUrl) {
				throw new Error("Invalid upload initiation response. Please try again.");
			}

			this.transition(PresignedUploader.PHASES.UPLOADING);
			const stored = await fetch(upload.uploadUrl, {
				method: "PUT",
				headers: upload.uploadHeaders || {},
				body: file,
				credentials: "omit"
			});
			if (!stored.ok) {
				throw new Error("Failed to upload file to storage. Please try again.");
			}

			this.transition(PresignedUploader.PHASES.COMPLETING);
			const complete = await fetch(upload.completeUrl, {
				method: "POST",
				headers: { "Content-Type": "application/x-www-form-urlencoded" },
				body: new URLSearchParams({ csrf }),
				credentials: "same-origin"
			});
			if (!complete.ok) {
				const errMsg = await PresignedUploader.extractErrorMessage(complete, "Failed to finalize file upload. Please try again.");
				throw new Error(errMsg);
			}

			this.transition(PresignedUploader.PHASES.SUCCESS);
			if (this.form?.action && typeof window !== "undefined" && window.location) {
				window.location.assign(this.form.action);
			}
		} catch (err) {
			const message = err?.message || "Upload failed. Please try again.";
			this.transition(PresignedUploader.PHASES.ERROR, message);
		}
	}

	static async extractErrorMessage(response, defaultMsg) {
		try {
			const text = await response.text();
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

PresignedUploader.Phases = PresignedUploader.PHASES;
PresignedUploader.PhaseMessages = PresignedUploader.PHASE_MESSAGES;

if (typeof window !== "undefined") {
	window.PresignedUploader = PresignedUploader;
}
if (typeof module !== "undefined" && module.exports) {
	module.exports = { PresignedUploader };
}

const fileUploadForm = document.querySelector("form[data-file-upload][data-presigned='true']");
if (fileUploadForm) {
	const uploader = new PresignedUploader(fileUploadForm);
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
		.find((form) => form.dataset.webmcpActions.split(" ").includes(action) && (!id || form.dataset.webmcpId === id));
	const control = (form, name) => [...form.elements].find((element) => element.name === name && element.type !== "hidden" && !element.disabled);
	const prepare = (tool, form, fields, button) => {
		if (!form) return result(tool, "resource-not-found");
		for (const [name, value] of Object.entries(fields || {})) {
			const element = control(form, name);
			if (!element) return result(tool, "visible-control-not-found");
			element.value = value;
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
