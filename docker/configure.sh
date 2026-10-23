#!/usr/bin/env bash
# ===========================================================================
# configure.sh - write this checkout's .env for compose.yaml
#
# Compose cannot work out absolute host paths or the host account, so this
# records them once per checkout (a worktree gets its own .env):
#
#   SRO_DOCKER_UID, SRO_DOCKER_GID   the container user, so files written
#                                    into the checkout stay yours
#   SRO_DOCKER_CHECKOUT              this checkout
#   SRO_DOCKER_MAIN_CHECKOUT         the main checkout a worktree shares
#                                    .generated with (itself otherwise)
#   SRO_DOCKER_GAME_ROOT             the licensed client folder: SRO_GAME_ROOT
#                                    when set, else beside the main checkout,
#                                    the rule scripts/build/world/paths.mjs uses
#   SRO_DOCKER_RENDER_GID,           with a host GPU (/dev/dri): the groups
#   SRO_DOCKER_VIDEO_GID,            owning its device nodes, and COMPOSE_FILE
#   COMPOSE_FILE                     adding docker/compose.gpu.yaml
#
# Other lines in .env are kept. .env is git-ignored.
# ===========================================================================
set -euo pipefail

checkout=$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)
common_dir=$(git -C "$checkout" rev-parse --path-format=absolute --git-common-dir)
main_checkout=$(dirname "$common_dir")
game_root=${SRO_GAME_ROOT:-$(dirname "$main_checkout")}
game_root=$(cd "$game_root" && pwd)

# compose.yaml mounts a volume on extracted/; created here, it is yours
# rather than a root-owned folder Docker makes.
mkdir -p "$game_root/extracted"

if [[ ! -f "$game_root/SRO_Client.exe" ]]; then
	echo "configure.sh: no SRO_Client.exe in $game_root" >&2
	echo "configure.sh: put the v1.150 client there, or rerun with SRO_GAME_ROOT=<client folder>" >&2
	echo "configure.sh: (the stack itself runs without it once game data is built)" >&2
fi

# The asset build's portraits need a GPU (docker/compose.gpu.yaml).
shopt -s nullglob
render_nodes=(/dev/dri/renderD*)
card_nodes=(/dev/dri/card*)
shopt -u nullglob
if ((${#render_nodes[@]} == 0)); then
	echo "configure.sh: no GPU (/dev/dri); the asset build's portrait step will fail without one" >&2
fi

env_file="$checkout/.env"
keys='^(SRO_DOCKER_(UID|GID|CHECKOUT|MAIN_CHECKOUT|GAME_ROOT|RENDER_GID|VIDEO_GID)|COMPOSE_FILE)='
{
	if [[ -f "$env_file" ]]; then
		grep -Ev "$keys" "$env_file" || true
	fi
	# Single quotes keep Compose from interpolating anything in a path.
	echo "SRO_DOCKER_UID=$(id -u)"
	echo "SRO_DOCKER_GID=$(id -g)"
	echo "SRO_DOCKER_CHECKOUT='$checkout'"
	echo "SRO_DOCKER_MAIN_CHECKOUT='$main_checkout'"
	echo "SRO_DOCKER_GAME_ROOT='$game_root'"
	if ((${#render_nodes[@]} > 0)); then
		echo "SRO_DOCKER_RENDER_GID=$(stat -c %g "${render_nodes[0]}")"
		echo "SRO_DOCKER_VIDEO_GID=$(stat -c %g "${card_nodes[0]:-${render_nodes[0]}}")"
		echo "COMPOSE_FILE='$checkout/compose.yaml:$checkout/docker/compose.gpu.yaml'"
	fi
} >"$env_file.tmp"
mv "$env_file.tmp" "$env_file"

echo "configure.sh: wrote $env_file"
grep -E "$keys" "$env_file"
