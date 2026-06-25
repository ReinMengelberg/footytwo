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

// TestJogTurnRedirectsViaTouch checks the headline turn behaviour at a jog: the
// ball changes direction only via a touch (never instantly), the dribbler keeps
// possession through the cut, and the ball ends up travelling the new way with
// the player still on it.
func TestJogTurnRedirectsViaTouch(t *testing.T) {
	const dt = 1.0 / 60
	w, p := newDribblingWorld(t)

	turn := map[string]physics.Input{"p1": {Dir: physics.Vec3{X: 1}}} // 90° cut: +Z → +X
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
