#!/bin/sh
set -eu

violations="$(grep -R -n -E '^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]*[^[:space:]#]' .github/workflows \
  | grep -Ev '@[0-9a-f]{40}[[:space:]]+# v[0-9]' || true)"
if [ -n "$violations" ]; then
	echo "workflow actions must use a full commit SHA and version comment:" >&2
	echo "$violations" >&2
	exit 1
fi

invalid=0
actions="$(grep -R -h -E '^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]*[^[:space:]#]' .github/workflows \
  | sed -E 's/^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]*([^[:space:]#]+).*/\2/' \
  | sort -u || true)"

# Uses git over HTTPS instead of the REST API to avoid API rate limits and 403s.
# Accepts a SHA that is a branch tip, a peeled tag commit, or a lightweight tag.
# Rejects a SHA that only names an annotated tag object.
check_commit_ref() {
	action="$1"
	repo="$(printf '%s' "${action%@*}" | cut -d/ -f1,2)"
	sha="${action##*@}"
	if ! refs="$(GIT_TERMINAL_PROMPT=0 git ls-remote --tags --heads "https://github.com/$repo.git")"; then
		echo "could not list refs for $repo" >&2
		invalid=1
		return
	fi
	if ! printf '%s\n' "$refs" | awk -v target="$sha" '
		{ sha = $1; ref = $2 }
		ref ~ /\^\{\}$/ { peeled[substr(ref, 1, length(ref) - 3)] = 1; if (sha == target) found = 1; next }
		ref ~ /^refs\/heads\// { if (sha == target) found = 1; next }
		ref ~ /^refs\/tags\// { tag[ref] = sha }
		END {
			for (ref in tag) if (!(ref in peeled) && tag[ref] == target) found = 1
			exit !found
		}'; then
		echo "workflow action must reference a commit object: $action" >&2
		invalid=1
	fi
}

for action in $actions; do
	check_commit_ref "$action"
done

test "$invalid" -eq 0
