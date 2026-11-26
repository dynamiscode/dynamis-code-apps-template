#!/bin/sh
set -eu

if [ "$#" -gt 1 ]; then
	echo "Usage: sh scripts/profile-preflight.sh [base-ref]" >&2
	exit 2
fi
base=${1:-origin/main}
root=$(git rev-parse --show-toplevel)
cd "$root"
git rev-parse --verify --quiet "$base^{commit}" >/dev/null || {
	echo "Profile preflight: base commit not found: $base" >&2
	exit 2
}

# Shared application code can affect any generated profile; do not guess which.
set -- cmd/ internal/ api/ scripts/actioncheck/ go.mod go.sum Makefile package.json package-lock.json .env.example
tracked=$(git diff --no-renames --name-only "$base" -- "$@")
untracked=$(git ls-files --others --exclude-standard -- "$@")
if [ -z "$tracked$untracked" ]; then
	echo "Profile preflight: no generated application code changes; skipped."
	exit 0
fi

echo "Profile preflight: testing generated profile matrix against working tree (base: $base)."
go test -count=1 -timeout 30m ./cmd/template-init -run '^(TestGenerateCatalogProfile|TestGenerateWithClientsAndFilesProfilesBuilds|TestGenerateWithoutFilesPrunesFilesTemplAndBuilds|TestGenerateWithoutAgentPrunesAgentSurfaceAndBuilds|TestGenerateWithClientsProfileRecordsLockAndBuilds|TestGenerateWithoutClientsPrunesClientsSurfaceAndBuilds)$'
