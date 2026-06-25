package tests

import (
	"testing"

	"footy/server/internal/physics"
)

func TestPossessionGainWithinRadius(t *testing.T) {
	w := physics.NewWorld(physics.DefaultPitch())
	w.AddPlayer(physics.Player{ID: "p1", Facing: physics.Vec3{Z: 1}, Stats: physics.Stats{Pace: 70, Dribbling: 70}})
	w.PlaceFreeBall(0.3, 0) // 0.3m from the player at origin, inside ControlRadius (0.6)

	if w.Owner != "" {
		t.Fatalf("expected a free ball initially, Owner=%q", w.Owner)
	}
	w.Step(map[string]physics.Input{}, 1.0/60) // no input; the player corrals the nearby ball

	if w.Owner != "p1" {
		t.Fatalf("expected p1 to gain possession, Owner=%q", w.Owner)
	}
}

func TestNoGainWhenBallTooFast(t *testing.T) {
	w := physics.NewWorld(physics.DefaultPitch())
	w.AddPlayer(physics.Player{ID: "p1", Facing: physics.Vec3{Z: 1}, Stats: physics.Stats{Pace: 70, Dribbling: 70}})
	w.PlaceFreeBall(0.3, 0)
	w.Ball.Vel = physics.Vec3{X: specCorralCap + 1} // above the corral cap

	w.Step(map[string]physics.Input{}, 1.0/60)

	if w.Owner != "" {
		t.Errorf("should not corral a fast ball, Owner=%q", w.Owner)
	}
}

// TestGainCooldownBlocksRegrab exercises the post-gain cooldown behaviorally
// (via the public Owner/Ball/Players surface): after a gain, an immediate
// re-grab is blocked until the cooldown elapses.
func TestGainCooldownBlocksRegrab(t *testing.T) {
	w := physics.NewWorld(physics.DefaultPitch())
	w.AddPlayer(physics.Player{ID: "p1", Facing: physics.Vec3{Z: 1}, Stats: physics.Stats{Pace: 70, Dribbling: 70}})
	w.PlaceFreeBall(0, 0) // right on the player

	w.Step(map[string]physics.Input{}, 0.1)
	if w.Owner != "p1" {
		t.Fatalf("expected initial gain, Owner=%q", w.Owner)
	}

	// Simulate the ball being knocked loose at the feet (a later-phase event).
	w.Owner = ""
	w.Ball.Pos = w.Players["p1"].Pos
	w.Ball.Vel = physics.Vec3{}

	// The 0.3s cooldown is active: stepping must NOT re-grab yet.
	w.Step(map[string]physics.Input{}, 0.1) // cooldown 0.3 → 0.2
	if w.Owner != "" {
		t.Fatalf("re-grabbed during cooldown at t≈0.1s")
	}
	w.Step(map[string]physics.Input{}, 0.1) // 0.2 → 0.1
	if w.Owner != "" {
		t.Fatalf("re-grabbed during cooldown at t≈0.2s")
	}
	w.Step(map[string]physics.Input{}, 0.1) // 0.1 → 0.0, gain allowed again
	if w.Owner != "p1" {
		t.Fatalf("expected re-grab after cooldown elapsed, Owner=%q", w.Owner)
	}
}

// TestDribbleHeadlessDemo is the headless tick test from the acceptance
// criteria: the seeded player jogs to the ball, gains possession, the ball then
// tracks ahead along Facing, and dribbling is slower than free jogging.
func TestDribbleHeadlessDemo(t *testing.T) {
	const (
		pace = 78
		drib = 82
		dt   = 1.0 / 60
	)

	w := physics.NewWorld(physics.DefaultPitch())
	w.AddPlayer(physics.Player{
		ID: "p1", Name: "Tryout",
		Facing: physics.Vec3{Z: 1},
		Stats:  physics.Stats{Pace: pace, Dribbling: drib},
	})
	w.PlaceFreeBall(0, 2) // 2m ahead in +Z

	in := map[string]physics.Input{"p1": {Dir: physics.Vec3{Z: 1}}} // jog toward the ball

	var freeSpeed, dribbleSpeed float64
	gainedTick := -1

	for i := 0; i < 180; i++ { // 3 seconds
		w.Step(in, dt)
		p := w.Players["p1"]
		if w.Owner == "" {
			freeSpeed = p.Vel.LengthXZ() // latest speed while still free-running
		} else {
			if gainedTick < 0 {
				gainedTick = i
			}
			dribbleSpeed = p.Vel.LengthXZ() // speed while dribbling
		}
	}

	p := w.Players["p1"]
	if w.Owner != "p1" {
		t.Fatalf("player never gained possession (Owner=%q)", w.Owner)
	}

	// The ball should sit AHEAD of the feet along Facing — loose, not glued,
	// not flung absurdly far.
	offset := w.Ball.Pos.Sub(p.Pos)
	ahead := offset.DotXZ(p.Facing)
	dist := offset.LengthXZ()
	if ahead <= 0 {
		t.Errorf("ball should be ahead of player along facing, dot=%v", ahead)
	}
	if dist < 0.2 || dist > 1.5 {
		t.Errorf("ball should be loosely ahead (~0.8m ideal), got dist=%v", dist)
	}

	// Dribbling must be slower than free jogging, and both speeds must match the
	// pure mapping (default movement with no sprint = jog).
	wantDribble := physics.JogSpeed(pace) * physics.DribbleSpeedFactor(drib)
	if dribbleSpeed >= freeSpeed {
		t.Errorf("dribble speed (%v) should be < free-jog speed (%v)", dribbleSpeed, freeSpeed)
	}
	if !approx(freeSpeed, physics.JogSpeed(pace)) {
		t.Errorf("free speed = %v, want JogSpeed(%d) = %v", freeSpeed, pace, physics.JogSpeed(pace))
	}
	if !approx(dribbleSpeed, wantDribble) {
		t.Errorf("dribble speed = %v, want JogSpeed*factor = %v", dribbleSpeed, wantDribble)
	}

	t.Logf("gained possession at tick %d (~%.2fs)", gainedTick, float64(gainedTick)*dt)
	t.Logf("free-jog=%.3f m/s, dribble=%.3f m/s (%.0f%% of jog)",
		freeSpeed, dribbleSpeed, 100*dribbleSpeed/freeSpeed)
	t.Logf("ball %.3fm ahead of feet (total dist %.3fm); player at %+v facing %+v",
		ahead, dist, p.Pos, p.Facing)
}
