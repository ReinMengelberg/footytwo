# Wire contract (`proto/`)

The single source of truth for the messages that cross the WebSocket, since Go and
TypeScript can't share code. This is **prose / pseudo-types only** for Phase 0 — no
codegen, no transport yet. When the WS layer lands (later phase), both halves are
implemented to match these shapes.

## Ground rules

- **Server-authoritative.** The browser never sends positions. Clients send *inputs*
  up; the server runs the only real simulation and broadcasts *snapshots* down.
- **JSON for now.** Readable while we build; a binary format can come later without
  changing the conceptual shapes below.
- Units: meters for position, meters/second for velocity, radians for angles.
  World is X/Z on the ground plane, Y is up.

---

## ClientInput — client → server (up)

Sent every client frame (or on change). Describes *intent*, not outcome.

```
ClientInput {
  type:  "input"        // message discriminator
  seq:   number         // monotonic per-client input counter; the server echoes the
                        // last-applied seq back in snapshots so the client can later
                        // reconcile its local prediction
  t:     number         // client send time, ms (epoch or performance.now); for RTT/debug

  // Desired move direction in the XZ plane, each component in [-1, 1].
  // It's a *direction*, not a position — the server decides resulting motion.
  move:  { x: number, z: number }

  // Discrete/held actions for this input.
  kick:   boolean       // attempt to kick the ball this tick
  sprint: boolean       // hold to sprint

  // Facing the player wants (radians, around Y). Optional; server may derive from move.
  aim?:  number
}
```

Notes:
- `seq` is the backbone of client-side prediction/reconciliation (a later phase).
- Multiple inputs may arrive between server ticks; the server applies them in order.

---

## Snapshot — server → client (down)

Broadcast to every client in a room at (or near) the server tick rate. This is the
authoritative state. Clients render remote entities ~100ms in the past by
interpolating between snapshots; they predict only their *own* player from inputs.

```
Snapshot {
  type:  "snapshot"     // message discriminator
  tick:  number         // server simulation tick (fixed 60Hz)
  t:     number         // server time for this tick, ms

  ball: {
    x: number, y: number, z: number      // position
    vx: number, vy: number, vz: number   // velocity (lets clients extrapolate/smooth)
  }

  players: Array<{
    id:   string        // stable per-match player id
    x: number, y: number, z: number       // position
    vx: number, vy: number, vz: number    // velocity
    rot:  number        // facing, radians around Y
    ack:  number        // last ClientInput.seq the server applied for THIS player
                        // (only meaningful to the player who owns that id; used for
                        //  prediction reconciliation later)
  }>

  score?: { home: number, away: number }  // optional; appears once gameplay exists
}
```

Notes:
- Snapshots are full state for Phase-0 simplicity; delta-compression can come later.
- `ack` is per-player so each client knows how far the server has consumed *its* inputs.
- Including velocities (not just positions) makes client-side interpolation and
  dead-reckoning smoother between the ~16.6ms snapshots.

---

## Lifecycle (sketch, not built yet)

1. Client opens WS to a room (`roomID` keys one in-memory world on the server).
2. Server starts/forwards that room's 60Hz loop.
3. Client streams `ClientInput` up; server applies them to the authoritative world.
4. Server broadcasts `Snapshot` to all clients in the room each tick.
5. Client interpolates ball + remote players ~100ms behind; predicts own player live.
