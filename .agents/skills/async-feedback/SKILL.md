---
name: async-feedback
description: Check busy forms, asynchronous feedback, lazy audit loading and keyboard focus after swaps. Use when changing wireForms feedback handlers or auditHistoryPage.
---

# Async feedback

Forms must retain ordinary server submission. During submission, busy controls prevent duplicate presses and recover after completion or back/forward navigation. Errors remain visible and focusable. The audit page provides a lazy table and a no-JavaScript fallback. Keyboard users can reach the skip link, dismiss menus with Escape, and regain focus after swaps.

## Checks

1. Run `node --test internal/web/assets/forms.test.mjs`.
2. Run `go test ./internal/web -run TestWebAuditHistoryIsScopedRedactedAndNotWebMCP -count=1`.
3. Run `make browser-smoke` and `make accessibility-smoke` for actual browser behavior.

The historical ItemDelete model describes the retired Items feature; it is not evidence of current application behavior.
