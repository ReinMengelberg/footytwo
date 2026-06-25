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

// TestCommittedKickWaitsThenFires: a committed kick lives on the player and waits
// while the ball is out of foot reach, then fires (releasing possession) once the
// ball comes into reach.
func TestCommittedKickWaitsThenFires(t *testing.T) {
	const dt = 1.0 / 60
	w := NewWorld(DefaultPitch())
	p := w.AddPlayer(Player{ID: "p1", Facing: Vec3{Z: 1}, Stats: Stats{Pace: 78, Dribbling: 82}})
	w.Owner = "p1"
	w.Ball = Ball{Pos: Vec3{X: 0, Y: BallRadius, Z: 1.15}} // beyond touchReach(1.0), within loseReach(1.3)
	p.kick = PendingAction{Kind: ActionKick, Dir: Vec3{Z: 1}, Power: 0.3}
	before := w.KickCount()

	w.Step(map[string]Input{}, dt) // out of reach → stays committed
	if p.kick.Kind != ActionKick {
		t.Fatalf("kick should wait while the ball is out of reach, kind=%d", p.kick.Kind)
	}
	if w.KickCount() != before {
		t.Fatalf("kick fired while out of reach")
	}

	w.Ball.Pos = Vec3{X: 0, Y: BallRadius, Z: 0.5} // bring it into reach
	w.Step(map[string]Input{}, dt)
	if w.KickCount() != before+1 {
		t.Errorf("kick should fire once the ball is in reach")
	}
	if w.Owner != "" {
		t.Errorf("kick should release possession, Owner=%q", w.Owner)
	}
	if p.kick.Kind != ActionNone {
		t.Errorf("kick should be consumed, kind=%d", p.kick.Kind)
	}
}

// TestKickTimesOut: a committed kick that never reaches a touch is abandoned after
// kickActionMaxAge (it lives on the player and is aged in stepCharge).
func TestKickTimesOut(t *testing.T) {
	const dt = 1.0 / 60
	w := NewWorld(DefaultPitch())
	p := w.AddPlayer(Player{ID: "p1", Facing: Vec3{Z: 1}, Stats: Stats{Pace: 70, Dribbling: 70}})
	w.Owner = "p1"
	w.Ball = Ball{Pos: Vec3{X: 0, Y: BallRadius, Z: 1.15}} // owned but out of foot reach, stationary
	p.kick = PendingAction{Kind: ActionKick, Dir: Vec3{Z: 1}, Power: 0.3}

	idle := map[string]Input{}
	for i := 0; i < 120; i++ { // 2.0s < kickActionMaxAge (2.5s)
		w.Step(idle, dt)
	}
	if p.kick.Kind != ActionKick {
		t.Fatalf("kick cleared too early, age=%.3f", p.kick.Age)
	}
	for i := 0; i < 40; i++ { // total ~2.67s > 2.5s
		w.Step(idle, dt)
	}
	if p.kick.Kind != ActionNone {
		t.Fatalf("kick should have timed out, kind=%d age=%.3f", p.kick.Kind, p.kick.Age)
	}
}

// TestChargeWithoutPossessionCommitsKick: a player with NO ball can still charge,
// and releasing commits a kick (which a later touch will strike first-time).
func TestChargeWithoutPossessionCommitsKick(t *testing.T) {
	const dt = 1.0 / 60
	w := NewWorld(DefaultPitch())
	p := w.AddPlayer(Player{ID: "p1", Facing: Vec3{Z: 1}, Stats: Stats{Pace: 70, Dribbling: 70}})
	w.PlaceFreeBall(0, 20) // free ball far away; the player never possesses it

	hold := map[string]Input{"p1": {Charge: true}}
	for i := 0; i < 30; i++ { // 0.5s of charge without the ball
		w.Step(hold, dt)
	}
	if w.Owner != "" {
		t.Fatalf("precondition: player should not have the ball, Owner=%q", w.Owner)
	}
	if p.chargePower < 0.45 || p.chargePower > 0.55 {
		t.Errorf("chargePower without the ball = %.3f, want ~0.5", p.chargePower)
	}

	w.Step(map[string]Input{"p1": {}}, dt) // release
	if p.kick.Kind != ActionKick {
		t.Errorf("release should commit a kick, kind=%d", p.kick.Kind)
	}
	if p.chargePower != 0 {
		t.Errorf("charge should reset on release, got %.3f", p.chargePower)
	}
}
