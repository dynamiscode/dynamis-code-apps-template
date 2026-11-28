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

bin="${BROWSER_SMOKE_BIN_DIR:-$work/bin}"
if [ -z "${BROWSER_SMOKE_BIN_DIR:-}" ]; then
  mkdir -p "$bin"
  go build -o "$bin/" ./cmd/server ./cmd/bootstrap-admin ./cmd/browsersmoke
fi
BOOTSTRAP_ADMIN_PASSWORD=a11y-password-123 DATABASE_DRIVER=sqlite SQLITE_PATH="$work/app.db" \
  "$bin/bootstrap-admin" -email a11y@example.com -workspace Accessibility >/dev/null
# Run the built server directly so cleanup can kill it without leaving a go run child.
MULTI_WORKSPACE_ENABLED=true MCP_OAUTH_ENABLED=true APP_PUBLIC_URL="http://127.0.0.1:$port" \
  HTTP_SECURE=false HTTP_REQUESTS_PER_MINUTE=10000 HTTP_AUTH_REQUESTS_PER_MINUTE=1000 \
  DATABASE_DRIVER=sqlite SQLITE_PATH="$work/app.db" HTTP_ADDRESS="127.0.0.1:$port" \
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

for locale in en es; do
  A11Y_BASE_URL="http://127.0.0.1:$port" A11Y_EMAIL=a11y@example.com \
    A11Y_PASSWORD=a11y-password-123 A11Y_LOCALE="$locale" "$bin/browsersmoke" -mode accessibility
done
