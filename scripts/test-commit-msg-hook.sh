#!/bin/sh
# Offline fixture tests for .githooks/commit-msg.
set -eu

hook="$(cd "$(dirname "$0")/.." && pwd)/.githooks/commit-msg"
root="$(mktemp -d)"
trap 'rm -rf "$root"' EXIT

printf 'feat: add thing\n\nCo-Authored-By: Claude Haiku 5.5 <noreply@anthropic.com>\n' >"$root/coauthor.txt"
printf 'fix: tweak\n\n🤖 Generated with [Claude Code](https://claude.com/claude-code)\n' >"$root/generated.txt"
printf 'fix: tweak\n\nGenerated with Claude Code\n' >"$root/generated-plain.txt"
printf 'feat: add thing\n\nCo-authored-by: claude <noreply@example.com>\n' >"$root/lowercase.txt"
printf 'feat: add thing\n\nCo-Authored-By: Jane Doe <jane@example.com>\n' >"$root/human.txt"
printf 'feat: add thing\n\nBody without trailers.\n' >"$root/good.txt"

for bad in coauthor generated generated-plain lowercase; do
	if "$hook" "$root/$bad.txt" 2>/dev/null; then
		echo "expected commit-msg hook to reject $bad.txt" >&2
		exit 1
	fi
done

for good in human good; do
	"$hook" "$root/$good.txt"
done

echo "test-commit-msg-hook fixtures passed"
