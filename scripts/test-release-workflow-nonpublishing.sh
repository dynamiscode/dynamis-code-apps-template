#!/bin/sh
set -eu

root="$(cd "$(dirname "$0")/.." && pwd)"
workflow="$root/.github/workflows/release.yml"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

check_nonpublishing() {
	file="$1"
	! grep -Eiq \
		'(^|[[:space:]])[[:alnum:]_-]+:[[:space:]]*write([[:space:]]|$)|permissions:[[:space:]]*write-all|push:[[:space:]]*true|actions/upload-artifact@|docker/(login|build-push)-action@|actions/attest-build-provenance@|sigstore/cosign-installer@|gh[[:space:]]+release[[:space:]]+(create|upload|edit|delete)|gh[[:space:]]+api.*--method[[:space:]]+(POST|PUT|PATCH|DELETE)|(^|[[:space:]])(cosign[[:space:]]+(sign|attest|sign-blob)|docker[[:space:]]+push|oras[[:space:]]+push)' \
		"$file"
}

if ! check_nonpublishing "$workflow"; then
	echo "release workflow contains a publication capability" >&2
	exit 1
fi

for forbidden in \
	'packages: write' \
	'push: true' \
	'uses: actions/upload-artifact@v1' \
	'run: gh release create v1.2.3' \
	'run: cosign sign --yes image'
do
	printf '%s\n' "$forbidden" >"$tmp"
	if check_nonpublishing "$tmp"; then
		echo "publication fixture was accepted: $forbidden" >&2
		exit 1
	fi
done

echo "release workflow nonpublishing check: passed"
