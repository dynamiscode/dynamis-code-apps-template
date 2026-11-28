#!/bin/sh
set -eu

script=$(CDPATH= cd -- "$(dirname "$0")" && pwd)/check-postgres-test-results.sh
results=$(mktemp)
trap 'rm -f "$results"' EXIT
package=example.com/dynamis-code/apps-template/internal/platform/database
for test in TestEngagementActionCompletionUpgrade/postgres TestPostgresEngagementNameUpgrade TestPostgresWorkspaceCurrenciesUpgrade TestPostgresRemoveItemsUpgrade; do
	printf '{"Package":"%s","Test":"%s","Action":"pass"}\n' "$package" "$test" >> "$results"
done
sh "$script" "$results" >/dev/null
for action in skip fail; do
	cp "$results" "$results.tmp"
	printf '{"Package":"%s","Test":"TestPostgresRemoveItemsUpgrade","Action":"%s"}\n' "$package" "$action" >> "$results.tmp"
	if sh "$script" "$results.tmp" >/dev/null 2>&1; then
		echo "expected $action to fail" >&2
		exit 1
	fi
done
head -n 3 "$results" > "$results.tmp"
if sh "$script" "$results.tmp" >/dev/null 2>&1; then
	echo 'expected missing test to fail' >&2
	exit 1
fi
rm -f "$results.tmp"
echo 'PostgreSQL result gate checks passed'
