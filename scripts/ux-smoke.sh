#!/bin/sh
# Drive the form UX patterns in headless Chrome against a fresh server.
# The run starts with no owner, so it also walks the setup wizard.
set -eu

work="$(mktemp -d /tmp/dynamis-code-ux.XXXXXX)"
port="${UX_PORT:-58093}"
pid=""

cleanup() {
  if [ -n "$pid" ]; then
    kill "$pid" >/dev/null 2>&1 || true
    wait "$pid" >/dev/null 2>&1 || true
  fi
  rm -rf "$work"
}
trap cleanup EXIT INT TERM

if curl --silent "http://127.0.0.1:$port/" >/dev/null 2>&1; then
  echo "port $port is already in use; set UX_PORT" >&2
  exit 1
fi
go build -o "$work/server" ./cmd/server
MULTI_WORKSPACE_ENABLED=true DATABASE_DRIVER=sqlite SQLITE_PATH="$work/app.db" HTTP_ADDRESS="127.0.0.1:$port" \
  "$work/server" >"$work/server.log" 2>&1 &
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

UX_BASE_URL="http://127.0.0.1:$port" npm run test:ux
