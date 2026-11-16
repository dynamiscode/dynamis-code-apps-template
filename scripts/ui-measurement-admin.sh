#!/bin/sh
set -eu

work="$(mktemp -d /tmp/dynamis-code-admin-measure.XXXXXX)"
port="${PLAYWRIGHT_PORT:-58096}"
pid=""
cleanup() {
  if [ -n "$pid" ]; then
    kill "$pid" >/dev/null 2>&1 || true
    wait "$pid" >/dev/null 2>&1 || true
  fi
  rm -rf "$work"
}
trap cleanup EXIT INT TERM

if curl --silent "http://127.0.0.1:$port/health/ready" >/dev/null 2>&1; then
  echo "port $port is already in use; set PLAYWRIGHT_PORT" >&2
  exit 1
fi

BOOTSTRAP_ADMIN_PASSWORD=playwright-password-123 DATABASE_DRIVER=sqlite SQLITE_PATH="$work/app.db" \
  go run ./cmd/bootstrap-admin -email playwright@example.com -workspace "UI Baselines" >/dev/null
go build -o "$work/server" ./cmd/server
MULTI_WORKSPACE_ENABLED=true HTTP_REQUESTS_PER_MINUTE=10000 HTTP_AUTH_REQUESTS_PER_MINUTE=1000 \
  ENCRYPTION_KEY= WEBHOOK_ENCRYPTION_KEY=0000000000000000000000000000000000000000000000000000000000000000 \
  DATABASE_DRIVER=sqlite SQLITE_PATH="$work/app.db" HTTP_ADDRESS="127.0.0.1:$port" \
  "$work/server" >"$work/server.log" 2>&1 &
pid=$!
if ! curl --fail --silent --retry 30 --retry-delay 1 --retry-connrefused \
  --max-time 2 "http://127.0.0.1:$port/health/ready" >/dev/null; then
  echo "server did not become ready" >&2
  exit 1
fi
export PLAYWRIGHT_BASE_URL="http://127.0.0.1:$port"
export PLAYWRIGHT_EMAIL=playwright@example.com PLAYWRIGHT_PASSWORD=playwright-password-123
node scripts/ui-measurement-admin.mjs
