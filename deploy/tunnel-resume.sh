#!/usr/bin/env bash
# Run by `make up` after the stack is up: if the Cloudflare tunnel was running
# before `make down` stopped it, starts it again, so a restart doesn't take
# the public address offline. The stopped container still holds the token.
# Refuses, like `make tunnel`, when another Docker host now serves the address.

compose="docker compose -f deploy/docker-compose.yml --profile tunnel"

stopped=$($compose ps -a -q --status exited cloudflared 2>/dev/null)
[ -n "$stopped" ] || exit 0

if ! "$(dirname "$0")/tunnel-guard.sh"; then
	echo "tunnel: not started."
	exit 1
fi
$compose start cloudflared
