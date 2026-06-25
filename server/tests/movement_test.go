package tests

import (
	"math"
	"testing"

	"footy/server/internal/physics"
)

func TestFacingNormalizedAndYZeroAfterMovement(t *testing.T) {
	w := physics.NewWorld(physics.DefaultPitch())
	w.AddPlayer(physics.Player{ID: "p", Facing: physics.Vec3{Z: 1}, Stats: physics.Stats{Pace: 50, Dribbling: 50}})

	// Includes a non-axis direction and one with a Y component to be flattened.
	dirs := []physics.Vec3{{X: 3, Z: 4}, {X: -1}, {Z: -2}, {X: 5, Y: 7, Z: 5}}
	for _, d := range dirs {
		w.Step(map[string]physics.Input{"p": {Dir: d}}, 1.0/60)
		f := w.Players["p"].Facing

		if math.Abs(f.LengthXZ()-1) > 1e-9 {
			t.Errorf("dir %+v: Facing not unit, len=%v", d, f.LengthXZ())
		}
		if f.Y != 0 {
			t.Errorf("dir %+v: Facing.Y = %v, want 0", d, f.Y)
		}
		want := d.NormalizedXZ()
		if math.Abs(f.X-want.X) > 1e-9 || math.Abs(f.Z-want.Z) > 1e-9 {
			t.Errorf("dir %+v: Facing = %+v, want %+v", d, f, want)
		}
	}
}

func TestFacingUnchangedAndStopsWithoutInput(t *testing.T) {
	w := physics.NewWorld(physics.DefaultPitch())
	w.AddPlayer(physics.Player{ID: "p", Facing: physics.Vec3{X: 1}, Stats: physics.Stats{Pace: 50}})

	before := w.Players["p"].Facing
	w.Step(map[string]physics.Input{"p": {Dir: physics.Vec3{}}}, 1.0/60) // no input
	after := w.Players["p"].Facing

	if before != after {
		t.Errorf("Facing changed on no input: before=%+v after=%+v", before, after)
	}
	if v := w.Players["p"].Vel; v != (physics.Vec3{}) {
		t.Errorf("Vel should be zero with no input, got %+v", v)
	}
}

func TestYawConvention(t *testing.T) {
	cases := []struct {
		facing physics.Vec3
		want   float64
	}{
		{physics.Vec3{Z: 1}, 0},             // +Z
		{physics.Vec3{X: 1}, math.Pi / 2},   // +X
		{physics.Vec3{Z: -1}, math.Pi},      // -Z
		{physics.Vec3{X: -1}, -math.Pi / 2}, // -X
	}
	for _, c := range cases {
		if got := (physics.Player{Facing: c.facing}).Yaw(); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("Yaw(%+v) = %v, want %v", c.facing, got, c.want)
		}
	}
}
