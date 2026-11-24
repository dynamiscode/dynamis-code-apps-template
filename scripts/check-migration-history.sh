#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
	echo "usage: $0 BASE HEAD" >&2
	exit 2
fi

base=$(git merge-base "$1" "$2")
head=$2
path=internal/platform/database/migrations

# Include each retained commit so a rename made during development
# cannot disappear from the final tree comparison.
for commit in $(git rev-list "$base..$head"); do
	deleted=$(git diff-tree --no-commit-id --diff-filter=D --name-only -r "$commit^" "$commit" -- "$path")
	if [ -n "$deleted" ]; then
		printf 'Migration removed or renamed in commit %s:\n%s\n' "$commit" "$deleted" >&2
		exit 1
	fi
done

changed=$(git diff --name-only --diff-filter=M "$base" "$head" -- "$path")
if [ -n "$changed" ]; then
	printf 'Existing migrations changed:\n%s\n' "$changed" >&2
	exit 1
fi
