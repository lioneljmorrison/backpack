#!/usr/bin/env bash
# Run before every `make` command that execs into the backpack container:
# fails with one short message, naming the Docker it looked at, when the
# backpack service isn't running there.

if [ -n "$(docker compose -f deploy/docker-compose.yml ps --status running -q backpack 2>/dev/null)" ]; then
	exit 0
fi
echo "Backpack is not running on the Docker context '${DOCKER_CONTEXT:-default}'."
echo "  Start it there with: make up"
echo "  Or, if it runs elsewhere, put BACKPACK_DOCKER_CONTEXT ?= <context> in local.mk"
echo "  (docker context ls lists the contexts)."
exit 1
