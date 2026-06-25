package tests

import (
	"math"
	"testing"

	"footy/server/internal/physics"
)

// newDribblingWorld returns a world whose seeded player is already running with
// the ball, plus the player handle. The player jogs +Z onto the ball and a few
// touches settle before the caller takes over.
func newDribblingWorld(t *testing.T) (*physics.World, *physics.Player) {
	t.Helper()
	w := physics.NewWorld(physics.DefaultPitch())
	p := w.AddPlayer(physics.Player{
		ID: "p1", Facing: physics.Vec3{Z: 1},
		Stats: physics.Stats{Pace: 78, Dribbling: 82},
	})
	w.PlaceFreeBall(0, 1) // 1m ahead in +Z

	in := map[string]physics.Input{"p1": {Dir: physics.Vec3{Z: 1}}}
	for i := 0; i < 60 && w.Owner != "p1"; i++ {
		w.Step(in, 1.0/60)
	}
	if w.Owner != "p1" {
		t.Fatalf("player never gained possession")
	}
	for i := 0; i < 30; i++ { // let the dribble settle into its rhythm
		w.Step(in, 1.0/60)
	}
	return w, p
}

// TestDribbleTouchesKnockBallForward checks the headline behaviour: while
// dribbling in a straight line the ball is repeatedly TOUCHED forward (the
// touch count rises) and stays loosely ahead in a sane band — never glued to the
// feet, never running away.
func TestDribbleTouchesKnockBallForward(t *testing.T) {
	const dt = 1.0 / 60
	w, p := newDribblingWorld(t)
	in := map[string]physics.Input{"p1": {Dir: physics.Vec3{Z: 1}}}

	start := w.TouchCount()
	var minAhead, maxAhead = math.Inf(1), math.Inf(-1)
	for i := 0; i < 180; i++ { // 3 seconds
		w.Step(in, dt)
		ahead := w.Ball.Pos.Sub(p.Pos).DotXZ(p.Facing)
		minAhead = math.Min(minAhead, ahead)
		maxAhead = math.Max(maxAhead, ahead)
	}
	touches := w.TouchCount() - start

	t.Logf("touches in 3s: %d; ahead range [%.2f, %.2f]m", touches, minAhead, maxAhead)
	if touches < 3 {
		t.Errorf("expected several touches while dribbling 3s, got %d", touches)
	}
	if touches > 60 {
		t.Errorf("touches firing too fast (%d in 3s) — jittering", touches)
	}
	if minAhead <= 0 {
		t.Errorf("ball drifted behind the feet (minAhead=%.2f)", minAhead)
	}
	if maxAhead > 1.6 {
		t.Errorf("ball ran too far ahead (maxAhead=%.2f) — not under control", maxAhead)
	}
}

// TestJogTurnRedirectsViaTouch checks the headline turn behaviour at a jog: on a
// MODERATE turn (within the steering cone) the ball changes direction only via a
// touch (never instantly), the dribbler keeps possession through the cut, and the
// ball ends up travelling the new way with the player still on it. (A sharper turn
// off the ball's line loses it — see TestSharpTurnLosesBall.)
func TestJogTurnRedirectsViaTouch(t *testing.T) {
	const dt = 1.0 / 60
	w, p := newDribblingWorld(t)

	turn := map[string]physics.Input{"p1": {Dir: physics.Vec3{X: 1, Z: 0.6}}} // ~59° turn off +Z
	before := w.TouchCount()

	redirected := false
	for i := 0; i < 60; i++ { // up to 1s to complete the cut
		w.Step(turn, dt)
		if w.Owner != "p1" {
			t.Fatalf("lost possession on a JOG turn at tick %d (should keep it)", i)
		}
		if w.Ball.Vel.X > 1 && w.Ball.Vel.X > math.Abs(w.Ball.Vel.Z) {
			redirected = true
			break
		}
	}
	if !redirected {
		t.Fatalf("ball never redirected toward +X on a jog turn (vel=%+v)", w.Ball.Vel)
	}
	if w.TouchCount() <= before {
		t.Errorf("the redirect happened without a touch firing")
	}

	// The ball should still be loosely under control after the cut.
	if dist := w.Ball.Pos.Sub(p.Pos).LengthXZ(); dist > 1.2 {
		t.Errorf("ball got away after the cut: dist=%.2f", dist)
	}
}

// TestSprintTouchLosesPossession checks that touching the ball while sprinting
// knocks it loose: possession is released on the touch and isn't re-won on the
// very next tick.
func TestSprintTouchLosesPossession(t *testing.T) {
	const dt = 1.0 / 60
	w, _ := newDribblingWorld(t)

	sprint := map[string]physics.Input{"p1": {Dir: physics.Vec3{Z: 1}, Sprint: true}}
	released := false
	for i := 0; i < 60; i++ { // a sprint touch should fire and drop possession within 1s
		w.Step(sprint, dt)
		if w.Owner == "" {
			released = true
			break
		}
	}
	if !released {
		t.Fatalf("sprint-dribbling never knocked the ball loose")
	}
	if w.Ball.Vel.LengthXZ() < 1 {
		t.Errorf("a knocked-loose ball should carry real pace, got %.2f m/s", w.Ball.Vel.LengthXZ())
	}
}

// TestAtFeetPulsesBetweenTouches checks the contact model directly: while
// dribbling, the ball is at the feet (atFeet) only intermittently — it pulses
// true on contact and false while it's been knocked ahead. Seeing both proves
// the touch is contact-driven, not glued.
func TestAtFeetPulsesBetweenTouches(t *testing.T) {
	const dt = 1.0 / 60
	w, _ := newDribblingWorld(t)
	in := map[string]physics.Input{"p1": {Dir: physics.Vec3{Z: 1}}}

	var sawAtFeet, sawAway bool
	for i := 0; i < 120; i++ { // 2 seconds
		w.Step(in, dt)
		if w.AtFeet() {
			sawAtFeet = true
		} else {
			sawAway = true
		}
	}
	if !sawAtFeet {
		t.Errorf("ball was never at the feet while dribbling — touches can't be firing on contact")
	}
	if !sawAway {
		t.Errorf("ball was always at the feet — it's glued, not being knocked ahead")
	}
}

// TestModerateTurnsKeepBall checks that a sequence of MODERATE turns (each well
// within the steering cone) keeps possession: you weave with the ball and it follows
// in front. Each leg turns ~45° off the previous one, so the ball stays on the
// player's line. (Sharper turns are meant to lose it — see TestSharpTurnLosesBall.)
func TestModerateTurnsKeepBall(t *testing.T) {
	const dt = 1.0 / 60
	w, _ := newDribblingWorld(t)

	// +Z → diagonal → +X → diagonal: each step is ~45° off the last, within the cone.
	dirs := []physics.Vec3{{X: 1, Z: 1}, {X: 1}, {X: 1, Z: -1}}
	for _, d := range dirs {
		in := map[string]physics.Input{"p1": {Dir: d}}
		for i := 0; i < 40; i++ { // ~0.67s per leg
			w.Step(in, dt)
			if w.Owner != "p1" {
				t.Fatalf("lost possession on a moderate turn toward %+v at tick %d (should keep it)", d, i)
			}
		}
	}
}

// TestSharpTurnLosesBall is the regression for the headline complaint: while
// dribbling, turning sharply OFF the ball's line (here ~90°) and running on must
// lose possession — you've moved away from a rolling ball and can't reach it.
func TestSharpTurnLosesBall(t *testing.T) {
	const dt = 1.0 / 60
	w, _ := newDribblingWorld(t)

	// Establish a forward dribble so the ball is rolling +Z.
	fwd := map[string]physics.Input{"p1": {Dir: physics.Vec3{Z: 1}}}
	for i := 0; i < 20; i++ {
		w.Step(fwd, dt)
	}
	// Veer 90° to +X and keep running — away from the rolling ball.
	veer := map[string]physics.Input{"p1": {Dir: physics.Vec3{X: 1}}}
	lost := false
	for i := 0; i < 60; i++ {
		w.Step(veer, dt)
		if w.Owner == "" {
			lost = true
			break
		}
	}
	if !lost {
		t.Errorf("kept possession after veering 90° away from a rolling ball — should lose it")
	}
}

// TestStationaryOwnerDoesNotNudge checks that a stationary owner with the ball
// settled at the feet does NOT perpetually nudge it (no touch fires without
// movement), and that touches resume once the player starts moving.
func TestStationaryOwnerDoesNotNudge(t *testing.T) {
	const dt = 1.0 / 60
	w := physics.NewWorld(physics.DefaultPitch())
	w.AddPlayer(physics.Player{ID: "p1", Facing: physics.Vec3{Z: 1}, Stats: physics.Stats{Pace: 78, Dribbling: 82}})
	w.PlaceFreeBall(0, 0) // right on the player → corralled immediately

	idle := map[string]physics.Input{} // no input: stationary owner
	for i := 0; i < 60 && w.Owner != "p1"; i++ {
		w.Step(idle, dt)
	}
	if w.Owner != "p1" {
		t.Fatalf("player never gained possession of the ball at its feet")
	}

	settled := w.TouchCount()
	for i := 0; i < 120; i++ { // 2s standing still with the ball
		w.Step(idle, dt)
	}
	if got := w.TouchCount() - settled; got != 0 {
		t.Errorf("a stationary owner touched the ball %d times — it should settle, not nudge", got)
	}

	run := map[string]physics.Input{"p1": {Dir: physics.Vec3{Z: 1}}}
	for i := 0; i < 120; i++ { // now move off
		w.Step(run, dt)
	}
	if w.TouchCount() <= settled {
		t.Errorf("touches never resumed once the owner started moving")
	}
}

// TestNoStallWhenBallPinned guards against the dribble stalling: over a
// sustained straight dribble the ball must keep clearing the fire radius and
// coming back, so touches keep firing across the WHOLE run, not just at the start.
func TestNoStallWhenBallPinned(t *testing.T) {
	const dt = 1.0 / 60
	w, _ := newDribblingWorld(t)
	in := map[string]physics.Input{"p1": {Dir: physics.Vec3{Z: 1}}}

	first := w.TouchCount()
	for i := 0; i < 90; i++ { // first 1.5s
		w.Step(in, dt)
	}
	mid := w.TouchCount()
	for i := 0; i < 90; i++ { // second 1.5s
		w.Step(in, dt)
	}
	last := w.TouchCount()

	if mid-first == 0 {
		t.Errorf("no touches in the first 1.5s of a straight dribble")
	}
	if last-mid == 0 {
		t.Errorf("touches stalled in the second 1.5s — the cycle stopped re-arming")
	}
}

// TestDiagonalJogKeepsBall is the regression for the diagonal-dribble bug: jogging
// diagonally must keep the ball in front and under control, not strand it behind
// and run off with phantom possession. The ball must stay owned, ahead of the feet,
// and within reach for the whole run.
func TestDiagonalJogKeepsBall(t *testing.T) {
	const dt = 1.0 / 60
	w, p := newDribblingWorld(t)
	in := map[string]physics.Input{"p1": {Dir: physics.Vec3{X: 1, Z: 1}}} // 45° diagonal

	for i := 0; i < 180; i++ { // 3 seconds
		w.Step(in, dt)
		if w.Owner != "p1" {
			t.Fatalf("lost the ball while jogging diagonally at tick %d (should follow the player)", i)
		}
		rel := w.Ball.Pos.Sub(p.Pos)
		ahead := rel.DotXZ(p.Facing)
		if i > 20 { // after the turn settles
			if ahead <= 0 {
				t.Fatalf("ball fell behind the player on a diagonal at tick %d (ahead=%.2f)", i, ahead)
			}
			if d := rel.LengthXZ(); d > 1.0 {
				t.Fatalf("ball drifted out of control on a diagonal at tick %d (dist=%.2f)", i, d)
			}
		}
	}
}

// TestLosesPossessionWhenMovingAway is the regression for the possession rule: if
// the player turns and runs AWAY from the ball, they should lose it (it becomes a
// free ball), rather than keep phantom possession of a ball left far behind.
func TestLosesPossessionWhenMovingAway(t *testing.T) {
	const dt = 1.0 / 60
	w, _ := newDribblingWorld(t)

	// Dribble forward a beat so the ball is knocked out ahead in +Z.
	fwd := map[string]physics.Input{"p1": {Dir: physics.Vec3{Z: 1}}}
	for i := 0; i < 20; i++ {
		w.Step(fwd, dt)
	}
	if w.Owner != "p1" {
		t.Fatalf("precondition failed: not dribbling before the turn")
	}

	// Now turn around and run the other way, away from the ball.
	away := map[string]physics.Input{"p1": {Dir: physics.Vec3{Z: -1}}}
	lost := false
	for i := 0; i < 90; i++ { // up to 1.5s
		w.Step(away, dt)
		if w.Owner == "" {
			lost = true
			break
		}
	}
	if !lost {
		t.Errorf("kept possession of a ball we ran away from — it should drop to a free ball")
	}
}
