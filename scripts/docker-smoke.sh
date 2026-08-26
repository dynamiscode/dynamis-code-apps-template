#!/bin/sh
set -eu

project="dynamis-code-smoke-$$"
port="${SMOKE_PORT:-58088}"

cleanup() {
  APP_PORT="$port" docker compose -p "$project" down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

APP_PORT="$port" docker compose -p "$project" up --build --detach

attempt=0
until curl --fail --silent --show-error "http://127.0.0.1:$port/health/ready" >/dev/null; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 60 ]; then
    APP_PORT="$port" docker compose -p "$project" logs app
    exit 1
  fi
  sleep 1
done

curl --fail --silent --show-error "http://127.0.0.1:$port/health/live" >/dev/null
curl --fail --silent --show-error "http://127.0.0.1:$port/api/openapi.json" | grep -q '"openapi":"3.1.0"\|"openapi": "3.1.0"'

APP_PORT="$port" docker compose -p "$project" restart app >/dev/null
attempt=0
until curl --fail --silent --show-error "http://127.0.0.1:$port/health/ready" >/dev/null; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 30 ]; then
    exit 1
  fi
  sleep 1
done
