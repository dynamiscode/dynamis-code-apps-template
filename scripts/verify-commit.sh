#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
 echo 'usage: sh scripts/verify-commit.sh <commit>' >&2
 exit 2
fi
commit=$(git rev-parse --verify --quiet "$1^{commit}") || {
 echo "commit not found: $1" >&2
 exit 1
}
repo=$(git rev-parse --show-toplevel)
work=$(mktemp -d "${TMPDIR:-/tmp}/verify-commit.XXXXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM
printf 'Verifying commit %s\n' "$commit"
git clone --quiet --no-local --no-checkout "$repo" "$work/repo"
git -C "$work/repo" checkout --quiet --detach "$commit"
cd "$work/repo"
npm ci
make verify
printf 'Verified commit %s\n' "$commit"
