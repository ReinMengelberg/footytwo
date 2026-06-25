package physics

import "math"

// Player is a ground-constrained body. Movement updates X/Z only; Y stays
// pinned. Orientation is a Facing vector in the X/Z plane (Y=0), not a scalar
// angle and not a quaternion: a player only rotates about Y, so a normalized
// ground-plane vector fully captures orientation. Facing is kept independent of
// Vel so later phases can let facing and movement diverge (shielding the ball,
// backpedaling, receiving on the half-turn).
type Player struct {
	ID     string
	Name   string
	Stats  Stats
	Pos    Vec3 // world position
	Vel    Vec3 // current velocity (units/sec), X/Z plane
	Facing Vec3 // unit vector in the X/Z plane (Y=0); the direction the player faces
}

// Yaw derives the heading angle from Facing on demand. The engine works in
// vectors; this exists only for things that need an angle (e.g. the client
// rendering rotation). Convention: 0 = +Z, increasing toward +X.
func (p Player) Yaw() float64 {
	return math.Atan2(p.Facing.X, p.Facing.Z)
}
