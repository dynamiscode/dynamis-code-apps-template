#!/usr/bin/env bash
# Smoke test for scripts/afk.sh: runs it with no flags against stub gh, herdr-app and hc
# executables and asserts it is a dry run that invokes no write verb.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

STUB_BIN="$TMP/bin"
STUB_LOG="$TMP/invocations.log"
mkdir -p "$STUB_BIN"
: > "$STUB_LOG"

# Each stub records its argv and prints fake output for the read calls the script makes.
for name in gh herdr-app hc; do
  cat > "$STUB_BIN/$name" <<EOF
#!/usr/bin/env bash
echo "$name \$*" >> "$STUB_LOG"
case "$name \$1 \$2" in
  "gh repo view") echo "example/repo" ;;
  "gh api "*) echo 0 ;;
  "gh issue list") printf '101\tExample ready issue\n' ;;
  "herdr-app status") echo ok ;;
esac
EOF
  chmod +x "$STUB_BIN/$name"
done

export AFK_LOG_DIR="$TMP/logs" AFK_WORKTREE_ROOT="$TMP/worktrees"
out="$TMP/stdout.txt"
status=0
PATH="$STUB_BIN:$PATH" "$ROOT/scripts/afk.sh" > "$out" 2>&1 || status=$?

fail() { echo "afk_test: FAIL: $*" >&2; cat "$out" >&2; exit 1; }

[ "$status" -eq 0 ] || fail "expected exit 0, got $status"

# Stubs must actually have been reached, or the write check below proves nothing.
grep -q '^gh repo view' "$STUB_LOG" || fail "gh stub was not invoked"

if grep -E ' (create|edit|label|do|run|start)( |$)|worktree add' "$STUB_LOG"; then
  fail "write verb invoked in default mode"
fi

grep -q 'mode=dry-run' "$AFK_LOG_DIR"/run-*.log || fail "log does not say dry-run"
grep -q 'PLAN #101' "$AFK_LOG_DIR"/run-*.log || fail "dry run did not plan issue #101"
[ -z "$(ls -A "$AFK_WORKTREE_ROOT")" ] || fail "worktree directory was populated"

echo "afk_test: PASS"
