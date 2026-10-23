#!/usr/bin/env bash
# ===========================================================================
# entrypoint.sh - the commands of the OpenSRO development container
#
#   stack   the docs/GETTING_STARTED.md server cluster and browser client in
#           one container: the pinned Nomad dev agent, provisioning,
#           deploy -build, then the client dev server on 127.0.0.1:5180.
#   assets  the game-data build from the licensed client mounted at /game.
#   setup   pnpm install only.
#
# Anything else runs as a command: a shell, pnpm check source, go test.
#
# Nomad remains the process owner. Stopping the stack drains the fleet with
# sro-nomad stop before Nomad exits, the order GETTING_STARTED.md section 5
# requires so a GameWorld's database shutdown is never cut off.
# ===========================================================================
set -euo pipefail

# compose.yaml starts every command in the checkout, at its host path.
CHECKOUT=$PWD
SERVER="$CHECKOUT/apps/server"
# The fleet commands, per checkout, in the server's ignored temp/ folder.
BIN="$SERVER/temp/docker"
NOMAD_NODES=http://127.0.0.1:4646/v1/nodes
NOMAD_READY_SECONDS=120
NOMAD_EXIT_SECONDS=60

nomad_pid=""
client_pid=""

#
# install_packages
#
# Idempotent: a current node_modules makes this a no-op.
#
install_packages() {
	pnpm -C "$CHECKOUT" install --frozen-lockfile
}

#
# build_operations
#
# Rebuilt at every start (incremental through the GOCACHE volume) and run
# from the module root, where they find go.mod.
#
build_operations() {
	mkdir -p "$BIN"
	local command
	for command in sro-nomad sro-bootstrap-development; do
		go -C "$SERVER" build -o "$BIN/$command" "./cmd/operations/$command"
	done
}

#
# wait_for_nomad
#
# deploy needs a ready client node, not just a listening API.
#
wait_for_nomad() {
	local deadline=$((SECONDS + NOMAD_READY_SECONDS))
	until curl -fsS "$NOMAD_NODES" 2>/dev/null | grep -q '"Status":"ready"'; do
		if ! kill -0 "$nomad_pid" 2>/dev/null; then
			echo "sro-docker: the Nomad dev agent exited during startup" >&2
			return 1
		fi
		if ((SECONDS >= deadline)); then
			echo "sro-docker: no ready Nomad node after ${NOMAD_READY_SECONDS}s" >&2
			return 1
		fi
		sleep 1
	done
}

#
# stop_stack
#
# The client first, then the fleet through sro-nomad stop, then Nomad.
# Runs on every exit of the stack, including a failed deploy.
#
stop_stack() {
	local status=$?
	trap - EXIT TERM INT
	if [[ -n "$client_pid" ]]; then
		# pnpm runs vite in process groups of its own; the session holds them all.
		pkill -TERM -s "$client_pid" 2>/dev/null || true
	fi
	if [[ -n "$nomad_pid" ]]; then
		echo "sro-docker: draining the fleet before Nomad stops"
		(cd "$SERVER" && "$BIN/sro-nomad" stop) || true
		# dev-agent and its Nomad child share one process group (setsid).
		kill -INT -- "-$nomad_pid" 2>/dev/null || true
		local deadline=$((SECONDS + NOMAD_EXIT_SECONDS))
		while kill -0 -- "-$nomad_pid" 2>/dev/null && ((SECONDS < deadline)); do
			sleep 1
		done
	fi
	exit "$status"
}

#
# run_stack
#
run_stack() {
	install_packages
	build_operations
	cd "$SERVER"

	trap stop_stack EXIT
	trap 'exit 143' TERM
	trap 'exit 130' INT

	# setsid puts dev-agent and the Nomad it runs in the foreground into their
	# own process group, so stop_stack can signal both.
	setsid "$BIN/sro-nomad" dev-agent &
	nomad_pid=$!
	wait_for_nomad

	"$BIN/sro-bootstrap-development"
	# A stop request waits for deploy to return: deploy holds the fleet lock
	# that sro-nomad stop needs.
	if ! "$BIN/sro-nomad" deploy -build; then
		echo "sro-docker: deploy failed. Missing game data? Build it with:" >&2
		echo "sro-docker:   docker compose run --rm sro assets" >&2
		return 1
	fi
	"$BIN/sro-nomad" status

	cd "$CHECKOUT"
	setsid pnpm dev &
	client_pid=$!
	echo "sro-docker: open http://127.0.0.1:5180/ and log in as tester / 123123"

	# Either side exiting ends the stack; stop_stack drains what is left.
	wait -n "$nomad_pid" "$client_pid"
}

#
# casefold_extracted
#
# compose.yaml mounts extracted/ on a casefold tmpfs. Lookups ignore case
# only in folders flagged +F, which must be set while the folder is empty;
# every folder prepare then creates inherits it.
#
casefold_extracted() {
	local extracted="$SRO_GAME_ROOT/extracted"
	if [[ "$(lsattr -d "$extracted" | cut -d' ' -f1)" == *F* ]]; then
		return
	fi
	if ! chattr +F "$extracted"; then
		echo "sro-docker: cannot make $extracted case-insensitive." >&2
		echo "sro-docker: it needs Linux 6.13+ and the empty compose volume (docker volume rm opensro_extracted)" >&2
		return 1
	fi
}

#
# run_assets
#
# Section 2 of GETTING_STARTED.md, in its order. extracted/ is empty at the
# start of each run (compose.yaml), so prepare extracts the client again.
#
run_assets() {
	install_packages
	casefold_extracted
	cd "$CHECKOUT"
	pnpm assets prepare
	pnpm assets doctor
	pnpm assets build full
	pnpm assets ledger
	pnpm task build server-game-data
	pnpm assets check integrity
}

case "${1:-stack}" in
stack) run_stack ;;
assets) run_assets ;;
setup) install_packages ;;
*) exec "$@" ;;
esac
