#!/bin/sh
set -eu
script=$(CDPATH= cd -- "$(dirname "$0")" && pwd)/profile-preflight.sh
work=$(mktemp -d)
stage=setup
trap 'status=$?; if [ "$status" -ne 0 ]; then echo "profile preflight self-check failed: $stage" >&2; if [ -f "$work/output" ]; then cat "$work/output" >&2; fi; fi; rm -rf "$work"' EXIT
repo=$work/repo
mkdir -p "$repo/internal/web" "$repo/docs" "$work/bin"
git -C "$repo" init -q
git -C "$repo" config user.name Test
git -C "$repo" config user.email test@example.invalid
printf 'base\n' > "$repo/internal/web/handler.go"
printf 'base\n' > "$repo/docs/guide.md"
git -C "$repo" add .
git -C "$repo" commit -qm base
base=$(git -C "$repo" rev-parse HEAD)
cat > "$work/bin/go" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >> "$GO_CALLS"
exit "${GO_EXIT:-0}"
EOF
chmod +x "$work/bin/go"
export GO_CALLS="$work/calls" PATH="$work/bin:$PATH"

run() { (cd "$repo" && sh "$script" "$@"); }
contains() {
	if ! grep -q -- "$1" "$2"; then
		echo "Expected $2 to contain: $1" >&2
		return 1
	fi
}
stage='unrelated changes skip the matrix'
printf 'unrelated\n' >> "$repo/docs/guide.md"
run "$base" > "$work/output"
contains 'skipped' "$work/output"
if [ -e "$GO_CALLS" ]; then
	echo 'unrelated changes unexpectedly ran Go tests' >&2; exit 1
fi

stage='unstaged code change runs the matrix'
printf 'changed\n' >> "$repo/internal/web/handler.go"
run "$base" > "$work/output"
contains 'testing generated profile matrix' "$work/output"
contains 'TestGenerateCatalogProfile' "$GO_CALLS"
contains 'TestGenerateWithClientsAndFilesProfilesBuilds' "$GO_CALLS"
contains '-count=1' "$GO_CALLS"
stage='preflight reruns the matrix'
run "$base" > "$work/output"
if [ "$(wc -l < "$GO_CALLS" | tr -d ' ')" != 2 ]; then
	echo 'expected two Go test invocations' >&2; exit 1
fi
stage='matrix failure propagates'
if (GO_EXIT=1; export GO_EXIT; run "$base") > "$work/output" 2>&1; then
	echo 'failed profile test unexpectedly passed' >&2; exit 1
fi
stage='staged code change runs the matrix'
git -C "$repo" add internal/web/handler.go
run "$base" > "$work/output"
contains 'testing generated profile matrix' "$work/output"
git -C "$repo" commit -qm change
stage='committed code change runs the matrix'
run "$base" > "$work/output"
contains 'testing generated profile matrix' "$work/output"
stage='deleted code runs the matrix'
rm "$repo/internal/web/handler.go"
run HEAD > "$work/output"
contains 'testing generated profile matrix' "$work/output"
printf 'base\nchanged\n' > "$repo/internal/web/handler.go"
stage='untracked code runs the matrix'
printf 'new\n' > "$repo/internal/web/new.go"
run HEAD > "$work/output"
contains 'testing generated profile matrix' "$work/output"
stage='missing base is rejected'
if run missing-base > "$work/output" 2>&1; then
	echo 'missing base unexpectedly passed' >&2; exit 1
fi
contains 'base commit not found' "$work/output"
echo 'profile preflight checks passed'
