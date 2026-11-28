#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir "$work/bin"
cat > "$work/bin/docker" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >> "$DOCKER_CALLS"
if [ "$1" = port ]; then echo '127.0.0.1:54321'; fi
EOF
cat > "$work/bin/go" <<'EOF'
#!/bin/sh
printf '%s | %s\n' "$POSTGRES_TEST_URL" "$*" >> "$GO_CALLS"
exit "${GO_EXIT:-0}"
EOF
chmod +x "$work/bin/docker" "$work/bin/go"
export PATH="$work/bin:$PATH" DOCKER_CALLS="$work/docker" GO_CALLS="$work/go"
cd "$root"

POSTGRES_PACKAGES= POSTGRES_TEST_URL=postgres://unsafe make -s test-postgres
printf '%s\n' \
	'postgres://postgres@127.0.0.1:54321/dynamis_code_test?sslmode=disable | test -p 1 ./internal/identity/... ./internal/files/... -count=1' \
	'postgres://postgres@127.0.0.1:54321/scheduling_test?sslmode=disable | test ./internal/scheduling/... -count=1' > "$work/expected"
cmp "$work/expected" "$GO_CALLS"
grep -q 'createdb -U postgres scheduling_test' "$DOCKER_CALLS"
grep -q '^rm -f ' "$DOCKER_CALLS"

: > "$GO_CALLS"
: > "$DOCKER_CALLS"
make -s test-postgres 'POSTGRES_PACKAGES=./internal/currency ./internal/catalog/... ./internal/transactions'
printf '%s\n' 'postgres://postgres@127.0.0.1:54321/dynamis_code_test?sslmode=disable | test -p 1 ./internal/currency ./internal/catalog/... ./internal/transactions -count=1' > "$work/expected"
cmp "$work/expected" "$GO_CALLS"
if grep -q createdb "$DOCKER_CALLS"; then echo 'unexpected scheduling database' >&2; exit 1; fi
grep -q '^rm -f ' "$DOCKER_CALLS"

for invalid in '   ' '-run=Test' './internal/../internal/catalog' './internal/missing' './internal/catalog;touch' './internal/catalog/.../x'; do
	: > "$DOCKER_CALLS"
	if POSTGRES_PACKAGES="$invalid" make -s test-postgres > "$work/output" 2>&1; then
		echo "accepted invalid packages: $invalid" >&2; exit 1
	fi
	if [ -s "$DOCKER_CALLS" ]; then echo 'started Docker for invalid input' >&2; exit 1; fi
done

: > "$DOCKER_CALLS"
if GO_EXIT=1 make -s test-postgres 'POSTGRES_PACKAGES=./internal/catalog' > "$work/output" 2>&1; then
	echo 'ignored Go test failure' >&2; exit 1
fi
grep -q '^rm -f ' "$DOCKER_CALLS"
echo 'Postgres routing checks passed'
