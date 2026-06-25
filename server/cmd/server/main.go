// Command server boots the footy game server.
//
// It seeds the default room (one tryout player + a free ball), starts the room's
// fixed 60Hz tick loop, and serves two endpoints: GET /health and the GET /ws
// WebSocket that carries inputs up and authoritative snapshots down. Standard
// library only apart from the WebSocket library: the match simulation is a
// stateful long-lived process, not request/response, so there's no web framework.
package main

import (
	"context"
	"log"
	"net/http"

	"footy/server/internal/game"
	"footy/server/internal/transport"
)

const addr = ":8080"

func main() {
	room := game.NewDefaultRoom()
	p := room.World.Players["p1"]
	log.Printf("seeded room %q: player %q (%s) pace=%d dribbling=%d at %+v facing %+v; free ball at %+v",
		room.ID, p.ID, p.Name, p.Stats.Pace, p.Stats.Dribbling, p.Pos, p.Facing, room.World.Ball.Pos)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go room.Run(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.Handle("GET /ws", transport.Handler(room))

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// handleHealth is a liveness probe. Returns 200 with a tiny JSON body.
func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}` + "\n"))
}
