#!/bin/sh
set -eu

script=$(CDPATH= cd -- "$(dirname "$0")" && pwd)/check-migration-history.sh
repo=$(mktemp -d)
trap 'rm -rf "$repo"' EXIT
git -C "$repo" init -q
git -C "$repo" config user.name Test
git -C "$repo" config user.email test@example.invalid
mkdir -p "$repo/internal/platform/database/migrations"
dir=$repo/internal/platform/database/migrations
printf 'first\n' > "$dir/000001_first.sql"
git -C "$repo" add .
git -C "$repo" commit -qm base
base=$(git -C "$repo" rev-parse HEAD)
printf 'second\n' > "$dir/000002_second.sql"
git -C "$repo" add .
git -C "$repo" commit -qm add
(cd "$repo" && sh "$script" "$base" HEAD)
git -C "$repo" mv internal/platform/database/migrations/000002_second.sql internal/platform/database/migrations/000003_second.sql
git -C "$repo" commit -qm renumber
git -C "$repo" mv internal/platform/database/migrations/000003_second.sql internal/platform/database/migrations/000002_second.sql
git -C "$repo" commit -qm restore
if (cd "$repo" && sh "$script" "$base" HEAD >/dev/null 2>&1); then
	echo 'expected restored migration rename to fail' >&2
	exit 1
fi
git -C "$repo" checkout -q "$base" --detach
printf 'changed\n' > "$dir/000001_first.sql"
git -C "$repo" add .
git -C "$repo" commit -qm modify
if (cd "$repo" && sh "$script" "$base" HEAD >/dev/null 2>&1); then
	echo 'expected modified existing migration to fail' >&2
	exit 1
fi
echo 'migration history checks passed'
