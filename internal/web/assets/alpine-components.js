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

	if (typeof module !== "undefined" && module.exports) module.exports = { notificationsLive, passkeyLogin, passkeyEnroll };
	if (typeof document === "undefined" || typeof window === "undefined") return;
	document.addEventListener("alpine:init", () => {
		window.Alpine.data("notificationsLive", notificationsLive);
		window.Alpine.data("passkeyLogin", passkeyLogin);
		window.Alpine.data("passkeyEnroll", passkeyEnroll);
	});
})();
