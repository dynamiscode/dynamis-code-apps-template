---
name: form-field-input
description: Check field-level input help - inline validation with an error summary that takes focus, forgiving format (trimmed text, authenticator and recovery codes typed with spaces, dashes or lower case), input masks that agree with the server, the password strength meter and reveal toggle, matching confirmations, and smart defaults - with exhaustive tests, a Go test, and browser checks. Use when a change touches data-trim, data-mask, data-strength, data-match or data-default in internal/web/templates/, validation, summary, mask or password code in internal/web/assets/forms.js, normalizeTOTPCode, normalizeRecoveryCode or validTOTP in internal/identity/mfa.go, validateNewPassword, or DefaultTokenExpiry in internal/web/handler.go.
---

# Form field input

## The rules

1. **The meter never praises a password the server rejects.** `passwordStrength` is 0 exactly when the length is below 12 (`validateNewPassword`), never above 4, never falls as a password grows, and a repeated single character scores at most 1. Harm: users trust a "Strong" label and then get rejected, or pick a weak password.
2. **Masks agree with the server.** For every input the server accepts as a 6-digit code after `normalizeTOTPCode` (drop spaces, dashes, tabs), `maskDigits` sends the same 6 digits. `maskUpper` equals `normalizeRecoveryCode` (upper case, no whitespace; dashes are kept because the code alphabet contains `-` and `_`). Harm: a pasted "123 456" is cut to "123 45" or burns one of five MFA attempts.
3. **Invalid input is stopped in the browser with a summary.** On submit, invalid visible fields get `aria-invalid`, a message under the field, and an error summary (`role="alert"`) that takes focus; nothing is sent. A full-page server error takes focus on load.
4. **Reveal toggle and confirmation.** The toggle flips the input type and `aria-pressed`, and every password goes back to `type=password` before submit. A `data-match` field is invalid until it equals its partner.
5. **Smart defaults.** API token expiry defaults to 90 days (`DefaultTokenExpiry`); an empty account timezone is filled from the browser.

## Why tests, not a model

These are functions of one input, not orderings of events, so the check runs the real code on every input in a bound. `forms.test.mjs` enumerates every string up to 8 characters over `1 2 space - a` (over 400,000) for the TOTP mask, every string up to 6 characters over `a B - _ space tab` for the recovery mask, and every length 0–40 for all 15 character-class mixes for the meter. The Go test pins the server half (`normalizeTOTPCode`, `normalizeRecoveryCode`, and `validTOTP` accepting spaced and dashed codes). The browser checks cover the DOM wiring.

## Coverage record

- Covered code: `internal/web/assets/forms.js` (`passwordStrength`, `maskDigits`, `maskUpper`, `validateField`, `validateFields`, `showSummary`, `checkMatch`, `enhancePassword`, the input/focusout/submit handlers, the `data-default` timezone), `internal/identity/mfa.go` (`normalizeTOTPCode`, `normalizeRecoveryCode`, `validTOTP`, `CompleteRecoveryLogin`), `internal/web/handler.go` (`DefaultTokenExpiry`, login email echo), and the templates using these attributes (setup, login, account, password-reset, invitation, mfa, security, tokens, webhooks, invitations, item-list).
- Checks: `internal/web/assets/forms.test.mjs`, `internal/identity/mfa_normalize_test.go` (`TestMFACodeForgivingFormat`), `scripts/ux-patterns.mjs` (lines tagged `form-field-input`).
- Sync commit: the commit that last changed this file (`git log -1 --format=%H -- .agents/skills/form-field-input/SKILL.md`). Diff the covered files against it before trusting a pass, then update this file.

## Run the check

1. `node --test internal/web/assets/forms.test.mjs` → `fail 0`.
2. `go test ./internal/identity/ -run TestMFACodeForgivingFormat -count=1` → `ok`.
3. `make ux-smoke` → `N UX checks passed`.

## Read a failure

- `forms.test.mjs` prints the exact input, for example `"1 2-3a456" -> 12345`: the mask and the server disagree on it. Decide which side is wrong from the rule; the server normalizer is the contract.
- If the server's password minimum changes, change `MIN_PASSWORD` in forms.js with it; the meter test then follows.
- `TestMFACodeForgivingFormat` names the raw code that the server no longer reads the same.

## Proof the check catches a break

Measured on scratch changes, then reverted: the meter scoring from 8 characters, the mask keeping letters, and the server no longer dropping dashes each failed their test.
