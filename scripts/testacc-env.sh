#!/usr/bin/env bash
# Start or stop the local acceptance-test environment: Postgres in Docker and
# a Ramble server built from a source checkout, plus a seeded test user and
# write-scope API token.
#
#   scripts/testacc-env.sh up     start everything, write .testacc.env
#   scripts/testacc-env.sh down   stop and remove everything
#
# Environment:
#   RAMBLE_SRC    Ramble checkout (default ../ramble)
#   RAMBLE_PORT   port Ramble listens on (default 3999)
#   PG_PORT       host port for Postgres, bound to 127.0.0.1 (default 55432)
#
# The token is written only to .testacc.env (mode 0600), never to the terminal.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RAMBLE_SRC="${RAMBLE_SRC:-$root/../ramble}"
RAMBLE_PORT="${RAMBLE_PORT:-3999}"
PG_PORT="${PG_PORT:-55432}"
PG_IMAGE="postgres:17-alpine"
PG_NAME="ramble-tfacc-pg"
# A throwaway password for a container bound to loopback.
PG_PASSWORD="tfacc"
state="$root/.testacc"
envfile="$root/.testacc.env"

up() {
	[ -d "$RAMBLE_SRC/cmd/ramble" ] || { echo "RAMBLE_SRC=$RAMBLE_SRC is not a Ramble checkout" >&2; exit 1; }
	RAMBLE_SRC="$(cd "$RAMBLE_SRC" && pwd)"
	mkdir -p "$state"

	if [ -f "$state/ramble.pid" ] || docker inspect "$PG_NAME" >/dev/null 2>&1; then
		echo "environment already exists; run '$0 down' first" >&2
		exit 1
	fi
	# From here on a failed start leaves nothing behind for the next "up".
	trap 'rc=$?; if [ "$rc" -ne 0 ]; then echo "start-up failed; tearing down" >&2; down; fi' EXIT

	docker run -d --name "$PG_NAME" -p "127.0.0.1:$PG_PORT:5432" \
		-e POSTGRES_PASSWORD="$PG_PASSWORD" -e POSTGRES_DB=rmbl "$PG_IMAGE" >/dev/null
	for _ in $(seq 1 60); do
		docker exec "$PG_NAME" pg_isready -U postgres -d rmbl >/dev/null 2>&1 && break
		sleep 1
	done

	export DATABASE_URL="postgres://postgres:$PG_PASSWORD@127.0.0.1:$PG_PORT/rmbl?sslmode=disable"
	export RAMBLE_ADDR="http://127.0.0.1:$RAMBLE_PORT"

	(cd "$RAMBLE_SRC" && go build -o "$state/ramble" ./cmd/ramble)
	# Ramble loads ./views and ./public relative to its working directory.
	(
		cd "$RAMBLE_SRC"
		SESSION_SECRET="tfacc-session-secret-not-for-production" BASE_URL="$RAMBLE_ADDR" \
			nohup "$state/ramble" server --port "$RAMBLE_PORT" >"$state/ramble.log" 2>&1 &
		echo $! >"$state/ramble.pid"
	)

	# Ramble migrates the schema on start; /v1/packs is public and answers
	# only once the server is serving.
	ready=""
	for _ in $(seq 1 90); do
		if curl -fsS -o /dev/null "$RAMBLE_ADDR/v1/packs" 2>/dev/null; then ready=1; break; fi
		kill -0 "$(cat "$state/ramble.pid")" 2>/dev/null || break
		sleep 1
	done
	if [ -z "$ready" ]; then
		echo "Ramble did not become ready; last log lines:" >&2
		tail -n 30 "$state/ramble.log" >&2 || true
		exit 1
	fi

	(cd "$root" && go run ./scripts/seed -out "$envfile")
	trap - EXIT
	echo "Ramble is up at $RAMBLE_ADDR. Run: . ./.testacc.env && make testacc"
}

down() {
	if [ -f "$state/ramble.pid" ]; then
		kill "$(cat "$state/ramble.pid")" 2>/dev/null || true
	fi
	docker rm -f "$PG_NAME" >/dev/null 2>&1 || true
	rm -f "$state/ramble.pid" "$state/ramble.log" "$state/ramble" "$envfile"
	rmdir "$state" 2>/dev/null || true
}

case "${1:-}" in
up) up ;;
down) down ;;
*) echo "usage: $0 up|down" >&2; exit 2 ;;
esac
