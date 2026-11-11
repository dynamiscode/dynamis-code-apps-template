#!/bin/sh
# Offline fixture tests for check-logger-injection.sh.
set -eu

script="$(cd "$(dirname "$0")" && pwd)/check-logger-injection.sh"
root="$(mktemp -d)"
trap 'rm -rf "$root"' EXIT

mkdir -p "$root/internal/web" "$root/internal/bootstrap"
cat >"$root/internal/web/bad.go" <<'EOF'
package web

import "log/slog"

func f() { slog.WarnContext(nil, "boom") }
EOF
if "$script" "$root" 2>/dev/null; then
	echo "expected violation in internal/web/bad.go to fail" >&2
	exit 1
fi

rm "$root/internal/web/bad.go"
cat >"$root/internal/bootstrap/app.go" <<'EOF'
package bootstrap

import "log/slog"

func f() { slog.Info("composition root") }
EOF
cat >"$root/internal/web/good.go" <<'EOF'
package web

import "log/slog"

type Handler struct{ logger *slog.Logger }

func newHandler() *Handler { return &Handler{logger: slog.Default()} }
EOF
"$script" "$root"
echo "check-logger-injection fixtures passed"
