(() => {
  const button = document.getElementById("passkey-login");
  const optionsNode = document.getElementById("mfa-options");
  if (!window.PublicKeyCredential || !navigator.credentials) return;
  const decode = value => Uint8Array.from(atob(value.replace(/-/g, "+").replace(/_/g, "/") + "==="), c => c.charCodeAt(0));
  const encode = value => { let binary = ""; new Uint8Array(value).forEach(byte => binary += String.fromCharCode(byte)); return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, ""); };
  if (button && optionsNode) button.addEventListener("click", async () => {
    try {
      const options = JSON.parse(optionsNode.textContent);
      options.publicKey.challenge = decode(options.publicKey.challenge);
      (options.publicKey.allowCredentials || []).forEach(item => item.id = decode(item.id));
      const credential = await navigator.credentials.get({publicKey: options.publicKey});
      const response = {id: credential.id, rawId: encode(credential.rawId), type: credential.type, response: {clientDataJSON: encode(credential.response.clientDataJSON), authenticatorData: encode(credential.response.authenticatorData), signature: encode(credential.response.signature), userHandle: credential.response.userHandle ? encode(credential.response.userHandle) : null}};
      const result = await fetch("/mfa/passkey", {method: "POST", headers: {"Content-Type": "application/json", "X-MFA-CSRF": button.dataset.csrf}, body: JSON.stringify(response)});
      if (result.ok) window.location = "/"; else window.location.reload();
    } catch (_) {}
  });

  const enrollment = document.getElementById("passkey-enroll");
  if (!enrollment) return;
  enrollment.addEventListener("submit", async event => {
    event.preventDefault();
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
      window.location.reload();
    } catch (_) { window.location.reload(); }
  });
})();
