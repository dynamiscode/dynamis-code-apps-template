#!/bin/sh
set -eu

work="$(mktemp -d /tmp/dynamis-code-playwright.XXXXXX)"
port="${PLAYWRIGHT_PORT:-58095}"
oidc_port="${PLAYWRIGHT_OIDC_PORT:-$((port + 1))}"
setup_port="${PLAYWRIGHT_SETUP_PORT:-$((port + 2))}"
members_port="${PLAYWRIGHT_MEMBERS_PORT:-$((port + 3))}"
pid=""
oidc_pid=""
setup_pid=""
members_pid=""

if [ "${UI_MEASURE:-0}" != 1 ] && { [ "$members_port" = "$port" ] || [ "$members_port" = "$oidc_port" ] || [ "$members_port" = "$setup_port" ]; }; then
  echo "Members fixture port $members_port conflicts with another fixture port" >&2
  exit 1
fi

cleanup() {
  if [ -n "$members_pid" ]; then
    kill "$members_pid" >/dev/null 2>&1 || true
    wait "$members_pid" >/dev/null 2>&1 || true
  fi
  if [ -n "$setup_pid" ]; then
    kill "$setup_pid" >/dev/null 2>&1 || true
    wait "$setup_pid" >/dev/null 2>&1 || true
  fi
  if [ -n "$oidc_pid" ]; then
    kill "$oidc_pid" >/dev/null 2>&1 || true
    wait "$oidc_pid" >/dev/null 2>&1 || true
  fi
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
if curl --noproxy '*' --silent "http://127.0.0.1:$oidc_port/health/ready" >/dev/null 2>&1; then
  echo "OIDC fixture port $oidc_port is already in use; set PLAYWRIGHT_OIDC_PORT" >&2
  exit 1
fi
if curl --noproxy '*' --silent "http://127.0.0.1:$setup_port/health/ready" >/dev/null 2>&1; then
  echo "setup fixture port $setup_port is already in use; set PLAYWRIGHT_SETUP_PORT" >&2
  exit 1
fi
if [ "${UI_MEASURE:-0}" != 1 ]; then
  if curl --noproxy '*' --silent "http://127.0.0.1:$members_port/health/ready" >/dev/null 2>&1; then
    echo "Members fixture port $members_port is already in use; set PLAYWRIGHT_MEMBERS_PORT" >&2
    exit 1
  fi
fi

go build -o "$work/bootstrap-admin" ./cmd/bootstrap-admin
go build -o "$work/server" ./cmd/server
go build -o "$work/seed-member" ./scripts/playwright/seed-member
go build -tags oidcfixture -o "$work/oidc-fixture" ./internal/web/testdata/oidcfixture
env -i HOME="$work" TMPDIR="$work" BOOTSTRAP_ADMIN_PASSWORD=playwright-password-123 \
  DATABASE_DRIVER=sqlite SQLITE_PATH="$work/app.db" STORAGE_DRIVER=local STORAGE_LOCAL_PATH="$work/files" \
  "$work/bootstrap-admin" -email playwright@example.com -workspace "UI Baselines" >/dev/null
export PLAYWRIGHT_SQLITE_PATH="$work/app.db"
export PLAYWRIGHT_EMAIL=playwright@example.com PLAYWRIGHT_PASSWORD=playwright-password-123
export PLAYWRIGHT_MEMBER_EMAIL=playwright-member@example.com
export PLAYWRIGHT_MEMBER_PASSWORD=playwright-member-password-123
"$work/seed-member"
if [ "${UI_MEASURE:-0}" != 1 ]; then
  env -i HOME="$work" TMPDIR="$work" BOOTSTRAP_ADMIN_PASSWORD=playwright-password-123 \
    DATABASE_DRIVER=sqlite SQLITE_PATH="$work/members.db" STORAGE_DRIVER=local STORAGE_LOCAL_PATH="$work/members-files" \
    "$work/bootstrap-admin" -email playwright@example.com -workspace "UI Baselines" >/dev/null
  env -i PATH="$PATH" HOME="$work" TMPDIR="$work" PLAYWRIGHT_EMAIL=playwright@example.com \
    PLAYWRIGHT_MEMBER_EMAIL=playwright-member@example.com PLAYWRIGHT_MEMBER_PASSWORD=playwright-member-password-123 \
    PLAYWRIGHT_SQLITE_PATH="$work/members.db" "$work/seed-member"
fi
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
  WEBHOOK_ENCRYPTION_KEY=0000000000000000000000000000000000000000000000000000000000000000 \
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

env -i HOME="$work" TMPDIR="$work" OIDC_FIXTURE_ADDRESS="127.0.0.1:$oidc_port" \
  OIDC_FIXTURE_DATABASE="$work/oidc-fixture.db" \
  "$work/oidc-fixture" >"$work/oidc-fixture.log" 2>&1 &
oidc_pid=$!
if ! curl --noproxy '*' --fail --silent --retry 30 --retry-delay 1 --retry-connrefused \
  --max-time 2 "http://127.0.0.1:$oidc_port/health/ready" >/dev/null; then
  cat "$work/oidc-fixture.log"
  exit 1
fi

env -i HOME="$work" TMPDIR="$work" MULTI_WORKSPACE_ENABLED=true \
  HTTP_REQUESTS_PER_MINUTE=10000 HTTP_AUTH_REQUESTS_PER_MINUTE=1000 \
  DATABASE_DRIVER=sqlite SQLITE_PATH="$work/setup.db" STORAGE_DRIVER=local STORAGE_LOCAL_PATH="$work/setup-files" \
  HTTP_ADDRESS="127.0.0.1:$setup_port" \
  "$work/server" >"$work/setup-server.log" 2>&1 &
setup_pid=$!
if ! curl --noproxy '*' --fail --silent --retry 30 --retry-delay 1 --retry-connrefused \
  --max-time 2 "http://127.0.0.1:$setup_port/health/ready" >/dev/null; then
  cat "$work/setup-server.log"
  exit 1
fi

if [ "${UI_MEASURE:-0}" != 1 ]; then
  env -i HOME="$work" TMPDIR="$work" MULTI_WORKSPACE_ENABLED=true \
    HTTP_REQUESTS_PER_MINUTE=10000 HTTP_AUTH_REQUESTS_PER_MINUTE=1000 \
    DATABASE_DRIVER=sqlite SQLITE_PATH="$work/members.db" STORAGE_DRIVER=local STORAGE_LOCAL_PATH="$work/members-files" \
    HTTP_ADDRESS="127.0.0.1:$members_port" \
    "$work/server" >"$work/members-server.log" 2>&1 &
  members_pid=$!
  if ! curl --noproxy '*' --fail --silent --retry 30 --retry-delay 1 --retry-connrefused \
    --max-time 2 "http://127.0.0.1:$members_port/health/ready" >/dev/null; then
    cat "$work/members-server.log"
    exit 1
  fi
fi

export PLAYWRIGHT_BASE_URL="http://127.0.0.1:$port"
export PLAYWRIGHT_OIDC_BASE_URL="http://127.0.0.1:$oidc_port"
export PLAYWRIGHT_SETUP_BASE_URL="http://127.0.0.1:$setup_port"
if [ "${UI_MEASURE:-0}" = 1 ]; then
  node scripts/ui-measurement.mjs
elif [ "${UI_MEASURE:-0}" = workspace ]; then
  node scripts/ui-measurement-workspace.mjs
elif [ "${UI_MEASURE:-0}" = interactions ]; then
  node scripts/ui-measurement-interactions.mjs
elif [ "${UI_MEASURE:-0}" = gaps ]; then
  node scripts/ui-measurement-gaps.mjs
elif [ "${UI_MEASURE:-0}" = states ]; then
  node scripts/ui-measurement-workspace-states.mjs
else
  export PLAYWRIGHT_MEMBERS_BASE_URL="http://127.0.0.1:$members_port"
  npm run test:journeys -- "$@"
fi
