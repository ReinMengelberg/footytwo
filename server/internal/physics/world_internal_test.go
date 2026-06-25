package physics

import "testing"

// TestPendingTimeoutClears exercises the pending-action timeout from inside the
// package (the slot is unexported). An armed action that is never refreshed and
// never executed must be discarded once it has aged past pendingActionMaxAge.
// This is the general clearing rule the future one-shot pass/shot actions rely
// on; a dribble touch never hits it because live input re-arms it every tick.
func TestPendingTimeoutClears(t *testing.T) {
	const dt = 1.0 / 60

	w := NewWorld(DefaultPitch())
	w.AddPlayer(Player{ID: "p1", Facing: Vec3{Z: 1}, Stats: Stats{Pace: 70, Dribbling: 70}})
	// Own the ball but keep it far from the feet (beyond turnReach) so no touch can
	// fire and consume the slot — only the timeout can clear it.
	// Ball near the feet (within reach, so possession isn't dropped) but the owner
	// gets no input, so it's never knocked or re-armed — only the timeout can act.
	w.Owner = "p1"
	w.Ball = Ball{Pos: Vec3{X: 0.4, Y: BallRadius, Z: 0}}
	w.armPending(ActionTouch, Vec3{Z: 1})

	// Empty input → stationary owner, and crucially no re-arming from movement.
	idle := map[string]Input{}

	// Just before the timeout the action is still armed.
	for i := 0; i < 25; i++ { // ~0.42s < pendingActionMaxAge (0.5s)
		w.Step(idle, dt)
	}
	if w.pending.Kind != ActionTouch {
		t.Fatalf("pending action cleared too early (kind=%d, age=%.3f)", w.pending.Kind, w.pending.Age)
	}

	// Past the timeout it must be gone.
	for i := 0; i < 10; i++ { // pushes total age past 0.5s
		w.Step(idle, dt)
	}
	if w.pending.Kind != ActionNone {
		t.Fatalf("pending action should have timed out, still kind=%d age=%.3f", w.pending.Kind, w.pending.Age)
	}
}
