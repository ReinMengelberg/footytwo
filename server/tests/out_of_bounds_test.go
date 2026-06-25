package tests

import (
	"math"
	"testing"

	"footy/server/internal/physics"
)

// touchedWorld returns a world whose ball has just been touched by p1 (on the
// given team), so lastTouch is set — which is what tells a corner apart from a
// goal kick. The ball ends up corralled at the origin; callers reposition it.
func touchedWorld(t *testing.T, team physics.Team) *physics.World {
	t.Helper()
	w := physics.NewWorld(physics.DefaultPitch())
	w.AddPlayer(physics.Player{
		ID: "p1", Team: team,
		Facing: physics.Vec3{Z: 1},
		Stats:  physics.Stats{Pace: 70, Dribbling: 70},
	})
	w.PlaceFreeBall(0, 0) // on the player → corralled on the first step, recording the touch
	w.Step(map[string]physics.Input{}, kdt)
	if w.LastTouch() != "p1" {
		t.Fatalf("setup: expected p1 to have touched the ball, got %q", w.LastTouch())
	}
	return w
}

// runOutPause ticks ResolveRestart long enough to outlast even the goal
// celebration, so the dead-ball pause ends and the ball is placed at its spot.
func runOutPause(w *physics.World) {
	for i := 0; i < 300; i++ { // 5s > goalCelebration
		w.ResolveRestart(kdt)
	}
}

// TestThrowInOverTouchline: a ball fully over a touchline is a throw-in, restarted
// on the line at the point it crossed, with possession released.
func TestThrowInOverTouchline(t *testing.T) {
	w := touchedWorld(t, physics.TeamHome)
	w.Ball.Pos = physics.Vec3{X: 12, Y: physics.BallRadius, Z: 41} // past the +Z touchline (40)

	if !w.ResolveRestart(kdt) {
		t.Fatalf("a ball over the touchline should be out of play")
	}
	if w.LastRestart() != physics.RestartThrowIn {
		t.Errorf("want throw-in, got %v", w.LastRestart())
	}
	if w.RestartCount() != 1 {
		t.Errorf("the restart should be announced immediately, count=%d", w.RestartCount())
	}
	if w.Owner != "" {
		t.Errorf("going out should release possession, Owner=%q", w.Owner)
	}
	// During the pause the ball is still out of play, not yet placed on the line.
	if w.Ball.Pos.Z != 41 {
		t.Errorf("ball should still be out during the pause, Z=%.2f", w.Ball.Pos.Z)
	}

	runOutPause(w)
	if got := w.Ball.Pos; got.Z != 40 || math.Abs(got.X-12) > 1e-9 {
		t.Errorf("throw-in should restart on the touchline at the crossing X, got %+v", got)
	}
}

// TestRestartPauseHoldsThenPlaces pins the dead-ball timing: the ball stays out a
// short while (so the moment is visible) before it is placed at the restart spot.
func TestRestartPauseHoldsThenPlaces(t *testing.T) {
	w := touchedWorld(t, physics.TeamHome)
	w.Ball.Pos = physics.Vec3{X: 0, Y: physics.BallRadius, Z: 41}
	w.ResolveRestart(kdt) // detect → open the pause

	for i := 0; i < 20; i++ { // ~0.33s, inside the throw-in pause
		w.ResolveRestart(kdt)
	}
	if w.Ball.Pos.Z != 41 {
		t.Errorf("ball placed early (Z=%.2f) — it should still be out during the pause", w.Ball.Pos.Z)
	}
	for i := 0; i < 60; i++ { // +1s, now well past the pause
		w.ResolveRestart(kdt)
	}
	if w.Ball.Pos.Z != 40 {
		t.Errorf("ball never restarted after the pause (Z=%.2f)", w.Ball.Pos.Z)
	}
}

// TestGoalScoresAndDwellsLongerThanThrowIn: a ball through the mouth under the bar
// is a goal that credits the attacking side, and its celebration pause is LONGER
// than an ordinary restart before the ball kicks off from the centre spot.
func TestGoalScoresAndDwellsLongerThanThrowIn(t *testing.T) {
	w := touchedWorld(t, physics.TeamHome) // Home attacks +X
	h0, a0 := w.Score()
	w.Ball.Pos = physics.Vec3{X: 61, Y: 1.0, Z: 0} // through the +X mouth, under the bar

	if !w.ResolveRestart(kdt) {
		t.Fatalf("a ball over the goal line should be out of play")
	}
	if w.LastRestart() != physics.RestartGoal {
		t.Errorf("want goal, got %v", w.LastRestart())
	}
	if h, a := w.Score(); h != h0+1 || a != a0 {
		t.Errorf("Home should score: %d-%d became %d-%d", h0, a0, h, a)
	}

	// A throw-in would already be restarted after restartPause; the goal is not —
	// the ball is still out at the +X end, not yet kicked off from the centre.
	for i := 0; i < 60; i++ { // 1.0s > restartPause(0.8), < goalCelebration(2.5)
		w.ResolveRestart(kdt)
	}
	if w.Ball.Pos.X < w.Bounds.HalfX {
		t.Errorf("goal celebration ended too early: ball already back at X=%.2f", w.Ball.Pos.X)
	}
	// After the celebration the ball kicks off from the centre spot.
	for i := 0; i < 120; i++ { // +2s, total 3s > goalCelebration
		w.ResolveRestart(kdt)
	}
	if want := (physics.Vec3{X: 0, Y: physics.BallRadius, Z: 0}); w.Ball.Pos != want {
		t.Errorf("goal should kick off from the centre spot, got %+v", w.Ball.Pos)
	}
}

// TestGoalKickWhenAttackerPutsItOut: an attacker putting the ball over the far
// goal line (wide of the post) concedes a goal kick, taken from the defending box.
func TestGoalKickWhenAttackerPutsItOut(t *testing.T) {
	w := touchedWorld(t, physics.TeamHome)                         // Home attacks +X
	w.Ball.Pos = physics.Vec3{X: 61, Y: physics.BallRadius, Z: 10} // over the +X line, wide of the post

	w.ResolveRestart(kdt)
	if w.LastRestart() != physics.RestartGoalKick {
		t.Errorf("attacker over the goal line → goal kick, got %v", w.LastRestart())
	}
	runOutPause(w)
	if w.Ball.Pos.X <= 0 {
		t.Errorf("goal kick should restart at the +X end, X=%.2f", w.Ball.Pos.X)
	}
	if want := 60 - 5.5; math.Abs(w.Ball.Pos.X-want) > 1e-9 {
		t.Errorf("goal kick X=%.2f, want the 6-yard box line at %.2f", w.Ball.Pos.X, want)
	}
}

// TestCornerWhenDefenderPutsItOut: a defender putting the ball over their own goal
// line concedes a corner, taken from the nearest flag.
func TestCornerWhenDefenderPutsItOut(t *testing.T) {
	w := touchedWorld(t, physics.TeamHome)                          // Home defends -X
	w.Ball.Pos = physics.Vec3{X: -61, Y: physics.BallRadius, Z: 10} // over the -X line, wide

	w.ResolveRestart(kdt)
	if w.LastRestart() != physics.RestartCorner {
		t.Errorf("defender over their own goal line → corner, got %v", w.LastRestart())
	}
	runOutPause(w)
	if want := (physics.Vec3{X: -60, Y: physics.BallRadius, Z: 40}); w.Ball.Pos != want {
		t.Errorf("corner should restart at the nearest flag, got %+v want %+v", w.Ball.Pos, want)
	}
}

// TestRestartFollowsTeam: the SAME exit is a corner or a goal kick depending on
// the last toucher's team — the regression that the call is team-aware, not fixed
// to a side. Home is an attacker at the +X goal (goal kick); Away defends it (corner).
func TestRestartFollowsTeam(t *testing.T) {
	out := physics.Vec3{X: 61, Y: physics.BallRadius, Z: 8} // over the +X goal line, wide

	home := touchedWorld(t, physics.TeamHome)
	home.Ball.Pos = out
	home.ResolveRestart(kdt)
	if home.LastRestart() != physics.RestartGoalKick {
		t.Errorf("Home attacks +X: out there should be a goal kick, got %v", home.LastRestart())
	}

	away := touchedWorld(t, physics.TeamAway)
	away.Ball.Pos = out
	away.ResolveRestart(kdt)
	if away.LastRestart() != physics.RestartCorner {
		t.Errorf("Away defends +X: out there should be a corner, got %v", away.LastRestart())
	}
}

// TestInBoundsBallNeverRestarts: a ball inside the lines — even one whose body has
// only partly crossed — stays in play.
func TestInBoundsBallNeverRestarts(t *testing.T) {
	w := touchedWorld(t, physics.TeamHome)

	w.Ball.Pos = physics.Vec3{X: 59.9, Y: physics.BallRadius, Z: -39.9} // clearly inside
	if w.ResolveRestart(kdt) {
		t.Errorf("a ball inside the lines should not restart")
	}
	w.Ball.Pos = physics.Vec3{X: 60.05, Y: physics.BallRadius, Z: 0} // over the line, but < a full radius
	if w.ResolveRestart(kdt) {
		t.Errorf("a ball only partly over the line is still in play")
	}
	if w.RestartCount() != 0 {
		t.Errorf("no restart should have been announced, count=%d", w.RestartCount())
	}
}

// TestDribbleOutTriggersThrowIn runs the full room loop (Step then ResolveRestart):
// dribbling the ball over a touchline puts it out, releases possession, and — once
// the pause elapses — restarts it back on the field.
func TestDribbleOutTriggersThrowIn(t *testing.T) {
	w := physics.NewWorld(physics.DefaultPitch())
	w.AddPlayer(physics.Player{
		ID: "p1", Team: physics.TeamHome,
		Pos:    physics.Vec3{Z: 38}, // near the +Z touchline
		Facing: physics.Vec3{Z: 1},
		Stats:  physics.Stats{Pace: 78, Dribbling: 82},
	})
	w.PlaceFreeBall(0, 39) // just ahead of the player

	drive := map[string]physics.Input{"p1": {Dir: physics.Vec3{Z: 1}}}
	out := false
	for i := 0; i < 1200; i++ {
		w.Step(drive, kdt)
		w.ResolveRestart(kdt)
		if w.RestartCount() > 0 {
			out = true
			break
		}
	}
	if !out {
		t.Fatalf("dribbling at the touchline never put the ball out")
	}
	if w.LastRestart() != physics.RestartThrowIn {
		t.Errorf("dribbled out over the touchline → throw-in, got %v", w.LastRestart())
	}
	if w.Owner != "" {
		t.Errorf("going out should release possession, Owner=%q", w.Owner)
	}

	// Let the pause run out (no input) and confirm the ball comes back in bounds.
	for i := 0; i < 120; i++ {
		w.Step(map[string]physics.Input{}, kdt)
		w.ResolveRestart(kdt)
	}
	if math.Abs(w.Ball.Pos.Z) > w.Bounds.HalfZ {
		t.Errorf("throw-in should restart the ball in bounds, Z=%.2f", w.Ball.Pos.Z)
	}
	if w.RestartCount() != 1 {
		t.Errorf("only one restart expected, got %d", w.RestartCount())
	}
}

// TestDeadBallRollsFurtherOut: once it's out, the ball keeps its momentum and rolls
// ON past the line during the pause (so the exit is visible), but is kept on the
// rendered field — it never sails off past the turf margin.
func TestDeadBallRollsFurtherOut(t *testing.T) {
	w := touchedWorld(t, physics.TeamHome)
	w.Owner = ""
	// A ball rolling out over the +X goal line, wide of the post (a goal kick).
	w.Ball.Pos = physics.Vec3{X: 59, Y: physics.BallRadius, Z: 20}
	w.Ball.Vel = physics.Vec3{X: 12} // rolling toward +X

	crossedAt := 0.0
	out := false
	for i := 0; i < 120; i++ {
		w.Step(map[string]physics.Input{}, kdt)
		if w.ResolveRestart(kdt) {
			crossedAt = w.Ball.Pos.X
			out = true
			break
		}
	}
	if !out {
		t.Fatalf("ball never rolled out over the goal line")
	}

	// A little way into the pause the ball has rolled FURTHER past the line...
	for i := 0; i < 20; i++ {
		w.Step(map[string]physics.Input{}, kdt)
		w.ResolveRestart(kdt)
	}
	if w.Ball.Pos.X <= crossedAt {
		t.Errorf("dead ball should roll further out: crossed at X=%.2f, now X=%.2f", crossedAt, w.Ball.Pos.X)
	}
	// ...but never off the visible turf (clamped within the out-of-bounds margin).
	if limit := w.Bounds.HalfX + 5.0 + 1e-6; w.Ball.Pos.X > limit {
		t.Errorf("dead ball ran off the field: X=%.2f exceeds the turf margin (%.2f)", w.Ball.Pos.X, limit)
	}
}
