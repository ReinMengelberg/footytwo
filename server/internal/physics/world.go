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

// ActionKind enumerates the queued ball actions a player can have pending. Only
// ActionTouch exists today; ActionPass and ActionShot are reserved so the
// pending-action slot is the future home for queued kicks — they resolve through
// the same "execute on the next contact" path the dribble touch uses.
type ActionKind uint8

const (
	ActionNone ActionKind = iota
	ActionTouch
	// ActionPass, ActionShot — later phases, once pass/shot inputs exist.
)

// PendingAction is the 1-deep action register: the most recent committed intent
// awaiting execution at the owner's feet. It is cleared on three events —
// execution, timeout (Age >= pendingActionMaxAge), and loss of possession. For a
// dribble touch the slot is continuously re-armed from live movement input each
// tick (latest-input-wins), so its Dir is just the current heading and it never
// times out while the player keeps dribbling; the timeout/loss clearing is the
// general mechanism that future one-shot pass/shot actions will rely on.
type PendingAction struct {
	Kind ActionKind
	Dir  Vec3    // committed knock direction (X/Z unit); future: pass/shot aim
	Age  float64 // seconds since armed; advanced each tick, used for the timeout
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

	gainCooldown float64 // seconds remaining before a possession gain may occur
	touchCount   int     // monotonic count of dribble touches (surfaced for clients)

	pending PendingAction // 1-deep action register: the queued touch (later: pass/shot)
	atFeet  bool          // ball within touchFireRadius of the owner this tick (snapshot contact flag)
}

// TouchCount returns the running total of dribble touches taken in this world. It
// only ever increases; clients diff it between snapshots to know a touch happened
// (and play the matching animation) without needing the exact tick.
func (w *World) TouchCount() int { return w.touchCount }

// AtFeet reports whether the ball was within touchFireRadius of the owner on the
// last step (false when there is no owner). It pulses true on contact and false
// while the ball is knocked ahead, and is what gates touch/action execution —
// "conservative possession" without making ownership itself fragile.
func (w *World) AtFeet() bool { return w.atFeet }

// armPending sets the pending action to a fresh intent (Age reset to 0).
func (w *World) armPending(kind ActionKind, dir Vec3) {
	w.pending = PendingAction{Kind: kind, Dir: dir}
}

// clearPending empties the pending-action slot.
func (w *World) clearPending() { w.pending = PendingAction{} }

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
			w.Ball.Vel = Vec3{} // corral: kill the free velocity
			w.clearPending()    // fresh possession: no queued action carried over
			w.gainCooldown = possessionGainCooldown
			return
		}
	}
}

// stepDribble advances a dribble as a sequence of discrete TOUCHES instead of
// gluing the ball to the feet. Each tick the ball rolls under dribbleRollDrag
// (keeping real momentum the instant it's released) while the owner runs after it.
// The control objective is to keep the ball IN FRONT along the queued
// (most-recent-input) direction: when its lead drops below frontMin the owner
// knocks it back out to ~knockAhead in front, so the ball follows the player in any
// direction, diagonals included, and a direction change redirects it on the next
// touch. Possession is conservative: the touch won't reach a ball that has ended up
// behind the feet (you turned away from it), and once the ball gets beyond
// loseReach — knocked too hard by a SPRINT touch, or simply left behind — it is
// released as a free ball to be re-won by chasing it down. See tuning.go for the
// full rationale.
func (w *World) stepDribble(in Input, dt float64) {
	owner, ok := w.Players[w.Owner]
	if !ok {
		w.Owner = ""
		w.clearPending()
		return
	}

	// Age the pending action and time it out if it's gone stale. A dribble touch
	// is re-armed below every tick there's input, so this only bites a future
	// one-shot action that can't reach the feet.
	if w.pending.Kind != ActionNone {
		w.pending.Age += dt
		if w.pending.Age >= pendingActionMaxAge {
			w.clearPending()
		}
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

	// Arm/refresh the pending touch from LIVE movement input (latest-input-wins).
	// On a zero-input tick we keep the last committed direction so a momentary gap
	// doesn't wipe a queued cut; the timeout above is what eventually clears it.
	moveDir := in.Dir.NormalizedXZ()
	if moveDir.LengthXZ() > epsilon {
		w.armPending(ActionTouch, moveDir)
	}

	// How far the ball is from the feet, and whether it reads as "at the feet" for
	// the client. Possession is conservative: once the ball is beyond loseReach it is
	// out of control — knocked too hard, or left behind because the player turned and
	// ran away from it — so drop it to a free ball to be chased down.
	rel := w.Ball.Pos.Sub(owner.Pos)
	dist := rel.LengthXZ()
	w.atFeet = dist <= touchFireRadius
	if dist > loseReach {
		w.Owner = ""
		w.atFeet = false
		w.clearPending()
		w.gainCooldown = possessionGainCooldown
		return
	}

	// A touch needs a queued intent and a moving owner (a stationary owner lets the
	// ball settle at the feet).
	speed := owner.Vel.LengthXZ()
	if w.pending.Kind != ActionTouch || speed <= epsilon {
		return
	}

	// Keep the ball in front along the queued direction. Fire a touch when its lead
	// has dropped below frontMin and it's within touchReach.
	aim := w.pending.Dir
	ahead := rel.DotXZ(aim) // signed distance the ball leads the feet along aim
	if ahead >= frontMin || dist > touchReach {
		return
	}

	// ...but you can only steer a ROLLING ball within the towardMin cone of its own
	// travel: turn sharply off the ball's line and you're moving away from it, so we
	// don't knock it — its momentum carries it on and the loseReach check above drops
	// possession. A SLOW ball (at the feet, under control) can be knocked any
	// direction: that's what lets you start dribbling and play tight cuts.
	ballSpeed := w.Ball.Vel.LengthXZ()
	if ballSpeed > slowBallCap && aim.DotXZ(w.Ball.Vel.NormalizedXZ()) < towardMin {
		return
	}

	// Knock the ball toward a point ~knockAhead further ahead than it is now, along
	// the queued direction. Anchoring the target to the ball's own lead means we never
	// aim behind a ball that's already ahead, so a touch is never backwards.
	target := owner.Pos.Add(aim.Scale(ahead + knockAhead))
	dir := target.Sub(w.Ball.Pos).NormalizedXZ()
	if dir.LengthXZ() < epsilon {
		dir = aim
	}
	w.Ball.Vel = dir.Scale(TouchSpeed(speed, owner.Stats.Dribbling, in.Sprint))
	w.touchCount++
	w.clearPending() // executed; it re-arms next tick from live input

	// A sprint touch is heavy: it knocks the ball loose and releases possession, to be
	// re-won by catching up (the gain cooldown stops an instant re-grab).
	if in.Sprint {
		w.Owner = ""
		w.gainCooldown = possessionGainCooldown
		w.clearPending()
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
