package tests

import (
	"math"
	"testing"

	"footy/server/internal/physics"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// Spec-pinned design values. The physics package keeps these as internal tuning
// constants; from outside we assert against the documented numbers in the phase
// spec so the public mapping stays honest to it.
const (
	specMinRunSpeed = 4.5
	specMaxRunSpeed = 8.5
	specCorralCap   = 6.0 // a ball must be slower than this (m/s) to be corralled
)

func TestRunSpeedMapping(t *testing.T) {
	if got := physics.RunSpeed(99); !approx(got, specMaxRunSpeed) {
		t.Errorf("RunSpeed(99) = %v, want max %v", got, specMaxRunSpeed)
	}
	if got := physics.RunSpeed(0); !approx(got, specMinRunSpeed) {
		t.Errorf("RunSpeed(0) = %v, want min %v", got, specMinRunSpeed)
	}
	// Out-of-range pace is clamped defensively.
	if got := physics.RunSpeed(150); !approx(got, specMaxRunSpeed) {
		t.Errorf("RunSpeed(150) clamped = %v, want max %v", got, specMaxRunSpeed)
	}
	// Mid value sits strictly between min and max.
	if mid := physics.RunSpeed(50); mid <= specMinRunSpeed || mid >= specMaxRunSpeed {
		t.Errorf("RunSpeed(50) = %v, want within (%v, %v)", mid, specMinRunSpeed, specMaxRunSpeed)
	}
}

func TestDribbleSpeedPenalty(t *testing.T) {
	if got := physics.DribbleSpeedFactor(99); !approx(got, 1.0) {
		t.Errorf("DribbleSpeedFactor(99) = %v, want 1.0", got)
	}
	if got := physics.DribbleSpeedFactor(0); !approx(got, 0.55) {
		t.Errorf("DribbleSpeedFactor(0) = %v, want 0.55", got)
	}
	if got := physics.DribbleSpeedFactor(50); math.Abs(got-0.78) > 0.01 {
		t.Errorf("DribbleSpeedFactor(50) = %v, want ~0.78", got)
	}
	// Dribbling 99 keeps full pace; the dribble speed equals the run speed.
	if got := physics.DribbleSpeed(99, 99); !approx(got, physics.RunSpeed(99)) {
		t.Errorf("DribbleSpeed(99,99) = %v, want full run speed %v", got, physics.RunSpeed(99))
	}
	// Anything below 99 dribbling must be slower than free running.
	if physics.DribbleSpeed(78, 82) >= physics.RunSpeed(78) {
		t.Errorf("DribbleSpeed(78,82)=%v should be < RunSpeed(78)=%v",
			physics.DribbleSpeed(78, 82), physics.RunSpeed(78))
	}
}

func TestControlStrengthMapping(t *testing.T) {
	if low, high := physics.ControlStrength(20), physics.ControlStrength(90); high <= low {
		t.Errorf("ControlStrength should rise with dribbling: 20→%v, 90→%v", low, high)
	}
	if got := physics.ControlStrength(0); got <= 0 {
		t.Errorf("ControlStrength(0) = %v, want positive", got)
	}
}

func TestStatsClampOnConstruction(t *testing.T) {
	got := physics.Stats{Pace: 150, Shooting: -5, Passing: 99, Dribbling: 0, Defending: 100, Physical: 50}.Clamped()
	want := physics.Stats{Pace: 99, Shooting: 0, Passing: 99, Dribbling: 0, Defending: 99, Physical: 50}
	if got != want {
		t.Errorf("Clamped() = %+v, want %+v", got, want)
	}
}
