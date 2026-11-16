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

go build -o "$work/bootstrap" ./cmd/bootstrap-admin
go build -o "$work/server" ./cmd/server
sh scripts/ui-measurement-admin-env.sh "$work" "$port" "$work/bootstrap" \
  -email playwright@example.com -workspace "UI Baselines" >/dev/null
sh scripts/ui-measurement-admin-env.sh "$work" "$port" "$work/server" >"$work/server.log" 2>&1 &
pid=$!
if ! curl --fail --silent --retry 30 --retry-delay 1 --retry-connrefused \
  --max-time 2 "http://127.0.0.1:$port/health/ready" >/dev/null; then
  echo "server did not become ready" >&2
  exit 1
fi
export PLAYWRIGHT_BASE_URL="http://127.0.0.1:$port"
export PLAYWRIGHT_EMAIL=playwright@example.com PLAYWRIGHT_PASSWORD=playwright-password-123
node scripts/ui-measurement-admin.mjs
