#!/bin/sh
set -eu

work="$(mktemp -d /tmp/dynamis-code-a11y.XXXXXX)"
port="${A11Y_PORT:-58089}"
pid=""

cleanup() {
  if [ -n "$pid" ]; then
    kill "$pid" >/dev/null 2>&1 || true
    wait "$pid" >/dev/null 2>&1 || true
  fi
  rm -rf "$work"
}
trap cleanup EXIT INT TERM

BOOTSTRAP_PASSWORD=a11y-password-123 DATABASE_DRIVER=sqlite SQLITE_PATH="$work/app.db" \
  go run ./cmd/bootstrap-admin -email a11y@example.com -workspace Accessibility >/dev/null
DATABASE_DRIVER=sqlite SQLITE_PATH="$work/app.db" HTTP_ADDRESS="127.0.0.1:$port" \
  go run ./cmd/server >"$work/server.log" 2>&1 &
pid=$!

attempt=0
until curl --fail --silent "http://127.0.0.1:$port/health/ready" >/dev/null; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 30 ]; then
    cat "$work/server.log"
    exit 1
  fi
  sleep 1
done

A11Y_BASE_URL="http://127.0.0.1:$port" A11Y_EMAIL=a11y@example.com \
  A11Y_PASSWORD=a11y-password-123 npm run test:a11y
