#!/bin/sh
set -eu

if [ -n "${POSTGRES_PACKAGES:-}" ]; then
	set -f
	# Package arguments are whitespace-separated, never evaluated as shell code.
	set -- $POSTGRES_PACKAGES
	[ "$#" -gt 0 ] || { echo 'POSTGRES_PACKAGES must contain package paths' >&2; exit 2; }
	for package do
		case "$package" in
			./...|./[A-Za-z0-9_]* ) ;;
			*) echo "Invalid package path: $package" >&2; exit 2 ;;
		esac
		case "$package" in
			*[!A-Za-z0-9_./-]*|*//*|*/../*|*/..|*/./*|*/.|*/.../*)
				echo "Invalid package path: $package" >&2; exit 2 ;;
		esac
		case "$package" in
			./...) directory=. ;;
			*/...) directory=${package%/...} ;;
			*) directory=$package ;;
		esac
		[ -d "$directory" ] || { echo "Package directory not found: $directory" >&2; exit 2; }
	done
fi

name="dynamis-code-test-pg-$$"
trap 'docker rm -f "$name" >/dev/null 2>&1 || true' EXIT
docker run -d --name "$name" -e POSTGRES_DB=dynamis_code_test -e POSTGRES_USER=postgres -e POSTGRES_HOST_AUTH_METHOD=trust -p 127.0.0.1::5432 postgres:14.24-alpine3.24@sha256:727876d274666da0b92a445390ba093c84b8e9f8343e1c53cd4e9a7ab2d85310 >/dev/null
port=$(docker port "$name" 5432/tcp | head -n1 | sed 's/.*://')
for i in $(seq 1 60); do
	docker exec "$name" pg_isready -h 127.0.0.1 -U postgres -d dynamis_code_test >/dev/null 2>&1 && break
	sleep 1
done
docker exec "$name" pg_isready -h 127.0.0.1 -U postgres -d dynamis_code_test >/dev/null
if [ -n "${POSTGRES_PACKAGES:-}" ]; then
	POSTGRES_TEST_URL="postgres://postgres@127.0.0.1:$port/dynamis_code_test?sslmode=disable" go test -p 1 "$@" -count=1
else
	POSTGRES_TEST_URL="postgres://postgres@127.0.0.1:$port/dynamis_code_test?sslmode=disable" go test -p 1 ./internal/identity/... ./internal/files/... -count=1
	docker exec "$name" createdb -U postgres scheduling_test
	POSTGRES_TEST_URL="postgres://postgres@127.0.0.1:$port/scheduling_test?sslmode=disable" go test ./internal/scheduling/... -count=1
fi
