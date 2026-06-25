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
}

// Snapshot is sent server -> client each tick: the authoritative world state.
type Snapshot struct {
	Tick    int           `json:"tick"`
	Owner   string        `json:"owner"` // player ID with possession, "" if free
	Ball    BallState     `json:"ball"`
	Players []PlayerState `json:"players"`
	Touch   int           `json:"touch"` // monotonic dribble-touch count; clients diff it to animate touches
}

// BallState is the ball's world position.
type BallState struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// PlayerState is one player's render state in a snapshot.
type PlayerState struct {
	ID         string  `json:"id"`
	X          float64 `json:"x"`
	Z          float64 `json:"z"`
	FacingX    float64 `json:"facingX"`
	FacingZ    float64 `json:"facingZ"`
	Possessing bool    `json:"possessing"` // owns the ball (sticky)
	AtFeet     bool    `json:"atFeet"`     // ball at this player's feet this tick (pulses on contact)
}
