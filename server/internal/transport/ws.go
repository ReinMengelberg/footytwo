// Package transport is the WebSocket edge: it upgrades GET /ws connections,
// binds each to the seeded player, feeds decoded inputs into the room, and pumps
// authoritative snapshots back out. It depends on game + proto + physics; the
// simulation layers know nothing about WebSockets.
package transport

import (
	"context"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"footy/server/internal/game"
	"footy/server/internal/physics"
	"footy/server/internal/proto"
)

// boundPlayer is the player every connection controls (single player, no auth).
const boundPlayer = "p1"

const writeTimeout = 5 * time.Second

// Handler returns the GET /ws handler for the given room.
func Handler(room *game.Room) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			// Allow the Vite dev origin (different port = cross-origin). The
			// server's own host is always authorized. Localhost PoC only.
			OriginPatterns: []string{"localhost:5173", "127.0.0.1:5173"},
		})
		if err != nil {
			return
		}
		defer c.CloseNow()

		ctx := r.Context()
		sub := room.Subscribe()
		defer room.Unsubscribe(sub)

		// One writer goroutine per connection: coder/websocket forbids
		// concurrent writes, so all snapshot sends funnel through here.
		go func() {
			for data := range sub.C {
				wctx, cancel := context.WithTimeout(ctx, writeTimeout)
				err := c.Write(wctx, websocket.MessageText, data)
				cancel()
				if err != nil {
					return
				}
			}
		}()

		// Read loop: decode each ClientInput and store it as the latest input.
		for {
			var in proto.ClientInput
			if err := wsjson.Read(ctx, c, &in); err != nil {
				return
			}
			room.SetInput(boundPlayer, physics.Input{
				Dir:    physics.Vec3{X: in.MoveX, Z: in.MoveZ},
				Sprint: in.Sprint,
			})
		}
	})
}
