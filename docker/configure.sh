#!/usr/bin/env bash
# ===========================================================================
# configure.sh - write this checkout's .env for compose.yaml
#
# Compose cannot work out absolute host paths, the host account or what the
# host supports, so this records them once per checkout (a worktree gets its
# own .env):
#
#   SRO_DOCKER_UID, SRO_DOCKER_GID   the container user, so files written
#                                    into the checkout stay yours
#   SRO_DOCKER_CHECKOUT              this checkout
#   SRO_DOCKER_MAIN_CHECKOUT         the main checkout a worktree shares
#                                    .generated with (itself otherwise)
#   SRO_DOCKER_GAME_ROOT             the licensed client folder: SRO_GAME_ROOT
#                                    when set, else the one .env already
#                                    names, else beside the main checkout,
#                                    the rule scripts/build/world/paths.mjs uses
#   SRO_DOCKER_TARGET                the image target (docker/Dockerfile):
#                                    SRO_DOCKER_TARGET when set, else assets
#                                    on amd64 and stack elsewhere
#   SRO_DOCKER_RENDER_GID,           with an Intel or AMD GPU: the groups
#   SRO_DOCKER_VIDEO_GID             owning its device nodes
#   COMPOSE_FILE                     compose.yaml, plus
#                                    docker/compose.casefold.yaml on Linux
#                                    6.13+ and docker/compose.gpu.yaml with
#                                    that GPU, for the assets target
#   COMPOSE_PROJECT_NAME             in a linked worktree only: its own
#                                    project, so its container and volumes
#                                    do not replace the main checkout's
#
# Other lines in .env are kept, SRO_DOCKER_EXTRACTED_SIZE among them.
# .env is git-ignored.
# ===========================================================================
set -euo pipefail

say() { echo "configure.sh: $*" >&2; }

checkout=$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)
common_dir=$(git -C "$checkout" rev-parse --path-format=absolute --git-common-dir)
main_checkout=$(dirname "$common_dir")
env_file="$checkout/.env"

#
# env_value KEY
#
# The value .env holds for KEY, without the quotes this script writes.
#
env_value() {
	[[ -f "$env_file" ]] || return 0
	local value
	value=$(grep -E "^$1=" "$env_file" | tail -n 1 | cut -d= -f2-) || return 0
	value=${value#\'}
	echo "${value%\'}"
}

# --- The Docker daemon ------------------------------------------------------

arch=$(uname -m)
if docker_info=$(docker info --format '{{.OperatingSystem}}|{{.Architecture}}|{{json .SecurityOptions}}' 2>/dev/null); then
	IFS='|' read -r docker_os docker_arch docker_security <<<"$docker_info"
	arch=${docker_arch:-$arch}
	if [[ "$docker_security" == *name=rootless* ]]; then
		say "this Docker daemon is rootless, which the stack cannot use:"
		say "its host network is not the host's, so 127.0.0.1:5180 is unreachable from"
		say "your browser, and files the container writes would not be yours."
		say "Use the system (rootful) Docker Engine; see docker/README.md."
		exit 1
	fi
	if [[ "$docker_os" == *"Docker Desktop"* ]]; then
		say "warning: Docker Desktop runs containers in a VM, which this stack does not support:"
		say "warning: host networking, host pids and /dev/dri all name the VM, not this machine."
		say "warning: use Docker Engine; see docker/README.md."
	fi
else
	say "warning: cannot reach the Docker daemon; skipped its checks"
fi

# --- The client folder ------------------------------------------------------

if [[ -n "${SRO_GAME_ROOT:-}" ]]; then
	game_root=$SRO_GAME_ROOT
else
	game_root=$(env_value SRO_DOCKER_GAME_ROOT)
	game_root=${game_root:-$(dirname "$main_checkout")}
fi
if [[ ! -d "$game_root" ]]; then
	say "the client folder $game_root does not exist"
	say "rerun with SRO_GAME_ROOT=<client folder>"
	exit 1
fi
game_root=$(cd "$game_root" && pwd)

# The bind mount needs extracted/; created here, it is yours rather than a
# root-owned folder Docker makes.
mkdir -p "$game_root/extracted"

if [[ ! -f "$game_root/SRO_Client.exe" ]]; then
	say "no SRO_Client.exe in $game_root"
	say "put the v1.150 client there, or rerun with SRO_GAME_ROOT=<client folder>"
	say "(the stack itself runs without it once game data is built)"
fi

# --- The image target -------------------------------------------------------

target=${SRO_DOCKER_TARGET:-}
if [[ -z "$target" ]]; then
	case $arch in
	x86_64 | amd64) target=assets ;;
	*)
		target=stack
		say "no Google Chrome for $arch: the image runs the stack but cannot build game data;"
		say "build it on an amd64 Linux machine (see docker/README.md)"
		;;
	esac
fi
if [[ "$target" != stack && "$target" != assets ]]; then
	say "SRO_DOCKER_TARGET must be stack or assets, not $target"
	exit 1
fi

compose_files=("$checkout/compose.yaml")
render_gid=""
video_gid=""
if [[ "$target" == assets ]]; then
	# The asset build opens client files case-insensitively
	# (docker/compose.casefold.yaml).
	if [[ -e /sys/fs/tmpfs/features/casefold ]]; then
		compose_files+=("$checkout/docker/compose.casefold.yaml")
	else
		say "this kernel ($(uname -r)) has no case-insensitive tmpfs, which needs Linux 6.13+;"
		say "the stack runs, but \`assets\` needs $game_root/extracted on a casefold"
		say "ext4 or f2fs filesystem instead (see docker/README.md)"
	fi

	# The asset build's portraits need a GPU the image's Mesa drivers
	# cover (docker/compose.gpu.yaml).
	other_drivers=()
	shopt -s nullglob
	for node in /sys/class/drm/renderD*; do
		driver=$(readlink -f "$node/device/driver" 2>/dev/null) || driver=unknown
		driver=${driver##*/}
		case $driver in
		i915 | xe | amdgpu)
			if [[ -z "$render_gid" ]]; then
				render_gid=$(stat -c %g "/dev/dri/${node##*/}")
				cards=("$node"/device/drm/card*)
				if ((${#cards[@]} > 0)); then
					video_gid=$(stat -c %g "/dev/dri/${cards[0]##*/}")
				else
					video_gid=$render_gid
				fi
			fi
			;;
		*) other_drivers+=("$driver") ;;
		esac
	done
	shopt -u nullglob
	if [[ -n "$render_gid" ]]; then
		compose_files+=("$checkout/docker/compose.gpu.yaml")
	elif ((${#other_drivers[@]} > 0)); then
		say "found only GPUs on ${other_drivers[*]}; the image's Mesa drivers cover Intel"
		say "(i915, xe) and AMD (amdgpu), so the asset build's portrait step will fail"
	else
		say "no GPU (/dev/dri); the asset build's portrait step will fail without one"
	fi
fi

project=""
if [[ "$checkout" != "$main_checkout" ]]; then
	project=$(basename "$checkout")
	project=${project,,}
	project="opensro-${project//[^a-z0-9_-]/-}"
fi

# --- .env -------------------------------------------------------------------

keys='^(SRO_DOCKER_(UID|GID|CHECKOUT|MAIN_CHECKOUT|GAME_ROOT|TARGET|RENDER_GID|VIDEO_GID)|COMPOSE_FILE|COMPOSE_PROJECT_NAME)='
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
	echo "SRO_DOCKER_TARGET=$target"
	if [[ -n "$render_gid" ]]; then
		echo "SRO_DOCKER_RENDER_GID=$render_gid"
		echo "SRO_DOCKER_VIDEO_GID=$video_gid"
	fi
	echo "COMPOSE_FILE='$(IFS=:; echo "${compose_files[*]}")'"
	if [[ -n "$project" ]]; then
		echo "COMPOSE_PROJECT_NAME=$project"
	fi
} >"$env_file.tmp"
mv "$env_file.tmp" "$env_file"

echo "configure.sh: wrote $env_file"
grep -E "$keys" "$env_file"
