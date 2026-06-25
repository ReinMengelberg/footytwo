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
// possessing, looser ball control. Charge/Lift/Spin drive the charged kick: hold
// Charge to build power, and while charging the arrow intents set the kick's
// height (Lift) and curve (Spin). The server integrates hold-durations from these
// per-tick booleans, so the client only reports current key state.
type Input struct {
	Dir    Vec3 // desired move direction, X/Z plane
	Sprint bool // hold to sprint (top speed, looser control with the ball)
	Charge bool // hold to build kick power; release commits the kick
	Lift   int  // while charging: +1 = loft (into the air), -1 = driven (low), 0 = flat
	Spin   int  // while charging: -1 = curve one way, +1 = the other, 0 = none
}

// ActionKind enumerates the queued ball actions a player can have pending. Only
// ActionTouch exists today; ActionPass and ActionShot are reserved so the
// pending-action slot is the future home for queued kicks — they resolve through
// the same "execute on the next contact" path the dribble touch uses.
type ActionKind uint8

const (
	ActionNone ActionKind = iota
	ActionTouch
	ActionKick // a committed charged kick: fires on the next foot-contact, then releases possession
)

// PendingAction is the 1-deep action register: the most recent committed intent
// awaiting execution at the owner's feet. It is cleared on three events —
// execution, timeout (Age past the kind's max), and loss of possession. For a
// dribble touch the slot is continuously re-armed from live movement input each
// tick (latest-input-wins), so its Dir is just the current heading and it never
// times out while the player keeps dribbling. A committed ActionKick is the
// opposite: a one-shot that the dribble re-arm must not clobber, carrying the
// kick's power/loft/spin until the ball next reaches the feet.
type PendingAction struct {
	Kind  ActionKind
	Dir   Vec3    // committed direction (X/Z unit): dribble knock, or kick aim (facing at release)
	Age   float64 // seconds since armed; advanced each tick, used for the timeout
	Power float64 // kick only: charge seconds (floored) → horizontal launch speed
	Loft  float64 // kick only: signed loft seconds (+loft / -driven)
	Spin  float64 // kick only: signed spin seconds → Ball.Spin
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
	kickCount    int     // monotonic count of kicks (surfaced for clients)

	pending PendingAction // 1-deep action register: the queued touch (later: pass/shot)
	atFeet  bool          // ball within touchFireRadius of the owner this tick (snapshot contact flag)

	// Out-of-bounds / restart bookkeeping (see restart.go). lastTouch records the
	// last player to play the ball so a goal-line exit can be told apart (corner vs
	// goal kick); the rest is surfaced to clients to announce restarts and the score.
	lastTouch            string      // ID of the last player to touch the ball
	restartCount         int         // monotonic count of restarts awarded (out-of-bounds events)
	lastRestart          RestartKind // kind of the most recent restart
	scoreHome, scoreAway int         // goals for the Home (+X) and Away (-X) sides

	// Dead-ball pause: while restartTimer > 0 the ball is frozen out of play and
	// will be placed at restartSpot when it elapses (set by beginRestart).
	restartTimer float64 // seconds left in the post-out pause (0 = ball in play)
	restartSpot  Vec3    // where the ball is placed when the pause ends
}

// TouchCount returns the running total of dribble touches taken in this world. It
// only ever increases; clients diff it between snapshots to know a touch happened
// (and play the matching animation) without needing the exact tick.
func (w *World) TouchCount() int { return w.touchCount }

// KickCount returns the running total of kicks taken in this world. Like
// TouchCount it only increases; clients diff it between snapshots to fire a kick
// animation/sound without needing the exact tick.
func (w *World) KickCount() int { return w.kickCount }

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

	// Dead-ball pause after the ball goes out (see ResolveRestart): no possession or
	// kicks, but the ball keeps rolling on out of play (into the net / onto the turf)
	// so the exit is visible, kept on the field by clampDeadBall. Players still move;
	// the pause timer is counted down by ResolveRestart, not here.
	if w.restartTimer > 0 {
		w.stepFreeBall(dt)
		w.clampDeadBall()
		return
	}

	w.tryGainPossession()

	// The owner is in continuous contact with the ball, so they're the last to
	// have touched it (a first-time strike on a loose ball records itself, in
	// tryGainPossession). This decides corner vs goal kick if it later goes out.
	if w.Owner != "" {
		w.lastTouch = w.Owner
	}

	// Process kick charging after possession is resolved (so "possessing" is
	// current) and before the dribble step (so a kick committed on release this
	// tick is in the pending slot when stepDribble looks for a foot-contact).
	w.stepCharge(inputs, dt)

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
	if w.Ball.Pos.Y > BallRadius+groundEpsilon {
		return // a ball in flight or mid-bounce can't be corralled
	}
	for id, p := range w.Players {
		if p.Pos.Sub(w.Ball.Pos).LengthXZ() <= ControlRadius {
			// A committed kick is struck FIRST-TIME: the player reaches the ball and
			// kicks it straight away rather than taking it under control.
			if p.kick.Kind == ActionKick {
				w.launchKick(p, p.kick)
				p.kick = PendingAction{}
				w.kickCount++
				w.lastTouch = id // a first-time strike is this player's touch
				w.gainCooldown = possessionGainCooldown
				return
			}
			w.Owner = id
			w.Ball.Vel = Vec3{} // corral: kill the free velocity
			w.clearPending()    // fresh possession: no queued action carried over
			w.gainCooldown = possessionGainCooldown
			return
		}
	}
}

// stepCharge integrates each player's kick wind-up. A player can charge at ANY
// time, with or without the ball: while they hold Charge, power and the arrow-driven
// loft/spin ramp with hold time (capped). On the release edge (was charging, no
// longer) the kick is committed onto the player, where it waits to be consumed by
// the player's next touch of the ball (a first-time strike on a loose ball, or the
// next dribble contact). A committed kick ages out if no touch comes.
func (w *World) stepCharge(inputs map[string]Input, dt float64) {
	for id, p := range w.Players {
		in := inputs[id]

		// Age a committed kick; abandon it if no touch consumes it in time.
		if p.kick.Kind != ActionNone {
			p.kick.Age += dt
			if p.kick.Age >= kickActionMaxAge {
				p.kick = PendingAction{}
			}
		}

		switch {
		case in.Charge:
			p.chargePower = min(p.chargePower+dt, maxChargeTime)
			p.loftAccum = clampF(p.loftAccum+dt*sign(in.Lift), -maxLoftTime, maxLoftTime)
			p.spinAccum = clampF(p.spinAccum+dt*sign(in.Spin), -maxSpinTime, maxSpinTime)
		case p.wasCharging:
			// Release edge: commit the kick (aimed along current facing), then reset.
			power := max(p.chargePower, minChargeTime) // tap floor: a flick still passes
			p.kick = PendingAction{Kind: ActionKick, Dir: p.Facing, Power: power, Loft: p.loftAccum, Spin: p.spinAccum}
			p.chargePower, p.loftAccum, p.spinAccum = 0, 0, 0
		}
		p.wasCharging = in.Charge
	}
}

// launchKick fires a committed kick: it overwrites the ball's velocity with the
// launch computed from the kick's power (horizontal speed, minKickSpeed..max by
// charge), loft (vertical speed, trading a little pace; a negative Loft is a driven
// ball that stays flat and gains drivenBoost pace), and spin (lateral curve). The
// ball launches from its current position along the committed aim; stepFreeBall
// takes it from there under gravity, Magnus, and bounce.
func (w *World) launchKick(owner *Player, k PendingAction) {
	aim := k.Dir.NormalizedXZ()
	if aim.LengthXZ() < epsilon {
		aim = owner.Facing
	}

	horiz := lerp(minKickSpeed, maxKickSpeed, k.Power/maxChargeTime)
	vertical := 0.0
	switch {
	case k.Loft > 0: // loft into the air, trading a little horizontal pace
		lf := min(k.Loft, maxLoftTime) / maxLoftTime
		vertical = lf * maxLoftSpeed
		horiz *= 1 - loftHorizGive*lf
	case k.Loft < 0: // driven: flat and faster
		df := min(-k.Loft, maxLoftTime) / maxLoftTime
		horiz *= 1 + drivenBoost*df
	}

	w.Ball.Vel = Vec3{aim.X * horiz, vertical, aim.Z * horiz}
	w.Ball.Spin = clampF(k.Spin, -maxSpinTime, maxSpinTime) / maxSpinTime * maxSpin
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

	// Age the pending touch and time it out if it's gone stale. The touch is
	// re-armed below every tick there's input, so this rarely bites; it only clears
	// a queued cut the owner stopped feeding. (A committed kick lives on the player,
	// aged in stepCharge.)
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
	// No bounds clamp here: a ball dribbled over a line goes out of play, caught by
	// ResolveRestart (a throw-in or goal-line restart). Players still clamp to the pitch.

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

	// The owner's committed kick fires as soon as the ball is in reach of the foot:
	// launch it (in 3D), release possession, and block an instant self re-corral.
	// This outranks the dribble touch, so it's checked first.
	if owner.kick.Kind == ActionKick && dist <= touchReach {
		w.launchKick(owner, owner.kick)
		owner.kick = PendingAction{}
		w.Owner = ""
		w.atFeet = false
		w.gainCooldown = possessionGainCooldown
		w.kickCount++
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

// stepFreeBall advances a free ball as a full 3D body: gravity pulls it down,
// side-spin curves it (Magnus), it bounces on landing and rolls out, and its spin
// decays. Free physics is suspended while the ball is possessed and resumes on
// loss (a kick, or a knocked-loose dribble). It runs in this order each tick:
// gravity → Magnus → integrate → ground collision/bounce → drag → spin decay.
func (w *World) stepFreeBall(dt float64) {
	b := &w.Ball

	// Gravity on the vertical axis.
	b.Vel.Y -= gravity * dt

	// Magnus curve: a sideways acceleration perpendicular to horizontal travel,
	// scaled by spin and speed. perp = (-Vel.Z, 0, Vel.X) is the left-rotation of
	// the horizontal velocity about +Y, so +Spin curves a +Z-moving ball toward -X
	// (the sign is pinned by TestSpinCurvesOppositeDirections). Applies in flight
	// and on the ground.
	hspeed := math.Hypot(b.Vel.X, b.Vel.Z)
	if hspeed > epsilon && b.Spin != 0 {
		perp := Vec3{-b.Vel.Z, 0, b.Vel.X}.Scale(1 / hspeed)
		a := magnusFactor * b.Spin * hspeed
		b.Vel.X += perp.X * a * dt
		b.Vel.Z += perp.Z * a * dt
	}

	// Integrate all three axes.
	b.Pos.X += b.Vel.X * dt
	b.Pos.Y += b.Vel.Y * dt
	b.Pos.Z += b.Vel.Z * dt

	// Ground collision: clamp out of the floor and bounce a descending ball. Tiny
	// post-bounce hops settle to a roll so the ball doesn't buzz forever.
	grounded := false
	if b.Pos.Y <= BallRadius {
		b.Pos.Y = BallRadius
		if b.Vel.Y < 0 {
			b.Vel.Y = -b.Vel.Y * restitution
			if b.Vel.Y < bounceSettleCap {
				b.Vel.Y = 0
			}
		}
		grounded = true
	}

	// Horizontal drag: real roll drag on the deck, only a light drag aloft so a
	// lofted ball keeps its pace through the air. Gravity is the only vertical force.
	hdrag := airDrag
	if grounded {
		hdrag = freeBallDrag
	}
	decay := math.Exp(-hdrag * dt)
	b.Vel.X *= decay
	b.Vel.Z *= decay

	// Spin bleeds off over time.
	b.Spin *= math.Exp(-spinDecay * dt)
	if math.Abs(b.Spin) < spinSettleCap {
		b.Spin = 0
	}
	// No bounds clamp: a free ball is allowed to cross a boundary line (and visibly
	// run onto the surrounding turf) — ResolveRestart turns that into the right
	// restart. Step stays pure flight so it can be unit-tested without a pitch edge.
}
