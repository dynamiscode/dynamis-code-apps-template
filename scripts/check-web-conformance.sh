#!/bin/sh
set -eu

# Enforces web theming conformance:
# - app.css colours live only in custom property declarations (token blocks).
# - every template <html> root carries data-theme from the resolved theme.
status=0

css=internal/web/assets/app.css
bad_colours="$(grep -nE '#[0-9a-fA-F]{3,8}([^0-9a-fA-F]|$)' "$css" | grep -vE '^[0-9]+:[[:space:]]*--[A-Za-z0-9-]+:' || true)"
if [ -n "$bad_colours" ]; then
	echo "hard-coded colour outside a token declaration in $css; use a var(--token):" >&2
	echo "$bad_colours" >&2
	status=1
fi

for template in internal/web/templates/*.html; do
	missing="$(grep -n '<html' "$template" | grep -v 'data-theme="{{.Theme}}"' || true)"
	if [ -n "$missing" ]; then
		echo "<html> root without data-theme=\"{{.Theme}}\" in $template:" >&2
		echo "$missing" >&2
		status=1
	fi
done

exit "$status"
