package tests

import (
	"testing"

	"footy/server/internal/game"
	"footy/server/internal/physics"
)

// TestNewDefaultRoomSeed verifies the startup seed spanning game + physics: the
// default room holds the exact tryout player and a free ball 2m ahead.
func TestNewDefaultRoomSeed(t *testing.T) {
	room := game.NewDefaultRoom()

	if room.ID != "default" {
		t.Errorf("room ID = %q, want %q", room.ID, "default")
	}

	p, ok := room.World.Players["p1"]
	if !ok {
		t.Fatalf("expected seeded player %q", "p1")
	}
	if p.Name != "Tryout" {
		t.Errorf("player name = %q, want %q", p.Name, "Tryout")
	}

	wantStats := physics.Stats{Pace: 78, Shooting: 70, Passing: 80, Dribbling: 82, Defending: 65, Physical: 72}
	if p.Stats != wantStats {
		t.Errorf("stats = %+v, want %+v", p.Stats, wantStats)
	}
	if want := (physics.Vec3{Z: 1}); p.Facing != want {
		t.Errorf("facing = %+v, want %+v (+Z)", p.Facing, want)
	}

	if room.World.Owner != "" {
		t.Errorf("expected a free ball at seed, Owner=%q", room.World.Owner)
	}
	wantBall := physics.Vec3{X: 0, Y: physics.BallRadius, Z: 2}
	if room.World.Ball.Pos != wantBall {
		t.Errorf("ball pos = %+v, want %+v", room.World.Ball.Pos, wantBall)
	}
}
