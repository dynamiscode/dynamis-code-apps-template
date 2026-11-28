//go:build browser

package browsertest

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chromedp/cdproto/webauthn"
	"github.com/chromedp/chromedp"
)

func TestPasskeyEnrollmentAndLogin(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "server")
	build := exec.Command("go", "build", "-o", binary, "./cmd/server")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build server: %v\n%s", err, out)
	}
	port, err := freePort()
	if err != nil {
		t.Fatal(err)
	}
	base := fmt.Sprintf("http://localhost:%d", port)
	server := exec.Command(binary)
	server.Env = []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "TMPDIR=" + dir,
		"DATABASE_DRIVER=sqlite", "SQLITE_PATH=" + filepath.Join(dir, "app.db"),
		"STORAGE_DRIVER=local", "STORAGE_LOCAL_PATH=" + filepath.Join(dir, "files"),
		"MFA_ENABLED=true", "MFA_ENCRYPTION_KEY=" + strings.Repeat("01", 32),
		"WEBAUTHN_RP_ID=localhost", "WEBAUTHN_RP_ORIGIN=" + base,
		fmt.Sprintf("HTTP_ADDRESS=127.0.0.1:%d", port), "HTTP_SECURE=false",
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Process.Kill(); _ = server.Wait() })
	if err := waitReady(base + "/health/ready"); err != nil {
		t.Fatal(err)
	}

	tab := newTab(t, 1280, 900, "")
	call(t, tab, webauthn.Enable, webauthn.EnableParams{})
	authenticator := call(t, tab, webauthn.AddVirtualAuthenticator, webauthn.AddVirtualAuthenticatorParams{Options: &webauthn.VirtualAuthenticatorOptions{
		Protocol: webauthn.AuthenticatorProtocolCtap2, Transport: webauthn.AuthenticatorTransportInternal,
		HasResidentKey: true, HasUserVerification: true, AutomaticPresenceSimulation: true, IsUserVerified: true,
	}})
	t.Cleanup(func() {
		call(t, tab, webauthn.RemoveVirtualAuthenticator, webauthn.RemoveVirtualAuthenticatorParams{AuthenticatorID: authenticator.AuthenticatorID})
	})

	const email = "passkey@example.com"
	const password = "passkey-password-1234!"
	run(t, tab, chromedp.Navigate(base+"/setup"))
	fill(t, tab, "#email", email)
	fill(t, tab, "#password", password)
	fill(t, tab, "#password-confirmation", password)
	click(t, tab, "form[data-wizard] .wizard-nav button:not([hidden])")
	fill(t, tab, "#workspace", "Passkey Test")
	click(t, tab, `form[data-wizard] button[type="submit"]`)
	eventually(t, tab, `!location.pathname.endsWith("/setup")`, "setup did not finish")
	run(t, tab, chromedp.Navigate(base+"/login"))
	fill(t, tab, "#email", email)
	fill(t, tab, "#password", password)
	click(t, tab, `form[action='/login'] button[type='submit']`)
	eventually(t, tab, `location.pathname !== "/login"`, "initial login did not finish")

	run(t, tab, chromedp.Navigate(base+"/security"))
	waitVisible(t, tab, "#passkey-enroll")
	fill(t, tab, "#passkey-password", password)
	ctx, cancel := context.WithTimeout(tab, actionWait)
	defer cancel()
	if err := chromedp.Do(ctx, chromedp.Click(`#passkey-enroll button[type="submit"]`), chromedp.WaitVisible("#mfa-recovery-output")); err != nil {
		t.Fatalf("passkey enrollment: %v", err)
	}
	if !eval[bool](t, tab, `document.querySelector("#passkey-enroll").hidden && document.querySelector("#mfa-recovery-values").textContent.trim().length > 0`) {
		t.Fatal("enrollment did not hide the form and show recovery codes")
	}
	if err := chromedp.Do(ctx, chromedp.Navigate(base+"/security")); err != nil {
		t.Fatalf("leave recovery page: %v", err)
	}
	run(t, tab, chromedp.Click(chromedp.CSS(`header details:has(form[action="/logout"]) > summary`)))
	run(t, tab, chromedp.Click(chromedp.CSS(`header form[action="/logout"] button`)))
	eventually(t, tab, `location.pathname === "/login"`, "sign-out did not reach login")
	fill(t, tab, "#email", email)
	fill(t, tab, "#password", password)
	click(t, tab, `form[action='/login'] button[type='submit']`)
	eventually(t, tab, `location.pathname === "/mfa"`, "login did not reach MFA")
	waitVisible(t, tab, "#passkey-login")
	click(t, tab, "#passkey-login")
	eventually(t, tab, `location.pathname !== "/mfa"`, "passkey login did not finish")
	run(t, tab, chromedp.Navigate(base+"/security"))
	waitVisible(t, tab, "[data-account-page]")
}
