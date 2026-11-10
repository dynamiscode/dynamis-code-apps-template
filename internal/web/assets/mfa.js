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

  if (button && optionsNode) button.addEventListener("click", async () => {
    const buttons = document.querySelectorAll("button, input[type=submit]");
    const setButtonsDisabled = disabled => buttons.forEach(b => { b.disabled = disabled; });
    setButtonsDisabled(true);
    try {
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
    } catch (err) {
      if (isCancellation(err)) {
        setButtonsDisabled(false);
        showNotice("Passkey verification was canceled or timed out.");
        return;
      }
      window.location.reload();
    }
  });

  const enrollment = document.getElementById("passkey-enroll");
  if (!enrollment) return;
  enrollment.addEventListener("submit", async event => {
    event.preventDefault();
    const buttons = enrollment.querySelectorAll("button, input[type=submit]");
    const setButtonsDisabled = disabled => buttons.forEach(b => { b.disabled = disabled; });
    setButtonsDisabled(true);
    try {
      const csrf = enrollment.querySelector("[name=csrf]").value;
      const response = await fetch(enrollment.action, {method: "POST", headers: {"Content-Type": "application/json", "X-CSRF-Token": csrf}, body: JSON.stringify({password: enrollment.querySelector("[name=password]").value})});
      const data = await response.json();
      if (!response.ok) throw new Error();
      const options = data.options;
      options.publicKey.challenge = decode(options.publicKey.challenge);
      options.publicKey.user.id = decode(options.publicKey.user.id);
      (options.publicKey.excludeCredentials || []).forEach(item => item.id = decode(item.id));
      const credential = await navigator.credentials.create({publicKey: options.publicKey});
      const result = {id: credential.id, rawId: encode(credential.rawId), type: credential.type, response: {clientDataJSON: encode(credential.response.clientDataJSON), attestationObject: encode(credential.response.attestationObject), transports: credential.response.getTransports ? credential.response.getTransports() : []}};
      const completed = await fetch("/api/v1/auth/mfa/passkeys/register", {method: "POST", headers: {"Content-Type": "application/json", "X-CSRF-Token": csrf, "X-MFA-Challenge": data.challenge, "X-MFA-Name": "Passkey"}, body: JSON.stringify(result)});
      if (!completed.ok) throw new Error();
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
    } catch (err) {
      if (isCancellation(err)) {
        setButtonsDisabled(false);
        showNotice("Passkey registration was canceled or timed out.", enrollment.parentElement);
        return;
      }
      window.location.reload();
    }
  });
})();
