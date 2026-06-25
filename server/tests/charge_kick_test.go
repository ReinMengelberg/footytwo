package tests

import (
	"math"
	"testing"

	"footy/server/internal/physics"
)

const kdt = 1.0 / 60

// chargeAndRelease holds Space for holdTicks (with the given arrow effects) while
// running +Z, then releases Space and keeps running +Z until the queued kick fires
// (Owner becomes "") or a cap elapses. It returns the ball's velocity on the firing
// tick (the raw launch velocity, before any free-flight integration) and whether it
// fired. The seeded player faces +Z, so a kick travels +Z.
func chargeAndRelease(t *testing.T, w *physics.World, lift, spin, holdTicks int) (physics.Vec3, bool) {
	t.Helper()
	charge := map[string]physics.Input{"p1": {Dir: physics.Vec3{Z: 1}, Charge: true, Lift: lift, Spin: spin}}
	for i := 0; i < holdTicks; i++ {
		w.Step(charge, kdt)
	}
	release := map[string]physics.Input{"p1": {Dir: physics.Vec3{Z: 1}}}
	for i := 0; i < 120; i++ {
		w.Step(release, kdt)
		if w.Owner == "" {
			return w.Ball.Vel, true // launch velocity: free physics hasn't run yet this tick
		}
	}
	return physics.Vec3{}, false
}

// freeStep advances the world with no player input (the ball flies free).
func freeStep(w *physics.World) { w.Step(map[string]physics.Input{}, kdt) }

// TestChargeReleaseCommitsKickOnContact: charging and releasing fires exactly one
// kick (on the next contact) and releases possession.
func TestChargeReleaseCommitsKickOnContact(t *testing.T) {
	w, _ := newDribblingWorld(t)
	before := w.KickCount()

	if _, fired := chargeAndRelease(t, w, 0, 0, 18); !fired {
		t.Fatalf("kick never fired after charge+release")
	}
	if w.Owner != "" {
		t.Errorf("possession should be released by the kick, Owner=%q", w.Owner)
	}
	if got := w.KickCount() - before; got != 1 {
		t.Errorf("expected exactly one kick, got %d", got)
	}
}

// TestNeutralKickTravelsFlatAlongFacing: a no-arrow kick is a flat ground pass in
// the facing direction and never leaves the ground.
func TestNeutralKickTravelsFlatAlongFacing(t *testing.T) {
	w, _ := newDribblingWorld(t)
	vel, fired := chargeAndRelease(t, w, 0, 0, 18)
	if !fired {
		t.Fatalf("kick never fired")
	}
	if vel.Y != 0 {
		t.Errorf("neutral kick should be flat, got Vel.Y=%.2f", vel.Y)
	}
	if vel.Z <= 0 {
		t.Errorf("kick should travel +Z (facing), got Vel.Z=%.2f", vel.Z)
	}
	if math.Abs(vel.X) > 0.5 {
		t.Errorf("kick should be roughly straight, got Vel.X=%.2f", vel.X)
	}
	maxY := w.Ball.Pos.Y
	for i := 0; i < 90; i++ {
		freeStep(w)
		maxY = math.Max(maxY, w.Ball.Pos.Y)
	}
	if maxY > physics.BallRadius+0.02 {
		t.Errorf("neutral kick left the ground (maxY=%.3f, ground=%.3f)", maxY, physics.BallRadius)
	}
}

// TestLoftedKickLeavesGroundAndBounces: holding Up lofts the ball into a real arc
// that then bounces (a second airborne phase) before settling.
func TestLoftedKickLeavesGroundAndBounces(t *testing.T) {
	w, _ := newDribblingWorld(t)
	vel, fired := chargeAndRelease(t, w, +1, 0, 42) // hold Up the whole charge → full loft
	if !fired {
		t.Fatalf("kick never fired")
	}
	if vel.Y <= 0 {
		t.Errorf("lofted kick should launch upward, got Vel.Y=%.2f", vel.Y)
	}

	maxY := w.Ball.Pos.Y
	landed := -1
	reboundY := physics.BallRadius
	for i := 0; i < 300; i++ { // 5s: up, down, bounce
		freeStep(w)
		y := w.Ball.Pos.Y
		maxY = math.Max(maxY, y)
		if landed < 0 && i > 5 && maxY > 0.5 && y <= physics.BallRadius+0.02 {
			landed = i // first touchdown after being airborne
		}
		if landed >= 0 && i > landed+2 {
			reboundY = math.Max(reboundY, y)
		}
	}
	if maxY < 0.5 {
		t.Errorf("lofted ball never got airborne (maxY=%.2f)", maxY)
	}
	if landed < 0 {
		t.Fatalf("lofted ball never landed (maxY=%.2f)", maxY)
	}
	if reboundY < physics.BallRadius+0.15 {
		t.Errorf("ball didn't bounce after landing (reboundY=%.3f)", reboundY)
	}
}

// TestDrivenKickStaysLowAndFasterThanNeutral: holding Down makes a flat ball that
// stays on the deck and carries more pace than a neutral pass at the same charge.
func TestDrivenKickStaysLowAndFasterThanNeutral(t *testing.T) {
	wn, _ := newDribblingWorld(t)
	neutral, okN := chargeAndRelease(t, wn, 0, 0, 42)
	wd, _ := newDribblingWorld(t)
	driven, okD := chargeAndRelease(t, wd, -1, 0, 42)
	if !okN || !okD {
		t.Fatalf("a kick failed to fire (neutral=%v driven=%v)", okN, okD)
	}

	if driven.LengthXZ() <= neutral.LengthXZ() {
		t.Errorf("driven kick should be faster: driven=%.2f neutral=%.2f", driven.LengthXZ(), neutral.LengthXZ())
	}
	if driven.Y != 0 {
		t.Errorf("driven kick should be flat, got Vel.Y=%.2f", driven.Y)
	}
	for i := 0; i < 120; i++ {
		freeStep(wd)
		if wd.Ball.Pos.Y > physics.BallRadius+0.02 {
			t.Fatalf("driven ball left the ground at tick %d (Y=%.3f)", i, wd.Ball.Pos.Y)
		}
	}
}

// TestSpinCurvesOppositeDirections: left vs right spin bend the ball to opposite
// sides. This pins the Magnus sign convention (+spin curves a +Z ball toward -X).
func TestSpinCurvesOppositeDirections(t *testing.T) {
	wr, _ := newDribblingWorld(t)
	if _, ok := chargeAndRelease(t, wr, 0, +1, 42); !ok {
		t.Fatalf("right-spin kick never fired")
	}
	wl, _ := newDribblingWorld(t)
	if _, ok := chargeAndRelease(t, wl, 0, -1, 42); !ok {
		t.Fatalf("left-spin kick never fired")
	}
	for i := 0; i < 60; i++ {
		freeStep(wr)
		freeStep(wl)
	}
	xr, xl := wr.Ball.Pos.X, wl.Ball.Pos.X
	if !(xr < -0.1 && xl > 0.1) {
		t.Errorf("spin should curve opposite ways (+spin→-X): xRight=%.2f xLeft=%.2f", xr, xl)
	}
}

// TestAirborneBallNotCorralled: a ball in flight overhead can't be gained; once it
// settles on the ground at the feet it can.
func TestAirborneBallNotCorralled(t *testing.T) {
	w := physics.NewWorld(physics.DefaultPitch())
	w.AddPlayer(physics.Player{ID: "p1", Facing: physics.Vec3{Z: 1}, Stats: physics.Stats{Pace: 70, Dribbling: 70}})

	w.Ball = physics.Ball{Pos: physics.Vec3{X: 0, Y: 2, Z: 0}} // directly above the player, airborne
	w.Step(map[string]physics.Input{}, kdt)
	if w.Owner != "" {
		t.Fatalf("corralled a ball flying overhead, Owner=%q", w.Owner)
	}

	w.Ball = physics.Ball{Pos: physics.Vec3{X: 0, Y: physics.BallRadius, Z: 0}} // settled at the feet
	w.Step(map[string]physics.Input{}, kdt)
	if w.Owner != "p1" {
		t.Errorf("should corral a grounded ball at the feet, Owner=%q", w.Owner)
	}
}

// TestFirstTimeKickOnLooseBall: charging WITHOUT the ball and then reaching a loose
// ball strikes it first-time (a kick fires, possession is not taken).
func TestFirstTimeKickOnLooseBall(t *testing.T) {
	w := physics.NewWorld(physics.DefaultPitch())
	w.AddPlayer(physics.Player{ID: "p1", Facing: physics.Vec3{Z: 1}, Stats: physics.Stats{Pace: 78, Dribbling: 82}})
	w.PlaceFreeBall(0, 2) // 2m ahead, free
	before := w.KickCount()

	// Approach the loose ball while charging, then release before reaching it.
	chargeRun := map[string]physics.Input{"p1": {Dir: physics.Vec3{Z: 1}, Charge: true}}
	for i := 0; i < 6; i++ {
		w.Step(chargeRun, kdt)
	}
	run := map[string]physics.Input{"p1": {Dir: physics.Vec3{Z: 1}}}
	fired := false
	for i := 0; i < 120; i++ {
		w.Step(run, kdt)
		if w.KickCount() > before {
			fired = true
			break
		}
	}
	if !fired {
		t.Fatalf("first-time kick never fired on reaching the loose ball")
	}
	if w.Owner != "" {
		t.Errorf("a first-time strike should not take possession, Owner=%q", w.Owner)
	}
	if w.Ball.Vel.LengthXZ() < 1 {
		t.Errorf("struck ball should carry pace, got %.2f m/s", w.Ball.Vel.LengthXZ())
	}
}
