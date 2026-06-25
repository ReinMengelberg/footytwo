package physics

// Tunable constants and the pure stat→physics mapping, kept in one place so the
// feel of the game can be tuned and unit-tested without touching step logic.
// Every function here is pure: stats in, physics parameters out.

const epsilon = 1e-9

// PACE → run speed (m/s).
const (
	minRunSpeed = 4.5
	maxRunSpeed = 8.5
)

// DRIBBLING → speed penalty while controlling the ball.
//
//	factor = dribbleSpeedFloor + dribbleSpeedGain * (dribbling/99)
//
// so dribbling 0 ≈ 0.55, 50 ≈ 0.78, 99 ≈ 1.00 (keeps full pace).
const (
	dribbleSpeedFloor = 0.55
	dribbleSpeedGain  = 0.45
)

// DRIBBLING → control tightness: the per-second rate the ball is pulled back
// toward its ideal position. Higher dribbling = tighter leash = less overrun.
const (
	minControlStrength = 6.0
	maxControlStrength = 14.0
)

// Possession and ball-control tuning.
const (
	BallRadius             = 0.11 // ~size-5 football radius, meters
	ControlRadius          = 0.6  // how close to a free ball to corral it, meters
	corralSpeedCap         = 6.0  // ball must be slower than this (m/s) to be corralled
	possessionGainCooldown = 0.3  // seconds after a gain before another may occur

	freeBallDrag = 1.5 // per-second ground drag on a free ball
)

// statFraction maps a 0–99 stat onto [0, 1], clamping defensively so callers
// can't push a parameter past its design range.
func statFraction(stat int) float64 {
	return float64(clampStat(stat)) / float64(StatMax)
}

// RunSpeed maps PACE to free-running speed in m/s.
func RunSpeed(pace int) float64 {
	return minRunSpeed + statFraction(pace)*(maxRunSpeed-minRunSpeed)
}

// DribbleSpeedFactor maps DRIBBLING to the fraction of run speed retained while
// controlling the ball: 0.55 at 0, ~0.78 at 50, 1.0 at 99.
func DribbleSpeedFactor(dribbling int) float64 {
	return dribbleSpeedFloor + dribbleSpeedGain*statFraction(dribbling)
}

// DribbleSpeed is the movement speed while controlling the ball.
func DribbleSpeed(pace, dribbling int) float64 {
	return RunSpeed(pace) * DribbleSpeedFactor(dribbling)
}

// ControlStrength maps DRIBBLING to the per-second pull rate toward the ideal
// ball position. Higher = tighter control.
func ControlStrength(dribbling int) float64 {
	return minControlStrength + statFraction(dribbling)*(maxControlStrength-minControlStrength)
}

// Sprint / jog (Phase 2). The pace-derived RunSpeed is reinterpreted as the
// SPRINT (top) speed; a jog baseline sits below it. activeSpeed = jog unless the
// player is sprinting.
const jogFactor = 0.65

// Sprint + ball = looser control (the risk/reward), scaled by dribbling: better
// dribblers loosen less. looseness = 1 - sprintLoosenSlope*(dribbling/99), so a
// 99 dribbler still loosens (by the floor) while a 0 dribbler loosens fully.
const sprintLoosenSlope = 0.6 // how strongly dribbling reduces looseness

// SprintSpeed is the pace-derived top speed (identical to RunSpeed; named for
// the sprint mechanic).
func SprintSpeed(pace int) float64 { return RunSpeed(pace) }

// JogSpeed is the relaxed baseline movement speed.
func JogSpeed(pace int) float64 { return RunSpeed(pace) * jogFactor }

// ActiveSpeed is the base movement speed (before any dribble penalty) for the
// given sprint flag.
func ActiveSpeed(pace int, sprint bool) float64 {
	if sprint {
		return SprintSpeed(pace)
	}
	return JogSpeed(pace)
}

// sprintLooseness is how much sprinting loosens ball control: 1.0 at dribbling
// 0, down to a floor at 99 (the best dribblers still loosen, but the least).
func sprintLooseness(dribbling int) float64 {
	return 1.0 - sprintLoosenSlope*statFraction(dribbling)
}

// Touch-based dribbling. Rather than gluing the ball to an ideal point, the owner
// knocks it forward in discrete TOUCHES: between touches the ball just rolls under
// dribbleRollDrag while the owner runs after it, and the queued (most-recent-input)
// direction decides where the next touch sends it. The control objective is to keep
// the ball IN FRONT of the player along that direction: a touch fires whenever the
// ball's lead drops below frontMin, knocking it back out to ~knockAhead in front, so
// the ball follows you and a turn redirects it on the next touch.
//
// Crucially, you only keep the ball while you're heading TOWARD it. You can only
// steer a ROLLING ball within a cone of its own travel: the move direction must be
// within ~acos(towardMin) of the ball's velocity. Turn sharper than that and you're
// moving AWAY from the ball — the touch doesn't reach it, its momentum carries it
// off, and possession is lost once it passes loseReach. A SLOW ball (slower than
// slowBallCap: at the feet, under control) escapes that cone and can be knocked any
// direction, which is what lets you start dribbling and play tight cuts. So:
// possession is conservative — dribble with the ball and it stays in front; turn and
// run away from it and you lose it. A SPRINT touch is heavy enough to knock the ball
// past loseReach by itself, so it also loses it.
//
// touchFireRadius (used only for the at-the-feet snapshot flag) sits just above
// ControlRadius so a freshly corralled ball reads as at the feet immediately.
const (
	dribbleRollDrag = 2.4  // per-second ground drag on the ball while being dribbled
	knockAhead      = 0.6  // a touch aims the ball this much further ahead of the feet
	frontMin        = 0.6  // re-knock once the ball's lead along the move dir drops below this (m)
	towardMin       = 0.3  // move dir must be within ~acos(0.3)≈72° of a rolling ball's travel to steer it
	slowBallCap     = 1.5  // a ball slower than this (m/s) is "at the feet" and can be knocked any direction
	touchReach      = 1.0  // a touch can only reach the ball within this distance (m)
	loseReach       = 1.3  // beyond this (m) the ball is out of control — possession is lost
	touchFireRadius = 0.65 // ball within this (m) of the feet reads as "at the feet" (snapshot flag only)
)

// pendingActionMaxAge bounds how long a queued action (a touch now, a pass/shot
// later) lives unrefreshed before it times out and is discarded. A dribble touch
// is re-armed from live input every tick so it never expires; the timeout exists
// for future one-shot actions that can't be executed because the ball never
// returns to the feet.
const pendingActionMaxAge = 0.5 // seconds

// A touch launches the ball to a multiple of the owner's speed so it pulls ahead,
// then drag and the chasing owner bring it back into reach. Kept gentle at a jog
// (ball stays close → turns redirect on the next touch) but heavy when sprinting
// (ball runs far → turns can't reach it → you lose it). Better dribblers knock a
// touch softer (tighter control).
const (
	touchPushFactor    = 1.7 // launch speed = ownerSpeed * this, before modifiers
	touchPushFloor     = 2.0 // minimum launch speed (m/s) so slow dribbles still knock
	sprintTouchGain    = 0.9 // up to +90% launch at max looseness when sprinting
	dribbleTightenGain = 0.2 // up to -20% launch for the best dribblers (tighter)
)

// TouchSpeed is the launch speed imparted to the ball on a dribble touch, given
// the owner's current speed, dribbling stat, and whether they're sprinting.
func TouchSpeed(ownerSpeed float64, dribbling int, sprint bool) float64 {
	v := ownerSpeed * touchPushFactor
	v *= 1 - dribbleTightenGain*statFraction(dribbling)
	if sprint {
		v *= 1 + sprintTouchGain*sprintLooseness(dribbling)
	}
	return max(v, touchPushFloor)
}
