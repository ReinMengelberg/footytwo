# footy

Browser-based multiplayer football, **server-authoritative**. A Go server runs the
real simulation in memory; the browser is a thin Three.js client that renders and
(later) predicts. This repo is currently at **Phase 0**: a clean-booting skeleton.
No physics, networking, or gameplay yet.

## Layout

```
.
├── server/   # Go, stdlib only. Owns all physics; one in-memory world per match @ 60Hz.
├── web/      # Vite + Vue 3 + TS SPA. Raw Three.js (three@0.184.0). No physics engine.
└── proto/    # Shared wire contract (ClientInput up, Snapshot down). The one source
              # of truth both sides read, since Go and TS can't share code.
```

## Run (two independent halves, no wiring between them yet)

Both at once (single Ctrl-C stops both):

```sh
make setup   # first time only — installs web deps
make dev
```

Or each half on its own — `make server` / `make web`, equivalent to:

Server — boots an HTTP server on `:8080` with a `GET /health` probe:

```sh
cd server
go run ./cmd/server
# logs: listening on :8080
# curl -i localhost:8080/health  -> 200 {"status":"ok"}
```

Web — full-window Three.js canvas showing a green pitch (uses [bun](https://bun.sh)):

```sh
cd web
bun install
bun run dev
# open the printed localhost URL
```

## Where this is heading (later phases)

- Transport: WebSocket. Clients send **inputs** up; server broadcasts authoritative
  **snapshots** down. Clients never send positions. JSON for now. See `proto/messages.md`.
- Physics: hand-rolled in Go (sphere/plane/capsule). No physics engine, no Rapier.
- Client predicts only its own local player and interpolates everything else
  (ball + remote players) from snapshots ~100ms in the past.

No auth, no database — pure localhost.
