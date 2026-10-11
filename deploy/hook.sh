#!/usr/bin/env bash
# Backpack session-start hook: prints "where are we" for this project.
#
# Copy it into any project. It reads the workspace and URL from the project's
# .backpack.json and the token from BACKPACK_TOKEN. It never fails a session:
# without .backpack.json it prints nothing, and on any other problem it says
# so in one line. It always exits 0.

config="${CLAUDE_PROJECT_DIR:-$PWD}/.backpack.json"
[ -f "$config" ] || exit 0

field() { sed -n "s/.*\"$1\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" "$config" | head -n 1; }
workspace=$(field workspace)
url=$(field url)

if [ -n "$BACKPACK_TOKEN" ] && [ -n "$workspace" ] && [ -n "$url" ] &&
	status=$(curl -fsS --max-time 3 \
		-H "Authorization: Bearer $BACKPACK_TOKEN" \
		-H "Accept: text/plain" \
		"${url%/}/v1/workspaces/$workspace/status" 2>/dev/null); then
	printf '%s\n' "$status"
else
	echo "Backpack unreachable — orientation skipped"
fi
exit 0
