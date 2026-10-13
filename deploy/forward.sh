#!/usr/bin/env bash
# `make forward PORTS…`: when the stack runs on a remote Docker host reached
# over SSH, forwards its ports to the same ports on this machine's 127.0.0.1,
# so the tests and a browser reach it as if it were local. The stack's ports
# stay bound to the remote host's 127.0.0.1; only SSH carries them.

if [ -z "$DOCKER_CONTEXT" ]; then
	echo "forward: the stack is local (BACKPACK_DOCKER_CONTEXT is unset); nothing to forward."
	exit 0
fi

endpoint=$(docker context inspect "$DOCKER_CONTEXT" --format '{{.Endpoints.docker.Host}}')
case "$endpoint" in
ssh://*) host=${endpoint#ssh://} ;;
*)
	echo "forward: Docker context $DOCKER_CONTEXT is not reached over SSH ($endpoint); open its ports another way."
	exit 1
	;;
esac

open() { (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; }

taken=()
for p in "$@"; do open "$p" && taken+=("$p"); done
if [ ${#taken[@]} -eq $# ]; then
	echo "forward: ports $* are already open."
	exit 0
elif [ ${#taken[@]} -gt 0 ]; then
	echo "forward: port(s) ${taken[*]} are already in use here; free them and try again."
	exit 1
fi

args=(-f -N -o ExitOnForwardFailure=yes)
for p in "$@"; do args+=(-L "127.0.0.1:$p:127.0.0.1:$p"); done
ssh "${args[@]}" "$host" && echo "forward: ports $* now reach $host."
