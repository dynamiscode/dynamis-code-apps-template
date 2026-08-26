#!/bin/sh
set -eu

project="dynamis-code-smoke-$$"
requested_port="${SMOKE_PORT:-0}"
port="$requested_port"
cookies="$(mktemp /tmp/dynamis-code-smoke-cookies.XXXXXX)"

cleanup() {
  rm -f "$cookies"
  APP_PORT="$requested_port" docker compose -p "$project" down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

APP_PORT="$requested_port" docker compose -p "$project" up --build --detach
if [ "$port" = "0" ]; then
  port="$(docker compose -p "$project" port app 8080 | head -1 | awk -F: '{print $NF}')"
fi

attempt=0
until curl --fail --silent --show-error "http://127.0.0.1:$port/health/ready" >/dev/null; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 60 ]; then
    APP_PORT="$requested_port" docker compose -p "$project" logs app
    exit 1
  fi
  sleep 1
done

curl --fail --silent --show-error "http://127.0.0.1:$port/health/live" >/dev/null
curl --fail --silent --show-error "http://127.0.0.1:$port/api/openapi.json" | grep -q '"openapi":"3.1.0"\|"openapi": "3.1.0"'

APP_PORT="$requested_port" docker compose -p "$project" exec -T \
  -e BOOTSTRAP_PASSWORD=smoke-password-123 app /bootstrap-admin \
  -email smoke@example.com -workspace Smoke >/dev/null
curl --fail --silent --show-error -c "$cookies" "http://127.0.0.1:$port/login" >/dev/null
login_csrf="$(awk '$6 == "login_csrf" { print $7 }' "$cookies")"
home="$(curl --fail --silent --show-error -L -b "$cookies" -c "$cookies" \
  --data-urlencode "email=smoke@example.com" \
  --data-urlencode "password=smoke-password-123" \
  --data-urlencode "csrf=$login_csrf" "http://127.0.0.1:$port/login")"
workspace_path="$(printf '%s' "$home" | grep -o '/workspaces/[^" ]*' | head -1)"
items_path="$workspace_path/items"
csrf="$(awk '$6 == "csrf" { print $7 }' "$cookies")"
curl --fail --silent --show-error -b "$cookies" -o /dev/null \
  --data-urlencode "title=container smoke item" \
  --data-urlencode "idempotency_key=container-smoke" \
  --data-urlencode "csrf=$csrf" "http://127.0.0.1:$port$items_path"
curl --fail --silent --show-error -b "$cookies" "http://127.0.0.1:$port$items_path" | grep -q 'container smoke item'

APP_PORT="$requested_port" docker compose -p "$project" restart app >/dev/null
if [ "$requested_port" = "0" ]; then
  port="$(docker compose -p "$project" port app 8080 | head -1 | awk -F: '{print $NF}')"
fi
attempt=0
until curl --fail --silent --show-error "http://127.0.0.1:$port/health/ready" >/dev/null; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 30 ]; then
    exit 1
  fi
  sleep 1
done
curl --fail --silent --show-error -b "$cookies" "http://127.0.0.1:$port$items_path" | grep -q 'container smoke item'
