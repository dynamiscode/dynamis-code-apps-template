#!/bin/sh
set -eu

work="$(mktemp -d /tmp/dynamis-code-playwright.XXXXXX)"
port="${PLAYWRIGHT_PORT:-58095}"
pid=""

cleanup() {
  if [ -n "$pid" ]; then
    kill "$pid" >/dev/null 2>&1 || true
    wait "$pid" >/dev/null 2>&1 || true
  fi
  rm -rf "$work"
}
trap cleanup EXIT INT TERM

if curl --noproxy '*' --silent "http://127.0.0.1:$port/health/ready" >/dev/null 2>&1; then
  echo "port $port is already in use; set PLAYWRIGHT_PORT" >&2
  exit 1
fi

go build -o "$work/bootstrap-admin" ./cmd/bootstrap-admin
go build -o "$work/server" ./cmd/server
env -i HOME="$work" TMPDIR="$work" BOOTSTRAP_ADMIN_PASSWORD=playwright-password-123 \
  DATABASE_DRIVER=sqlite SQLITE_PATH="$work/app.db" STORAGE_DRIVER=local STORAGE_LOCAL_PATH="$work/files" \
  "$work/bootstrap-admin" -email playwright@example.com -workspace "UI Baselines" >/dev/null
if [ "${PLAYWRIGHT_SEED_NOTIFICATION:-0}" = 1 ]; then
  env -i PATH="$PATH" HOME="$work" TMPDIR="$work" SQLITE_PATH="$work/app.db" \
    sqlite3 -batch -bail "$work/app.db" <<'SQL'
INSERT INTO notifications (user_id, workspace_id, notification_type, title, body, created_at)
SELECT u.id, wm.workspace_id, 'system', 'Playwright notification', 'Fixture body',
       strftime('%Y-%m-%dT%H:%M:%fZ','now')
FROM users u JOIN workspace_members wm ON wm.user_id=u.id
WHERE u.email='playwright@example.com' LIMIT 1;
SQL
  export PLAYWRIGHT_NOTIFICATION_SEEDED=1
fi
env -i HOME="$work" TMPDIR="$work" MULTI_WORKSPACE_ENABLED=true \
  HTTP_REQUESTS_PER_MINUTE=10000 HTTP_AUTH_REQUESTS_PER_MINUTE=1000 \
  DATABASE_DRIVER=sqlite SQLITE_PATH="$work/app.db" STORAGE_DRIVER=local STORAGE_LOCAL_PATH="$work/files" \
  HTTP_SSE_POLL_INTERVAL="${PLAYWRIGHT_SSE_POLL_INTERVAL:-1s}" \
  HTTP_SSE_MAX_LIFETIME="${PLAYWRIGHT_SSE_MAX_LIFETIME:-5m}" \
  HTTP_ADDRESS="127.0.0.1:$port" \
  "$work/server" >"$work/server.log" 2>&1 &
pid=$!

if ! curl --noproxy '*' --fail --silent --retry 30 --retry-delay 1 --retry-connrefused \
  --max-time 2 "http://127.0.0.1:$port/health/ready" >/dev/null; then
  cat "$work/server.log"
  exit 1
fi

export PLAYWRIGHT_BASE_URL="http://127.0.0.1:$port"
export PLAYWRIGHT_SQLITE_PATH="$work/app.db"
export PLAYWRIGHT_EMAIL=playwright@example.com PLAYWRIGHT_PASSWORD=playwright-password-123
if [ "${UI_MEASURE:-0}" = 1 ]; then
  node scripts/ui-measurement.mjs
else
  npm run test:journeys -- "$@"
fi
