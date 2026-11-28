#!/bin/sh
set -eu

work="$(mktemp -d /tmp/dynamis-code-webmcp.XXXXXX)"
port="${WEBMCP_PORT:-58090}"
pid=""

cleanup() {
  if [ -n "$pid" ]; then
    kill "$pid" >/dev/null 2>&1 || true
    wait "$pid" >/dev/null 2>&1 || true
  fi
  rm -rf "$work"
}
trap cleanup EXIT INT TERM

bin="${BROWSER_SMOKE_BIN_DIR:-$work/bin}"
if [ -z "${BROWSER_SMOKE_BIN_DIR:-}" ]; then
  mkdir -p "$bin"
  go build -o "$bin/" ./cmd/server ./cmd/bootstrap-admin ./cmd/browsersmoke
fi
BOOTSTRAP_ADMIN_PASSWORD=webmcp-password-123 DATABASE_DRIVER=sqlite SQLITE_PATH="$work/app.db" \
  "$bin/bootstrap-admin" -email webmcp@example.com -workspace WebMCP >/dev/null
# Run the built server directly so cleanup can kill it without leaving a go run child.
MULTI_WORKSPACE_ENABLED=true HTTP_SECURE=false DATABASE_DRIVER=sqlite SQLITE_PATH="$work/app.db" HTTP_ADDRESS="127.0.0.1:$port" \
  "$bin/server" >"$work/server.log" 2>&1 &
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

WEBMCP_BASE_URL="http://127.0.0.1:$port" WEBMCP_EMAIL=webmcp@example.com \
  WEBMCP_PASSWORD=webmcp-password-123 "$bin/browsersmoke" -mode webmcp
