#!/bin/sh
set -eu

repo=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
root=$(mktemp -d)
trap 'rm -rf "$root"' EXIT
mkdir -p "$root/bin" "$root/internal/web"
printf 'original\n' >"$root/internal/web/example_templ.go"
cat >"$root/bin/go" <<'EOF'
#!/bin/sh
case "$SCENARIO" in
  unchanged) ;;
  failure) exit 1 ;;
  changed) printf 'changed\n' > internal/web/example_templ.go ;;
  added) printf 'new\n' > internal/web/new_templ.go ;;
  deleted) rm internal/web/example_templ.go ;;
esac
EOF
chmod +x "$root/bin/go"

for scenario in unchanged failure changed added deleted; do
  printf 'original\n' >"$root/internal/web/example_templ.go"
  rm -f "$root/internal/web/new_templ.go"
  status=0
  (cd "$root" && PATH="$root/bin:$PATH" SCENARIO="$scenario" make -s -f "$repo/Makefile" templ-check >"$root/output" 2>&1) || status=$?
  if { [ "$scenario" = unchanged ] && [ "$status" -eq 0 ]; } ||
     { [ "$scenario" != unchanged ] && [ "$status" -ne 0 ]; }; then
    printf 'ok - %s\n' "$scenario"
  else
    printf 'not ok - %s (exit %s)\n' "$scenario" "$status" >&2
    cat "$root/output" >&2
    exit 1
  fi
done
