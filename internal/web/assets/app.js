if(window.htmx)htmx.config.noSwap=[204,304,"4xx","5xx"];
const list=document.querySelector("#item-list[data-events],#client-list[data-events]");
if(list&&window.EventSource){
	const listID=list.id;
	const messages={connecting:list.dataset.liveConnecting,connected:list.dataset.liveConnected,unavailable:list.dataset.liveUnavailable,reconnecting:list.dataset.liveReconnecting};
	let connection=messages.connecting,waiting=false,refreshing=false,pending=false,activeContext=null,cancelledContext=null;
	const current=()=>document.querySelector("#"+listID);
	const status=()=>document.querySelector("#realtime-status");
	const showStatus=()=>{const node=status();if(node)node.textContent=waiting?(current()?.dataset.liveWaiting||connection):connection};
	const events=new EventSource(list.dataset.events);
	events.addEventListener("open",()=>{connection=messages.connected;showStatus()});
	events.addEventListener("error",()=>{connection=events.readyState===EventSource.CLOSED?messages.unavailable:messages.reconnecting;showStatus()});
	const refresh=()=>{
		const node=current();
		if(!node)return;
		if(window.dynamisForms?.refreshDecision(window.dynamisForms.isEditing(node))==="defer"){waiting=true;showStatus();return}
		waiting=false;showStatus();
		if(refreshing){pending=true;return}
		refreshing=true;
		window.htmx?.ajax("GET",location.pathname+location.search,{target:"#"+listID,swap:"outerHTML"});
	};
	const retryWaiting=()=>setTimeout(()=>{if(waiting)refresh()},0);
	const finish=(ctx)=>{
		if(ctx!==activeContext)return;
		activeContext=null;cancelledContext=null;refreshing=false;
		if(pending){pending=false;refresh()}
	};
	document.addEventListener("htmx:before:request",event=>{
		const ctx=event.detail?.ctx;
		if(ctx?.target?.id===listID&&ctx.request?.method?.toLowerCase()==="get"){activeContext=ctx;refreshing=true}
	});
	document.addEventListener("htmx:before:swap",event=>{
		const ctx=event.detail?.ctx;
		if(ctx?.target?.id!==listID||ctx.request?.method?.toLowerCase()!=="get")return;
		const node=current();
		if(node&&window.dynamisForms?.refreshDecision(window.dynamisForms.isEditing(node))==="defer"){event.preventDefault();waiting=true;cancelledContext=ctx;showStatus()}
	});
	document.addEventListener("htmx:after:swap",event=>{
		const ctx=event.detail?.ctx;
		if(ctx?.target?.id===listID){showStatus();finish(ctx)}
	});
	document.addEventListener("htmx:after:request",event=>{
		const ctx=event.detail?.ctx;
		const status=ctx?.response?.status;
		if(ctx===cancelledContext||event.detail?.failed||status>=400||status===204||status===304)finish(ctx);
	});
	for(const name of["htmx:sendError","htmx:timeout"]){document.addEventListener(name,event=>finish(event.detail?.ctx))}
	document.addEventListener("htmx:finally:request",event=>{
		const ctx=event.detail?.ctx;
		if(ctx===cancelledContext||String(ctx?.status||"").startsWith("error:"))finish(ctx);
	});
	document.addEventListener("click",event=>{
		const summary=event.target instanceof Element?event.target.closest("summary"):null;
		if(summary?.parentElement?.matches("details"))summary.parentElement.dataset.userToggled="true";
	});
	document.addEventListener("toggle",event=>{
		if(event.target instanceof HTMLDetailsElement&&event.target.dataset.userToggled)retryWaiting();
	},true);
	for(const name of[listID==="client-list"?"client.changed":"item.changed","resync"]){events.addEventListener(name,refresh)}
	for(const name of["focusout","input","change","htmx:finally:request"]){document.addEventListener(name,retryWaiting)}
	events.addEventListener("close",()=>events.close());
}
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
