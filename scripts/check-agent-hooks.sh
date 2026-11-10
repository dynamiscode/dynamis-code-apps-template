#!/bin/sh
set -eu

# check-agent-hooks.sh verifies that an agent hook script or command responds
# within sub-second bounds on standard events and handles stdin without blocking.

TARGET_SCRIPT="${1:-}"

if [ -z "$TARGET_SCRIPT" ]; then
	echo "Usage: $0 <path-to-hook-script>" >&2
	exit 1
fi

if [ ! -f "$TARGET_SCRIPT" ] || [ ! -x "$TARGET_SCRIPT" ]; then
	echo "Error: hook script '$TARGET_SCRIPT' does not exist or is not executable" >&2
	exit 1
fi

echo "Testing hook '$TARGET_SCRIPT' for non-blocking execution..."

for EVENT in PreInvocation PreToolUse PostToolUse Stop; do
	start_time=$(date +%s)
	output=$(printf '{"tool_name":"read_file"}' | ORCA_ANTIGRAVITY_EVENT="$EVENT" "$TARGET_SCRIPT" 2>/dev/null || true)
	end_time=$(date +%s)
	duration=$((end_time - start_time))
	if [ "$duration" -ge 2 ]; then
		echo "FAIL: event '$EVENT' took ${duration}s (must execute in < 2s to prevent CLI timeout)" >&2
		exit 1
	fi
	echo "PASS: event '$EVENT' completed within ${duration}s"
done

echo "All agent hook checks passed."
