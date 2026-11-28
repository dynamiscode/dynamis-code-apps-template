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

stage='pre-push hook setup'
template=$(CDPATH= cd -- "$(dirname "$script")/.." && pwd)
push_repo=$work/push-repo
remote=$work/remote.git
mkdir -p "$push_repo/scripts" "$push_repo/internal/web" "$push_repo/docs"
cp "$template/Makefile" "$push_repo/Makefile"
cp "$script" "$push_repo/scripts/profile-preflight.sh"
printf 'base\n' > "$push_repo/internal/web/handler.go"
printf 'base\n' > "$push_repo/docs/guide.md"
git -C "$push_repo" init -q
git -C "$push_repo" config user.name Test
git -C "$push_repo" config user.email test@example.invalid
git -C "$push_repo" add .
git -C "$push_repo" -c core.hooksPath=/dev/null commit -qm base
push_base=$(git -C "$push_repo" rev-parse HEAD)
git -C "$push_repo" update-ref refs/remotes/origin/main "$push_base"
git init -q --bare "$remote"
unset BASE
export GO_CALLS="$work/push-calls"

stage='pre-push skips unrelated changes'
printf 'docs\n' >> "$push_repo/docs/guide.md"
git -C "$push_repo" add docs/guide.md
git -C "$push_repo" -c core.hooksPath=/dev/null commit -qm docs
git -C "$push_repo" -c core.hooksPath="$template/.githooks" push "$remote" HEAD:refs/heads/main > "$work/output" 2>&1
contains 'skipped' "$work/output"
if [ -e "$GO_CALLS" ]; then
	echo 'unrelated push unexpectedly ran Go tests' >&2; exit 1
fi

stage='pre-push does not expose remote or stdin data'
(cd "$push_repo" && printf 'secret-token\n' | sh "$template/.githooks/pre-push" origin 'https://secret-token@example.invalid/repo') > "$work/output" 2>&1
if grep -q 'secret-token' "$work/output"; then
	echo 'pre-push exposed hook data' >&2; exit 1
fi

stage='pre-push rejects matrix failure'
printf 'changed\n' >> "$push_repo/internal/web/handler.go"
git -C "$push_repo" add internal/web/handler.go
git -C "$push_repo" -c core.hooksPath=/dev/null commit -qm code
if (GO_EXIT=1; export GO_EXIT; git -C "$push_repo" -c core.hooksPath="$template/.githooks" push "$remote" HEAD:refs/heads/main) > "$work/output" 2>&1; then
	echo 'push unexpectedly passed a failed matrix' >&2; exit 1
fi
contains 'TestGenerateCatalogProfile' "$GO_CALLS"

stage='pre-push rejects missing base'
git -C "$push_repo" update-ref -d refs/remotes/origin/main
if git -C "$push_repo" -c core.hooksPath="$template/.githooks" push "$remote" HEAD:refs/heads/main > "$work/output" 2>&1; then
	echo 'push unexpectedly passed without a base' >&2; exit 1
fi
contains 'base commit not found' "$work/output"
git -C "$push_repo" update-ref refs/remotes/origin/main "$push_base"

stage='pre-push accepts passing matrix'
git -C "$push_repo" -c core.hooksPath="$template/.githooks" push "$remote" HEAD:refs/heads/main > "$work/output" 2>&1
contains 'testing generated profile matrix' "$work/output"
echo 'profile preflight checks passed'
