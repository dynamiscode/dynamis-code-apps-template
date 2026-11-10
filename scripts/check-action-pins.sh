#!/bin/sh
set -eu

violations="$(grep -R -n -E '^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]*[^[:space:]#]' .github/workflows \
  | grep -Ev '@[0-9a-f]{40}[[:space:]]+# v[0-9]' || true)"
if [ -n "$violations" ]; then
	echo "workflow actions must use a full commit SHA and version comment:" >&2
	echo "$violations" >&2
	exit 1
fi

api_url="${GITHUB_API_URL:-https://api.github.com}"
invalid=0
actions="$(grep -R -h -E '^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]*[^[:space:]#]' .github/workflows \
  | sed -E 's/^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]*([^[:space:]#]+).*/\2/' \
  | sort -u || true)"

check_commit_object() {
	action="$1"
	repo="$(printf '%s' "${action%@*}" | cut -d/ -f1,2)"
	sha="${action##*@}"
	set -- \
		--fail --silent --show-error --location \
		--connect-timeout 10 --max-time 30 \
		--retry 3 --retry-delay 5 --retry-all-errors \
		--header 'Accept: application/vnd.github+json' \
		--header 'X-GitHub-Api-Version: 2022-11-28'
	if [ -n "${GITHUB_TOKEN:-}" ]; then
		set -- "$@" --header "Authorization: Bearer $GITHUB_TOKEN"
	fi
	if ! curl "$@" "$api_url/repos/$repo/git/commits/$sha" >/dev/null; then
		echo "workflow action must reference a commit object: $action" >&2
		invalid=1
	fi
}

for action in $actions; do
	check_commit_object "$action"
done

test "$invalid" -eq 0
