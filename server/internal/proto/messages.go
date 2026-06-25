// Package proto holds the JSON wire contract shared (by hand) between the Go
// server and the TS client. It is the Go half of proto/messages.md; the TS half
// lives in web/src/net/protocol.ts and must be kept in sync manually.
//
// It is a leaf package of pure DTOs — it imports nothing of ours, so both the
// game layer (which builds Snapshots) and the transport layer (which decodes
// ClientInput) can depend on it without cycles.
package proto

// ClientInput is sent client -> server. Positions are never sent up; only intent.
type ClientInput struct {
	Seq    int     `json:"seq"`    // incrementing counter (reserved for reconciliation)
	MoveX  float64 `json:"moveX"`  // -1..1 desired direction on X
	MoveZ  float64 `json:"moveZ"`  // -1..1 desired direction on Z
	Sprint bool    `json:"sprint"` // hold to sprint
	Charge bool    `json:"charge"` // hold Space to build kick power; release commits
	Lift   int     `json:"lift"`   // while charging: +1 loft (up) / -1 driven (down) / 0
	Spin   int     `json:"spin"`   // while charging: -1 left / +1 right / 0 (curve)
}

// Snapshot is sent server -> client each tick: the authoritative world state.
type Snapshot struct {
	Tick    int           `json:"tick"`
	Owner   string        `json:"owner"` // player ID with possession, "" if free
	Ball    BallState     `json:"ball"`
	Players []PlayerState `json:"players"`
	Touch   int           `json:"touch"` // monotonic dribble-touch count; clients diff it to animate touches
	Kick    int           `json:"kick"`  // monotonic kick count; clients diff it to animate/sfx a kick

	// Out-of-bounds restarts and the score. Restart is a monotonic counter clients
	// diff to detect a fresh out-of-bounds event; RestartKind names the most recent
	// one ("throwin" | "goalkick" | "corner" | "goal" | "" before any).
	Restart     int    `json:"restart"`
	RestartKind string `json:"restartKind"`
	ScoreHome   int    `json:"scoreHome"` // goals for the Home (+X) side
	ScoreAway   int    `json:"scoreAway"` // goals for the Away (-X) side
}

// BallState is the ball's world position. Y is height (a lofted ball arcs above
// the ground); Spin is the current side-spin, surfaced for an optional visual cue.
type BallState struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Z    float64 `json:"z"`
	Spin float64 `json:"spin"`
}

// PlayerState is one player's render state in a snapshot.
type PlayerState struct {
	ID          string  `json:"id"`
	X           float64 `json:"x"`
	Z           float64 `json:"z"`
	FacingX     float64 `json:"facingX"`
	FacingZ     float64 `json:"facingZ"`
	Possessing  bool    `json:"possessing"`  // owns the ball (sticky)
	AtFeet      bool    `json:"atFeet"`      // ball at this player's feet this tick (pulses on contact)
	ChargePower float64 `json:"chargePower"` // current kick wind-up, 0..1 (0 = not charging)
	ChargeSpin  float64 `json:"chargeSpin"`  // dialled-in curve, -1..1 (sign = direction)
	ChargeLift  float64 `json:"chargeLift"`  // dialled-in loft, -1..1 (+loft / -driven)
}
