#!/bin/sh
set -eu
script=$(CDPATH= cd -- "$(dirname "$0")" && pwd)/verify-commit.sh
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
repo=$work/source
mkdir "$repo" "$work/bin"
git -C "$repo" init -q
git -C "$repo" config user.name Test
git -C "$repo" config user.email test@example.invalid
printf 'committed\n' > "$repo/content"
printf 'verify:\n\t@test "$$(cat content)" = committed\n' > "$repo/Makefile"
git -C "$repo" add .
git -C "$repo" commit -qm initial
commit=$(git -C "$repo" rev-parse HEAD)
printf 'dirty\n' > "$repo/content"
printf 'untracked\n' > "$repo/untracked"
printf '#!/bin/sh\nexit 0\n' > "$work/bin/npm"
chmod +x "$work/bin/npm"
export PATH="$work/bin:$PATH"
(cd "$repo" && sh "$script" "$commit") > "$work/output" 2>&1
grep -q "Verified commit $commit" "$work/output"
test "$(cat "$repo/content")" = dirty
test "$(cat "$repo/untracked")" = untracked
if (cd "$repo" && sh "$script" missing-commit) > "$work/output" 2>&1; then
 echo 'missing commit unexpectedly passed' >&2; exit 1
fi
grep -q 'commit not found' "$work/output"
printf 'verify:\n\t@false\n' > "$repo/Makefile"
git -C "$repo" add Makefile
git -C "$repo" commit -qm failure
if (cd "$repo" && sh "$script" HEAD) > "$work/output" 2>&1; then
 echo 'failed verification unexpectedly passed' >&2; exit 1
fi
if grep -q 'Verified commit' "$work/output"; then
 echo 'failed verification reported success' >&2; exit 1
fi
test "$(cat "$repo/content")" = dirty
test "$(cat "$repo/untracked")" = untracked
echo 'commit verification checks passed'
