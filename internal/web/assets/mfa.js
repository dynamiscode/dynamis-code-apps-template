(() => {
  const button = document.getElementById("passkey-login");
  const optionsNode = document.getElementById("mfa-options");
  if (!window.PublicKeyCredential || !navigator.credentials) return;
  const decode = value => Uint8Array.from(atob(value.replace(/-/g, "+").replace(/_/g, "/") + "==="), c => c.charCodeAt(0));
  const encode = value => { let binary = ""; new Uint8Array(value).forEach(byte => binary += String.fromCharCode(byte)); return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, ""); };
  const isCancellation = err => {
    if (!err) return false;
    const name = err.name || "";
    const msg = (err.message || "").toLowerCase();
    return name === "NotAllowedError" || name === "AbortError" || name === "TimeoutError" || msg.includes("abort") || msg.includes("cancel") || msg.includes("timeout") || msg.includes("notallowed");
  };
  const showNotice = (message, container) => {
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
  // Thrown for a non-OK server response; carries only the server's safe text, never raw internals.
  class RejectedResponse extends Error {}
  const rejectIfNotOk = async response => {
    if (response.ok) return;
    let detail = "";
    try {
      const body = await response.json();
      if (typeof body.error === "string") detail = body.error;
      else if (typeof body.detail === "string") detail = body.detail;
    } catch (err) {
      detail = "";
    }
    throw new RejectedResponse(detail.trim() || "The passkey registration failed.");
  };
  // Disables buttons while fn runs and leaves them disabled on success on purpose (navigation
  // or recovery codes follow). A cancel re-enables them with a notice; a server rejection shows
  // its message; any other failure reloads the page.
  const withBusy = async (buttons, fn, {cancelMessage, container}) => {
    const setButtonsDisabled = disabled => buttons.forEach(b => { b.disabled = disabled; });
    setButtonsDisabled(true);
    try {
      await fn();
    } catch (err) {
      if (err instanceof RejectedResponse) {
        setButtonsDisabled(false);
        showNotice(err.message, container);
      } else if (isCancellation(err)) {
        setButtonsDisabled(false);
        showNotice(cancelMessage, container);
      } else {
        window.location.reload();
      }
    }
  };

  if (button && optionsNode) button.addEventListener("click", () => withBusy(document.querySelectorAll("button, input[type=submit]"), async () => {
    const options = JSON.parse(optionsNode.textContent);
    options.publicKey.challenge = decode(options.publicKey.challenge);
    (options.publicKey.allowCredentials || []).forEach(item => item.id = decode(item.id));
    const credential = await navigator.credentials.get({publicKey: options.publicKey});
    const response = {id: credential.id, rawId: encode(credential.rawId), type: credential.type, response: {clientDataJSON: encode(credential.response.clientDataJSON), authenticatorData: encode(credential.response.authenticatorData), signature: encode(credential.response.signature), userHandle: credential.response.userHandle ? encode(credential.response.userHandle) : null}};
    const result = await fetch("/mfa/passkey", {method: "POST", headers: {"Content-Type": "application/json", "X-MFA-CSRF": button.dataset.csrf}, body: JSON.stringify(response)});
    if (result.ok) {
      window.location = result.headers.get("X-MFA-Return-To") || "/";
    } else {
      window.location.reload();
    }
  }, {cancelMessage: "Passkey verification was canceled or timed out."}));

  const enrollment = document.getElementById("passkey-enroll");
  if (!enrollment) return;
  enrollment.addEventListener("submit", event => {
    event.preventDefault();
    return withBusy(enrollment.querySelectorAll("button, input[type=submit]"), async () => {
      const csrf = enrollment.querySelector("[name=csrf]").value;
      const response = await fetch(enrollment.action, {method: "POST", headers: {"Content-Type": "application/json", "X-CSRF-Token": csrf}, body: JSON.stringify({password: enrollment.querySelector("[name=password]").value})});
      await rejectIfNotOk(response);
      const data = await response.json();
      const options = data.options;
      options.publicKey.challenge = decode(options.publicKey.challenge);
      options.publicKey.user.id = decode(options.publicKey.user.id);
      (options.publicKey.excludeCredentials || []).forEach(item => item.id = decode(item.id));
      const credential = await navigator.credentials.create({publicKey: options.publicKey});
      const result = {id: credential.id, rawId: encode(credential.rawId), type: credential.type, response: {clientDataJSON: encode(credential.response.clientDataJSON), attestationObject: encode(credential.response.attestationObject), transports: credential.response.getTransports ? credential.response.getTransports() : []}};
      const completed = await fetch("/api/v1/auth/mfa/passkeys/register", {method: "POST", headers: {"Content-Type": "application/json", "X-CSRF-Token": csrf, "X-MFA-Challenge": data.challenge, "X-MFA-Name": "Passkey"}, body: JSON.stringify(result)});
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
    }, {cancelMessage: "Passkey registration was canceled or timed out.", container: enrollment.parentElement});
  });
})();
