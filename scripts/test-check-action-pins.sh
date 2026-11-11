#!/bin/sh
# Offline fixture tests for check-action-pins.sh. A fake git on PATH serves
# ls-remote output from per-case fixture files, so no network access is used.
set -eu

script="$(cd "$(dirname "$0")" && pwd)/check-action-pins.sh"
root="$(mktemp -d)"
trap 'rm -rf "$root"' EXIT
export REAL_GIT="$(command -v git)"

mkdir -p "$root/bin"
cat >"$root/bin/git" <<'EOF'
#!/bin/sh
if [ "$1" != ls-remote ]; then
	exec "$REAL_GIT" "$@"
fi
for url; do :; done
repo="${url#https://github.com/}"
repo="${repo%.git}"
refs="$FIXTURE_DIR/refs-$(printf '%s' "$repo" | tr '/' '-')"
if [ ! -f "$refs" ]; then
	echo "fake git: no refs for $repo" >&2
	exit 128
fi
cat "$refs"
EOF
chmod +x "$root/bin/git"

BRANCH_SHA=1111111111111111111111111111111111111111
LIGHT_TAG_SHA=2222222222222222222222222222222222222222
ANNOTATED_TAG_SHA=3333333333333333333333333333333333333333
PEELED_COMMIT_SHA=4444444444444444444444444444444444444444
UNKNOWN_SHA=9999999999999999999999999999999999999999

failures=0

# run_case <name> <expect: pass|fail> <uses value> <refs repo> <refs body>
run_case() {
	name="$1"
	expect="$2"
	uses="$3"
	refs_repo="$4"
	refs_body="$5"

	dir="$(mktemp -d "$root/case.XXXXXX")"
	mkdir -p "$dir/.github/workflows"
	printf 'jobs:\n  test:\n    steps:\n      - uses: %s\n' "$uses" \
		>"$dir/.github/workflows/ci.yml"
	printf '%s\n' "$refs_body" \
		>"$dir/refs-$(printf '%s' "$refs_repo" | tr '/' '-')"

	status=0
	(cd "$dir" && PATH="$root/bin:$PATH" FIXTURE_DIR="$dir" sh "$script" \
		>"$dir/output" 2>&1) || status=$?

	if [ "$expect" = pass ] && [ "$status" -eq 0 ]; then
		echo "ok - $name"
	elif [ "$expect" = fail ] && [ "$status" -ne 0 ]; then
		echo "ok - $name"
	else
		echo "not ok - $name (exit $status, expected $expect)"
		sed 's/^/    /' "$dir/output"
		failures=$((failures + 1))
	fi
}

run_case "branch tip SHA is accepted" pass \
	"actions/fake@$BRANCH_SHA # v1.0.0" actions/fake \
	"$(printf '%s\trefs/heads/main' "$BRANCH_SHA")"

run_case "lightweight tag SHA is accepted" pass \
	"actions/fake@$LIGHT_TAG_SHA # v1.0.0" actions/fake \
	"$(printf '%s\trefs/tags/v1.0.0' "$LIGHT_TAG_SHA")"

run_case "annotated tag object SHA is rejected" fail \
	"actions/fake@$ANNOTATED_TAG_SHA # v2.0.0" actions/fake \
	"$(printf '%s\trefs/tags/v2.0.0\n%s\trefs/tags/v2.0.0^{}' \
		"$ANNOTATED_TAG_SHA" "$PEELED_COMMIT_SHA")"

run_case "SHA absent from refs is rejected" fail \
	"actions/fake@$UNKNOWN_SHA # v1.0.0" actions/fake \
	"$(printf '%s\trefs/heads/main' "$BRANCH_SHA")"

run_case "subpath action resolves to owner/repo" pass \
	"github/codeql-action/analyze@$BRANCH_SHA # v3.39.3" github/codeql-action \
	"$(printf '%s\trefs/heads/main' "$BRANCH_SHA")"

run_case "unreachable repo is rejected" fail \
	"actions/missing@$BRANCH_SHA # v1.0.0" actions/other \
	"$(printf '%s\trefs/heads/main' "$BRANCH_SHA")"

run_case "unpinned tag reference is rejected" fail \
	"actions/fake@v4" actions/fake \
	"$(printf '%s\trefs/heads/main' "$BRANCH_SHA")"

if [ "$failures" -ne 0 ]; then
	echo "check-action-pins tests: $failures failed" >&2
	exit 1
fi
echo "check-action-pins tests: all passed"
