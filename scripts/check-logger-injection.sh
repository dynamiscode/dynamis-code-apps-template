#!/bin/sh
set -eu

# Fails when non-test code under internal/ logs through the process-wide slog
# default. Packages receive a *slog.Logger from their constructor (AGENTS.md:
# manual constructor injection). internal/bootstrap is the composition root and
# may call slog directly. Passing slog.Default() as a value is the injection
# pattern and is allowed.
root="${1:-.}"
status=0
matches="$(find "$root/internal" -name '*.go' -not -name '*_test.go' -not -path '*/internal/bootstrap/*' -exec grep -nE 'slog\.(Info|Warn|Error|Debug|Log)[A-Za-z]*\(' {} + 2>/dev/null || true)"
if [ -n "$matches" ]; then
	echo "$matches" >&2
	echo "global slog logging found under internal/: inject a *slog.Logger through the constructor instead" >&2
	status=1
fi
exit "$status"
