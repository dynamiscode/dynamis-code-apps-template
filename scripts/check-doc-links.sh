#!/bin/sh
set -eu

# Reports relative Markdown links whose target path does not exist.
# Anchors are not validated; external, mailto, and absolute links are skipped.
status=0
for file in $(find . -name '*.md' -not -path './.git/*' -not -path './node_modules/*'); do
	dir="$(dirname "$file")"
	targets="$(grep -oE '\]\([^)]+\)' "$file" | sed -e 's/^](//' -e 's/)$//' -e 's/#.*$//' | grep -vE '^(https?:|mailto:|/|$)' || true)"
	for target in $targets; do
		if [ ! -e "$dir/$target" ]; then
			echo "broken relative Markdown link: $file -> $target" >&2
			status=1
		fi
	done
done
exit "$status"
