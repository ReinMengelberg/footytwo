package tests

import (
	"testing"

	"footy/server/internal/physics"
)

// freeSpeedFor steps a non-possessing player one tick and returns their speed.
func freeSpeedFor(pace int, sprint bool) float64 {
	w := physics.NewWorld(physics.DefaultPitch())
	w.AddPlayer(physics.Player{ID: "p", Facing: physics.Vec3{Z: 1}, Stats: physics.Stats{Pace: pace}})
	w.PlaceFreeBall(20, 0) // far away, so the player never gains possession
	w.Step(map[string]physics.Input{"p": {Dir: physics.Vec3{Z: 1}, Sprint: sprint}}, 1.0/60)
	return w.Players["p"].Vel.LengthXZ()
}

func TestSprintFasterThanJog(t *testing.T) {
	const pace = 78
	jog := freeSpeedFor(pace, false)
	sprint := freeSpeedFor(pace, true)

	if sprint <= jog {
		t.Errorf("sprint (%v) should be faster than jog (%v)", sprint, jog)
	}
	if !approx(jog, physics.JogSpeed(pace)) {
		t.Errorf("jog speed = %v, want JogSpeed(%d) = %v", jog, pace, physics.JogSpeed(pace))
	}
	if !approx(sprint, physics.SprintSpeed(pace)) {
		t.Errorf("sprint speed = %v, want SprintSpeed(%d) = %v", sprint, pace, physics.SprintSpeed(pace))
	}
}

func TestDiagonalNotFasterThanStraight(t *testing.T) {
	const pace = 78
	straight := freeSpeedFor(pace, false) // moving +Z only

	w := physics.NewWorld(physics.DefaultPitch())
	w.AddPlayer(physics.Player{ID: "p", Facing: physics.Vec3{Z: 1}, Stats: physics.Stats{Pace: pace}})
	w.PlaceFreeBall(20, 0)
	// Raw diagonal input (magnitude √2); the server must normalize it.
	w.Step(map[string]physics.Input{"p": {Dir: physics.Vec3{X: 1, Z: 1}}}, 1.0/60)
	diagonal := w.Players["p"].Vel.LengthXZ()

	if !approx(diagonal, straight) {
		t.Errorf("diagonal speed (%v) should equal straight speed (%v)", diagonal, straight)
	}
}

// A sprint touch is heavier than a jog touch (it knocks the ball further, which
// is why it gets away from you — see TestSprintTouchLosesPossession).
func TestSprintTouchHeavierThanJog(t *testing.T) {
	const speed = 5.0
	jog := physics.TouchSpeed(speed, 82, false)
	sprint := physics.TouchSpeed(speed, 82, true)
	if sprint <= jog {
		t.Errorf("a sprint touch should be heavier than a jog touch: jog=%.2f sprint=%.2f", jog, sprint)
	}
}

// Better dribblers take a softer (tighter) touch at the same speed, so they keep
// the ball closer — this is what makes them harder to dispossess.
func TestHigherDribblingSofterTouch(t *testing.T) {
	const speed = 5.0
	high := physics.TouchSpeed(speed, 95, true)
	low := physics.TouchSpeed(speed, 20, true)
	if high >= low {
		t.Errorf("higher dribbling should take a softer touch: high(95)=%.2f low(20)=%.2f", high, low)
	}
}
