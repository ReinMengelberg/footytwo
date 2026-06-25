package physics

import "math"

// Out-of-bounds and the restarts that follow it. Step (in world.go) flies the
// ball as a pure body and no longer pens it inside the pitch, so the ball can
// fully cross a boundary within a tick. ResolveRestart is the laws-of-the-game
// layer on top: called once per tick right after Step, it notices a ball that
// has left the field and awards the matching restart — a goal, a goal kick
// ("keeper ball"), a corner, or a throw-in.
//
// Rather than teleport instantly, going out opens a brief DEAD-BALL PAUSE: the
// restart is announced right away, but the ball keeps its momentum and ROLLS ON
// out of play — into the net for a goal, onto the surrounding turf otherwise — so
// the moment is actually visible. clampDeadBall keeps that roll on the rendered
// field (it can't sail off into the void). Only when the pause timer elapses is
// the ball placed at the awarded spot and play resumes; a goal dwells longer (a
// celebration) before kickoff. While paused the ball is uncontestable (no
// possession, no kicks), though players may still walk about.
//
// Keeping this OUT of Step is deliberate: Step stays a pure-flight simulation
// (unit-tested with kicks that sail far down the pitch), and only the match loop
// (the room) opts into boundary enforcement by calling ResolveRestart.

// Team is which half a player belongs to. It fixes the direction a player
// attacks, which is what tells a corner apart from a goal kick: the same ball
// over the same goal line is a corner if a DEFENDER put it out and a goal kick
// if an ATTACKER did. Home attacks the +X goal; Away attacks the -X goal. The
// zero value is Home.
type Team uint8

const (
	TeamHome Team = iota // attacks the +X goal, defends the -X goal
	TeamAway             // attacks the -X goal, defends the +X goal
)

// attackSign is the X sign of the goal a team is shooting at: Home → +X (+1),
// Away → -X (-1). A team defends the goal at the opposite sign.
func attackSign(t Team) float64 {
	if t == TeamAway {
		return -1
	}
	return 1
}

// RestartKind is how play resumes after the ball goes out (RestartNone while the
// ball is in play). Its String is the lowercase wire token sent to clients.
type RestartKind uint8

const (
	RestartNone     RestartKind = iota
	RestartThrowIn              // ball crossed a touchline (long side)
	RestartGoalKick             // over the goal line, last touched by the attacking team
	RestartCorner               // over the goal line, last touched by the defending team
	RestartGoal                 // through the mouth, under the bar — a goal; restart at kickoff
)

// String is the wire token for a restart (kept in sync with web/src/net/protocol.ts).
func (k RestartKind) String() string {
	switch k {
	case RestartThrowIn:
		return "throwin"
	case RestartGoalKick:
		return "goalkick"
	case RestartCorner:
		return "corner"
	case RestartGoal:
		return "goal"
	default:
		return ""
	}
}

// Goal mouth + restart geometry, in metres. These mirror the client's FIFA
// dimensions (web/src/game/Goal.ts, PitchMarkings.ts) so the goal the player
// sees is the goal the simulation scores into.
const (
	goalHalfWidth     = 7.32 / 2            // 3.66: half the distance between the posts
	goalHeight        = 2.44                // crossbar height; a ball above this is over, not in
	goalAreaDepth     = 5.5                 // 6-yard box depth; a goal kick restarts from its line
	goalAreaHalfWidth = goalHalfWidth + 5.5 // 9.16: half-width of the 6-yard box
)

// Dead-ball pause: how long the ball sits out of play before the restart is taken,
// so the moment is visible. A goal dwells longer for a celebration before kickoff.
const (
	restartPause    = 0.8 // seconds out of play before a throw-in / corner / goal kick
	goalCelebration = 2.5 // seconds to savour a goal before kicking off
)

// How far a dead ball may roll past a line before clampDeadBall stops it, keeping
// the roll-out on the rendered surface. ballOutMargin sits inside the client's
// 10 m turf border (Pitch.ts); goalNetDepth sits just inside the net (Goal.ts) so
// a scored ball settles in the goal rather than rolling out the back.
const (
	ballOutMargin = 5.0 // metres of turf a dead ball may run onto past a line
	goalNetDepth  = 1.8 // metres past the goal line a scored ball rolls into the net
)

// ResolveRestart enforces the touchlines and goal lines. If the ball has FULLY
// crossed a boundary (the whole ball over the line) it is out of play, and this
// awards the matching restart — releasing possession and opening the dead-ball
// pause that ends in the ball being placed at the restart spot — then returns
// true. It also drives that pause's countdown on subsequent calls. While the ball
// is in play it does nothing and returns false. Call it once per tick after Step.
func (w *World) ResolveRestart(dt float64) bool {
	// In a dead-ball pause: the ball is rolling on out of play (Step rolls and clamps
	// it, see clampDeadBall). Just count the timer down, and place it at the awarded
	// spot the moment the pause ends.
	if w.restartTimer > 0 {
		w.restartTimer -= dt
		if w.restartTimer <= 0 {
			w.restartTimer = 0
			w.placeRestartBall()
		}
		return false
	}

	p := w.Ball.Pos
	// How far the ball has crossed each line (negative = still inside). The ball
	// counts as out only once its whole body is over, hence the BallRadius gate.
	overX := math.Abs(p.X) - w.Bounds.HalfX // past a goal line (X is goal-to-goal)
	overZ := math.Abs(p.Z) - w.Bounds.HalfZ // past a touchline (Z is the long sides)
	if overX < BallRadius && overZ < BallRadius {
		return false // still in play
	}

	// At a corner both lines are crossed at once; attribute it to whichever the
	// ball is further beyond, so a ball that mostly crossed the goal line is a
	// goal-line event (corner / goal kick) and not a throw-in.
	if overX >= overZ {
		w.resolveGoalLine(math.Copysign(1, p.X), p.Z, p.Y)
	} else {
		w.resolveTouchLine(p.X, math.Copysign(1, p.Z))
	}
	return true
}

// resolveGoalLine handles a ball over the goal line at end sx (+1 = the +X goal).
// Between the posts and under the bar it's a goal; otherwise it's a corner if a
// defender of this goal put it out, or a goal kick if an attacker of this goal did.
func (w *World) resolveGoalLine(sx, ballZ, ballY float64) {
	if math.Abs(ballZ) <= goalHalfWidth && ballY <= goalHeight {
		w.awardGoal(sx)
		return
	}
	// The team attacking this goal has attackSign == sx; the defenders are the rest.
	if attackSign(w.lastTouchTeam()) == sx {
		// An attacker knocked it behind: goal kick (keeper ball) to the defenders,
		// taken from their 6-yard box on the side the ball left.
		w.beginRestart(RestartGoalKick, sx*(w.Bounds.HalfX-goalAreaDepth), math.Copysign(goalAreaHalfWidth*0.5, ballZ))
	} else {
		// A defender put it behind: corner to the attackers, at the nearest flag.
		w.beginRestart(RestartCorner, sx*w.Bounds.HalfX, math.Copysign(w.Bounds.HalfZ, ballZ))
	}
}

// resolveTouchLine handles a ball over a touchline: a throw-in at the point it
// crossed (its X, clamped to the pitch), on the line at side sz.
func (w *World) resolveTouchLine(ballX, sz float64) {
	x := clampF(ballX, -w.Bounds.HalfX, w.Bounds.HalfX)
	w.beginRestart(RestartThrowIn, x, sz*w.Bounds.HalfZ)
}

// awardGoal credits the team attacking goal sx and sets up a kickoff (the ball
// returns to the centre spot once the celebration pause ends).
func (w *World) awardGoal(sx float64) {
	if sx > 0 {
		w.scoreHome++ // Home attacks +X
	} else {
		w.scoreAway++ // Away attacks -X
	}
	w.beginRestart(RestartGoal, 0, 0)
}

// beginRestart opens the dead-ball pause: possession is released, the awarded spot
// is remembered for when the timer elapses, and the call is announced immediately
// (the restart counter advances and the score is already updated) so clients can
// flash it during the pause. A goal dwells longer. The ball itself is left alone —
// it keeps rolling out of play (clampDeadBall bounds the roll) so the exit is
// visible — and is only repositioned when the pause ends (placeRestartBall).
func (w *World) beginRestart(kind RestartKind, spotX, spotZ float64) {
	w.Owner = ""     // dead ball: drop possession the instant it goes out
	w.clearPending() //

	w.restartSpot = Vec3{X: spotX, Y: BallRadius, Z: spotZ}
	w.restartTimer = restartPause
	if kind == RestartGoal {
		w.restartTimer = goalCelebration
	}
	w.lastRestart = kind
	w.restartCount++
}

// clampDeadBall keeps a ball rolling out of play (during the pause) on the visible
// field: it may run ballOutMargin onto the turf past any line, and a scored ball
// settles in the net (goalNetDepth past the goal line, between the posts). Hitting
// a limit kills the outward velocity so the ball rests there rather than grinding.
func (w *World) clampDeadBall() {
	b := &w.Ball
	maxX := w.Bounds.HalfX + ballOutMargin
	maxZ := w.Bounds.HalfZ + ballOutMargin
	if w.lastRestart == RestartGoal {
		maxX = w.Bounds.HalfX + goalNetDepth // a scored ball settles in the net...
		maxZ = goalHalfWidth                 // ...and stays between the posts
	}
	if b.Pos.X > maxX {
		b.Pos.X, b.Vel.X = maxX, min(b.Vel.X, 0)
	} else if b.Pos.X < -maxX {
		b.Pos.X, b.Vel.X = -maxX, max(b.Vel.X, 0)
	}
	if b.Pos.Z > maxZ {
		b.Pos.Z, b.Vel.Z = maxZ, min(b.Vel.Z, 0)
	} else if b.Pos.Z < -maxZ {
		b.Pos.Z, b.Vel.Z = -maxZ, max(b.Vel.Z, 0)
	}
}

// placeRestartBall ends the pause: the ball appears at the restart spot as a fresh
// free ball, ready to be taken. Any committed kicks are dropped so a charged shot
// can't auto-fire on the restart, and the gain cooldown is cleared so the nearest
// player can collect it at once.
func (w *World) placeRestartBall() {
	w.PlaceFreeBall(w.restartSpot.X, w.restartSpot.Z) // Owner -> "", ball at the spot
	w.gainCooldown = 0
	for _, p := range w.Players {
		p.kick = PendingAction{}
	}
}

// lastTouchTeam is the team of the player who last touched the ball, defaulting
// to Home before anyone has (only reachable if the ball leaves untouched).
func (w *World) lastTouchTeam() Team {
	if p, ok := w.Players[w.lastTouch]; ok {
		return p.Team
	}
	return TeamHome
}

// RestartCount returns the running total of restarts awarded (out-of-bounds
// events). Like TouchCount it only increases; clients diff it between snapshots
// to announce a fresh throw-in/corner/goal kick/goal without needing the tick.
func (w *World) RestartCount() int { return w.restartCount }

// LastRestart returns the kind of the most recent restart (RestartNone before any).
func (w *World) LastRestart() RestartKind { return w.lastRestart }

// Score returns the current scoreline (home, away).
func (w *World) Score() (home, away int) { return w.scoreHome, w.scoreAway }

// LastTouch returns the ID of the player who last touched the ball ("" if none).
func (w *World) LastTouch() string { return w.lastTouch }
