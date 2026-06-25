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
	Team   Team // which goal this player attacks; decides corner vs goal kick (see restart.go)
	Stats  Stats
	Pos    Vec3 // world position
	Vel    Vec3 // current velocity (units/sec), X/Z plane
	Facing Vec3 // unit vector in the X/Z plane (Y=0); the direction the player faces

	// Charge accumulators for the kick. The player can charge ANY time (with or
	// without the ball): while they hold Charge these integrate hold-durations, and
	// the release edge commits the kick below. Unexported sim state.
	chargePower float64 // seconds of Charge held this wind-up, capped at maxChargeTime
	loftAccum   float64 // signed seconds of Lift held (+loft / -driven), capped
	spinAccum   float64 // signed seconds of Spin held (curve), capped
	wasCharging bool    // Charge state last tick, for release-edge detection

	// kick is the committed charged kick (Kind ActionKick) awaiting the player's
	// next touch of the ball — fired either by reaching a loose ball (a first-time
	// strike, in tryGainPossession) or on the next dribble contact (in stepDribble).
	// Kind is ActionNone when nothing is committed.
	kick PendingAction
}

// ChargePower reports the player's current kick wind-up as a fraction of full
// power (0..1); 0 when not charging. Surfaced so clients can draw a power meter.
func (p *Player) ChargePower() float64 { return p.chargePower / maxChargeTime }

// ChargeSpin reports the wind-up's side-spin as a signed fraction (-1..1, the sign
// is the curve direction); 0 when no curve is dialled in.
func (p *Player) ChargeSpin() float64 { return p.spinAccum / maxSpinTime }

// ChargeLift reports the wind-up's loft as a signed fraction (-1..1: +loft / -driven);
// 0 when flat.
func (p *Player) ChargeLift() float64 { return p.loftAccum / maxLoftTime }

// Yaw derives the heading angle from Facing on demand. The engine works in
// vectors; this exists only for things that need an angle (e.g. the client
// rendering rotation). Convention: 0 = +Z, increasing toward +X.
func (p Player) Yaw() float64 {
	return math.Atan2(p.Facing.X, p.Facing.Z)
}
