# footy — dev orchestration.
#
# Both halves are long-lived foreground processes. `make dev` runs them in one
# process group so a single Ctrl-C (the `trap` -> `kill 0`) tears both down.
# Run them individually with `make server` / `make web` when you only want one.
#
# First time only: `make setup` (installs web deps; Go modules resolve on build).

.PHONY: dev server web setup

dev:
	@trap 'kill 0' EXIT; \
	(cd server && go run ./cmd/server) & \
	(cd web && bun run dev) & \
	wait

server:
	cd server && go run ./cmd/server

web:
	cd web && bun run dev

setup:
	cd web && bun install
