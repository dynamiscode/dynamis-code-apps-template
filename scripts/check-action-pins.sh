#!/bin/sh
set -eu

violations="$(grep -R -n -E '^[[:space:]]*-[[:space:]]+uses:' .github/workflows \
  | grep -Ev '@[0-9a-f]{40}[[:space:]]+# v[0-9]' || true)"
if [ -n "$violations" ]; then
	echo "workflow actions must use a full commit SHA and version comment:" >&2
	echo "$violations" >&2
	exit 1
fi
