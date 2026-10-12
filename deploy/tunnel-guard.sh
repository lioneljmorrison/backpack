#!/usr/bin/env bash
# Run by `make tunnel` before it starts anything: refuses to put a second
# Backpack on the Cloudflare tunnel. If the project's public address in
# .backpack.json already answers, and the tunnel isn't running on the Docker
# host being started, the board is served from another host; two copies on
# one tunnel would split requests, and changes, between two boards.

config=.backpack.json
[ -f "$config" ] || exit 0
url=$(sed -n 's/.*"url"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$config" | head -n 1)
[ -n "$url" ] || exit 0

running=$(docker compose -f deploy/docker-compose.yml --profile tunnel ps -q --status running cloudflared 2>/dev/null)
[ -z "$running" ] || exit 0

if curl -fsS --max-time 5 -o /dev/null "${url%/}/healthz" 2>/dev/null; then
	echo "tunnel: Backpack is already served at $url from another Docker host."
	echo "        Stop it there first, or set BACKPACK_DOCKER_CONTEXT to that host."
	exit 1
fi
exit 0
