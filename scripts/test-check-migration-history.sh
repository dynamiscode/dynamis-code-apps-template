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

# The exception pins a real commit and blob, so simulate Git's responses to
# exercise each condition without rewriting that commit in the test repository.
mkdir "$repo/bin"
cat > "$repo/bin/git" <<'EOF'
#!/bin/sh
commit=4c58ed48f3318aa378dfd43362fcd3044c8f42ea
path=internal/platform/database/migrations
blob=180440452d6560b7a80f6ba318f243c633b0d973
case "$1" in
	merge-base) echo base ;;
	rev-list)
		if [ "$scenario" = other-commit ]; then echo other; else echo "$commit"; fi ;;
	diff-tree)
		if [ "$scenario" = other-deletion ]; then
			echo "$path/000050_other.sql"
		else
			echo "$path/000051_public_intake_encrypted_token.sql"
		fi ;;
	cat-file) [ "$scenario" = base-present ] ;;
	rev-parse)
		case "$scenario:$2" in
			old-hash:*^:*|new-hash:*:*/000055_*) echo changed ;;
			*) echo "$blob" ;;
		esac ;;
	diff) : ;;
	*) exit 2 ;;
esac
EOF
chmod +x "$repo/bin/git"
PATH="$repo/bin:$PATH" scenario=allowed sh "$script" base head
for scenario in base-present old-hash new-hash other-deletion other-commit; do
	if PATH="$repo/bin:$PATH" scenario="$scenario" sh "$script" base head >/dev/null 2>&1; then
		echo "expected migration exception to reject $scenario" >&2
		exit 1
	fi
done
echo 'migration history checks passed'
