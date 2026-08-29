document.addEventListener("htmx:afterSwap",()=>document.querySelector('[role="alert"]')?.focus());
const list=document.querySelector("#item-list[data-events]");
const realtimeStatus=document.querySelector("#realtime-status");
if(list&&window.EventSource){const messages={connecting:list.dataset.liveConnecting,connected:list.dataset.liveConnected,unavailable:list.dataset.liveUnavailable,reconnecting:list.dataset.liveReconnecting};if(realtimeStatus)realtimeStatus.textContent=messages.connecting;const events=new EventSource(list.dataset.events);events.addEventListener("open",()=>{if(realtimeStatus)realtimeStatus.textContent=messages.connected});events.addEventListener("error",()=>{if(realtimeStatus)realtimeStatus.textContent=events.readyState===EventSource.CLOSED?messages.unavailable:messages.reconnecting});for(const name of["item.changed","resync"]){events.addEventListener(name,()=>window.htmx?.ajax("GET",location.pathname,{target:"#item-list",swap:"outerHTML"}))}events.addEventListener("close",()=>events.close())}
document.addEventListener("click",event=>{const button=event.target instanceof Element?event.target.closest("button[data-confirm]"):null;if(button&&!window.confirm(button.dataset.confirm))event.preventDefault()});
const fileUploadForm=document.querySelector("form[data-file-upload][data-presigned='true']");
if(fileUploadForm){fileUploadForm.addEventListener("submit",async event=>{event.preventDefault();const input=fileUploadForm.querySelector("input[type=file]");const file=input?.files?.[0];if(!file)return;const csrf=fileUploadForm.querySelector("input[name=csrf]")?.value||"";try{const initiate=await fetch(fileUploadForm.dataset.initiate,{method:"POST",headers:{"Content-Type":"application/x-www-form-urlencoded"},body:new URLSearchParams({csrf,originalName:file.name,size:String(file.size),contentType:file.type}),credentials:"same-origin"});if(!initiate.ok)throw new Error("initiation failed");const upload=await initiate.json();const stored=await fetch(upload.uploadUrl,{method:"PUT",headers:upload.uploadHeaders||{},body:file,credentials:"omit"});if(!stored.ok)throw new Error("upload failed");const complete=await fetch(upload.completeUrl,{method:"POST",headers:{"Content-Type":"application/x-www-form-urlencoded"},body:new URLSearchParams({csrf}),credentials:"same-origin"});if(!complete.ok)throw new Error("completion failed");window.location.assign(fileUploadForm.action)}catch(_){window.location.reload()}})}

(() => {
	const context = document.modelContext;
	if (!context || typeof context.registerTool !== "function" || !document.querySelector("[data-webmcp-page]")) return;
	const idSchema = {type: "string", pattern: "^[0-9a-f]{32}$"};
	const textSchema = (maxLength) => ({type: "string", minLength: 1, maxLength});
	const schemas = {
		"workspace-create-v1": {type: "object", properties: {name: textSchema(120)}, required: ["name"], additionalProperties: false},
		"item-create-v1": {type: "object", properties: {title: textSchema(200)}, required: ["title"], additionalProperties: false},
		"item-update-v1": {type: "object", properties: {itemId: idSchema, title: textSchema(200), status: {type: "string", enum: ["active", "complete"]}}, required: ["itemId", "title", "status"], additionalProperties: false},
		"item-delete-v1": {type: "object", properties: {itemId: idSchema}, required: ["itemId"], additionalProperties: false},
		"member-role-update-v1": {type: "object", properties: {userId: idSchema, role: {type: "string", enum: ["admin", "member", "viewer"]}}, required: ["userId", "role"], additionalProperties: false},
		"member-remove-v1": {type: "object", properties: {userId: idSchema}, required: ["userId"], additionalProperties: false},
		"ownership-transfer-v1": {type: "object", properties: {userId: idSchema}, required: ["userId"], additionalProperties: false},
		"invitation-revoke-v1": {type: "object", properties: {invitationId: idSchema}, required: ["invitationId"], additionalProperties: false},
		"token-revoke-v1": {type: "object", properties: {tokenId: idSchema}, required: ["tokenId"], additionalProperties: false},
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
		if (name === "item-update-v1") return validId(input.itemId) && text(input.title, 200) && ["active", "complete"].includes(input.status)
			? prepare(name, formFor(actions[name], input.itemId), {title: input.title, status: input.status}, "update") : result(name, "invalid-input");
		if (name === "item-delete-v1") return validId(input.itemId) ? prepare(name, formFor(actions[name], input.itemId), {}, "delete") : result(name, "invalid-input");
		const idField = {"member-role-update-v1": "userId", "member-remove-v1": "userId", "ownership-transfer-v1": "userId", "invitation-revoke-v1": "invitationId", "token-revoke-v1": "tokenId", "session-revoke-v1": "sessionId"}[name];
		if (idField) {
			if (!validId(input[idField])) return result(name, "invalid-input");
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
