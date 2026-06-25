package physics

import "math"

// Bounds is the pitch extent as half-sizes from the origin in the X/Z plane.
type Bounds struct {
	HalfX, HalfZ float64
}

// DefaultPitch matches the client's 120×80 ground plane (X width, Z length).
func DefaultPitch() Bounds { return Bounds{HalfX: 60, HalfZ: 40} }

// Clamp constrains a position to the pitch on X/Z, leaving Y untouched.
func (b Bounds) Clamp(p Vec3) Vec3 {
	p.X = clampF(p.X, -b.HalfX, b.HalfX)
	p.Z = clampF(p.Z, -b.HalfZ, b.HalfZ)
	return p
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Input is a player's intent for a tick. Dir need not be normalized (Step
// normalizes it so diagonals aren't faster). Sprint selects top speed and, when
// possessing, looser ball control. Kick/pass come later.
type Input struct {
	Dir    Vec3 // desired move direction, X/Z plane
	Sprint bool // hold to sprint (top speed, looser control with the ball)
}

// World is one match's authoritative simulation state: it owns all players and
// the ball, and tracks possession via Owner ("" = free ball). The game/room
// layer wraps it with identity and a tick loop; World itself is pure state +
// Step so it stays unit-testable headlessly.
type World struct {
	Players map[string]*Player
	Ball    Ball
	Owner   string // player ID in possession, or "" for a free ball
	Bounds  Bounds

	gainCooldown  float64 // seconds remaining before a possession gain may occur
	touchCooldown float64 // seconds remaining before the next dribble touch may fire
	touchCount    int     // monotonic count of dribble touches (surfaced for clients)
	dribbleFacing Vec3    // owner's facing on the previous dribble tick (detects turns)
}

// TouchCount returns the running total of dribble touches taken in this world. It
// only ever increases; clients diff it between snapshots to know a touch happened
// (and play the matching animation) without needing the exact tick.
func (w *World) TouchCount() int { return w.touchCount }

// NewWorld creates an empty world with a free ball resting at the origin.
func NewWorld(bounds Bounds) *World {
	return &World{
		Players: make(map[string]*Player),
		Ball:    Ball{Pos: Vec3{0, BallRadius, 0}},
		Bounds:  bounds,
	}
}

// AddPlayer constructs a player into the world: stats are clamped and Facing is
// normalized to the X/Z plane (defaulting to +Z if degenerate). Returns the
// stored pointer so callers can read it back.
func (w *World) AddPlayer(p Player) *Player {
	p.Stats = p.Stats.Clamped()
	f := p.Facing.NormalizedXZ()
	if f.LengthXZ() < epsilon {
		f = Vec3{0, 0, 1}
	}
	p.Facing = f
	stored := &p
	w.Players[p.ID] = stored
	return stored
}

// PlaceFreeBall puts a stationary free ball at the given X/Z (Y pinned to the
// ground) and releases any possession.
func (w *World) PlaceFreeBall(x, z float64) {
	w.Owner = ""
	w.Ball = Ball{Pos: Vec3{x, BallRadius, z}}
}

// Step advances the simulation by dt seconds: move every player, resolve
// possession, then update the ball (dribble while owned, free physics otherwise).
func (w *World) Step(inputs map[string]Input, dt float64) {
	if w.gainCooldown > 0 {
		w.gainCooldown = max(0, w.gainCooldown-dt)
	}

	for id, p := range w.Players {
		w.stepPlayer(p, inputs[id], id == w.Owner, dt)
	}

	w.tryGainPossession()

	if w.Owner != "" {
		w.stepDribble(inputs[w.Owner], dt)
	} else {
		w.stepFreeBall(dt)
	}
}

// stepPlayer applies one player's input: base speed is jog or sprint (Pace),
// reduced by the dribble penalty (Dribbling) when possessing. Movement is X/Z
// only; the move direction is normalized so diagonals aren't faster.
func (w *World) stepPlayer(p *Player, in Input, possessing bool, dt float64) {
	dir := in.Dir.NormalizedXZ()
	if dir.LengthXZ() > epsilon {
		speed := ActiveSpeed(p.Stats.Pace, in.Sprint)
		if possessing {
			speed *= DribbleSpeedFactor(p.Stats.Dribbling)
		}
		p.Vel = dir.Scale(speed)
		p.Facing = dir // face the way we move; only updated when there is input
	} else {
		p.Vel = Vec3{} // no input → stop; leave Facing unchanged
	}

	// Integrate on the ground plane only; Y stays pinned.
	p.Pos.X += p.Vel.X * dt
	p.Pos.Z += p.Vel.Z * dt
	p.Pos = w.Bounds.Clamp(p.Pos)
}

// tryGainPossession is the simplest gain transition: a free, slow-enough ball
// within ControlRadius of a player is corralled by that player. A short
// cooldown after any gain prevents instant re-grab jitter once balls can be
// knocked loose (later phase).
func (w *World) tryGainPossession() {
	if w.Owner != "" || w.gainCooldown > 0 {
		return
	}
	if w.Ball.Vel.LengthXZ() > corralSpeedCap {
		return
	}
	for id, p := range w.Players {
		if p.Pos.Sub(w.Ball.Pos).LengthXZ() <= ControlRadius {
			w.Owner = id
			w.Ball.Vel = Vec3{}     // corral: kill the free velocity
			w.dribbleFacing = Vec3{} // fresh possession: no previous facing to turn from
			w.gainCooldown = possessionGainCooldown
			return
		}
	}
}

// stepDribble advances a dribble as a sequence of discrete TOUCHES instead of
// gluing the ball to the feet. Each tick the ball rolls under dribbleRollDrag
// (keeping real momentum the instant it's released) while the owner runs after
// it. A touch fires only once the foot has REACHED the ball (within touchReach)
// and the owner is moving — so the ball ONLY changes direction on a touch, never
// the instant the player turns: to cut, you must catch the ball and knock it the
// new way. A jog touch keeps the ball close (turns redirect on the next touch); a
// SPRINT touch is heavy and knocks the ball loose — possession is released and
// must be re-won by catching up to it, so cutting at full pace can lose the ball.
func (w *World) stepDribble(in Input, dt float64) {
	owner, ok := w.Players[w.Owner]
	if !ok {
		w.Owner = ""
		return
	}
	if w.touchCooldown > 0 {
		w.touchCooldown = max(0, w.touchCooldown-dt)
	}

	// Roll the ball with dribble drag, grounded (Y pinned to BallRadius).
	decay := math.Exp(-dribbleRollDrag * dt)
	w.Ball.Vel = Vec3{w.Ball.Vel.X * decay, 0, w.Ball.Vel.Z * decay}
	w.Ball.Pos = Vec3{
		w.Ball.Pos.X + w.Ball.Vel.X*dt,
		BallRadius,
		w.Ball.Pos.Z + w.Ball.Vel.Z*dt,
	}
	w.Ball.Pos = w.Bounds.Clamp(w.Ball.Pos)

	// A touch needs a moving owner. (A stationary owner lets the ball settle at
	// the feet.) Two ways to touch:
	//   - running close: while running within touchReach of the ball, the owner
	//     keeps knocking it along their running direction, paced by the anti-jitter
	//     cooldown — so a ball at the feet always travels where the player heads,
	//     and gradual curves are continuously re-aimed, not just sharp cuts.
	//   - turn: a sharp change of direction redirects a ball that's a bit further
	//     out (within turnReach) — bypassing the cooldown so the cut is immediate,
	//     but only while the ball is reachable. Knock it too far (a sprint push)
	//     and a turn can't get to it, so you lose it.
	speed := owner.Vel.LengthXZ()
	if speed <= epsilon {
		return
	}
	rel := w.Ball.Pos.Sub(owner.Pos)
	dist := rel.LengthXZ()

	// A turn is the *edge* where the facing swings away from last tick (a cut), not
	// a standing state — so it fires one redirect touch, then the knock's velocity
	// curls the ball back in front while the cooldown suppresses re-touching. The
	// turn touch bypasses the cooldown (so a cut is immediate) but still needs the
	// ball within turnReach: knock it too far and a turn can't reach it.
	turnedNow := w.dribbleFacing.LengthXZ() > epsilon &&
		owner.Facing.DotXZ(w.dribbleFacing) < touchTurnAngleCos
	w.dribbleFacing = owner.Facing

	runningClose := w.touchCooldown <= 0 && dist <= touchReach
	turned := turnedNow && dist <= turnReach
	if !runningClose && !turned {
		return
	}

	// Knock the ball toward a point a little further ahead than it already is,
	// along the CURRENT (running) facing. The forward component always pushes the
	// ball where the player is heading; the sideways component curls a ball that's
	// drifted off-centre (e.g. after a turn) back in front instead of leaving it
	// running parallel. Anchoring the target to the ball's own ahead-distance —
	// rather than a fixed point off the feet — means we never aim *behind* a ball
	// that's sitting further out than that point, so a touch is never backwards.
	ahead := rel.DotXZ(owner.Facing) // signed distance the ball is ahead of the feet
	target := owner.Pos.Add(owner.Facing.Scale(ahead + knockAhead))
	dir := target.Sub(w.Ball.Pos).NormalizedXZ()
	if dir.LengthXZ() < epsilon {
		dir = owner.Facing
	}
	w.Ball.Vel = dir.Scale(TouchSpeed(speed, owner.Stats.Dribbling, in.Sprint))
	w.touchCooldown = minTouchInterval
	w.touchCount++

	// A sprint touch is loose: the ball is knocked free and must be recovered by
	// catching up to it (the corral cooldown stops an instant re-grab). This is
	// the risk of sprint-dribbling, and means a sprint turn can't redirect the
	// ball on the spot — it's already gone.
	if in.Sprint {
		w.Owner = ""
		w.gainCooldown = possessionGainCooldown
	}
}

// stepFreeBall advances a free ball. Phase 1b: grounded with mild drag only —
// gravity, Magnus, and bounce arrive in a later phase. Free physics is
// suspended entirely while the ball is possessed and resumes on loss.
func (w *World) stepFreeBall(dt float64) {
	decay := math.Exp(-freeBallDrag * dt)
	w.Ball.Vel = Vec3{w.Ball.Vel.X * decay, 0, w.Ball.Vel.Z * decay}
	w.Ball.Pos = Vec3{
		w.Ball.Pos.X + w.Ball.Vel.X*dt,
		BallRadius,
		w.Ball.Pos.Z + w.Ball.Vel.Z*dt,
	}
	w.Ball.Pos = w.Bounds.Clamp(w.Ball.Pos)
}
