import assert from "node:assert/strict";
import test from "node:test";
import { checkReview } from "./check-pr-ux-review.mjs";

const head = "abc1234567890123456789012345678901234567";
const resolve = (revision) => {
  if (head.startsWith(revision)) return head;
  throw new Error("unknown or ambiguous");
};
const check = (body, sha = head) => checkReview(body, sha, resolve);

const reviewed = `## Browser UX review
- Status: reviewed
- Revision: abc1234
- Journey: Record a transaction and return to history
- Role: owner
- Viewport: desktop and narrow
- Evidence: https://github.com/example/repo/pull/1#issuecomment-1
- Unresolved gaps: none
- Actions and outcomes: Recorded a transaction and saw it in history.
## Safety and scope
`;

test("accepts a complete review or an explained non-visual exemption", () => {
  assert.equal(check(reviewed), null);
  assert.equal(check("## Browser UX review\n- Status: exempt\n- Reason: Refactors a server helper without changing rendered behavior.\n"), null);
});

test("rejects incomplete, blocked, and ambiguous records", () => {
  assert.match(check("## Browser UX review\n<!-- - Status: reviewed -->\n"), /status/i);
  assert.match(check(reviewed.replace("- Role: owner", "- Role: ")), /role/i);
  assert.match(check(reviewed.replace("https://github.com/example/repo/pull/1#issuecomment-1", "screenshot attached")), /HTTPS link/);
  assert.match(check(reviewed.replace("https://github.com/example/repo/pull/1#issuecomment-1", "https://[")), /HTTPS link/);
  assert.match(check(reviewed.replace("- Status: reviewed", "- Status: reviewed\n- Status: exempt")), /duplicate/i);
  assert.match(check(reviewed.replace("Status: reviewed", "Status: blocked")), /blocked/);
  assert.match(check(reviewed.replace("- Unresolved gaps: none", "")), /unresolved gaps/i);
  assert.match(check("## Browser UX review\n- Status: exempt\n- Reason: TBD\n"), /reason/i);
  assert.match(check(reviewed, "def1234567890123456789012345678901234567"), /current PR head/);
  assert.match(check(reviewed.replace("abc1234", "abcdef0")), /unknown or ambiguous/);
  assert.equal(check(reviewed.replace("abc1234", head)), null);
});
