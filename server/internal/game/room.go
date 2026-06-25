package game

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"footy/server/internal/physics"
	"footy/server/internal/proto"
)

const (
	tickRate   = 60 // Hz; also the snapshot broadcast rate (naive, fine on localhost)
	sendBuffer = 8  // per-subscriber snapshot queue depth before dropping
)

// Subscriber is one connected client's outbound snapshot queue. The transport
// layer owns the socket; the room only pushes marshaled snapshots onto C.
type Subscriber struct {
	C chan []byte
}

// Room is one match: a physics.World plus its fixed-rate tick loop, the latest
// input per player (last-write-wins), and the set of subscribers it broadcasts
// authoritative snapshots to. The World is touched only by the tick goroutine;
// inputs and subscribers are mutex-guarded for the transport goroutines.
type Room struct {
	ID    string
	World *physics.World

	inputMu sync.Mutex
	inputs  map[string]physics.Input

	subsMu sync.Mutex
	subs   map[*Subscriber]struct{}

	tick int
}

// NewDefaultRoom creates the default room seeded with one tryout player and a
// free ball a couple of meters ahead, so the PoC is immediately controllable.
func NewDefaultRoom() *Room {
	w := physics.NewWorld(physics.DefaultPitch())

	w.AddPlayer(physics.Player{
		ID:   "p1",
		Name: "Tryout",
		Stats: physics.Stats{
			Pace: 78, Shooting: 70, Passing: 80,
			Dribbling: 82, Defending: 65, Physical: 72,
		},
		Pos:    physics.Vec3{X: 0, Y: 0, Z: 0},
		Facing: physics.Vec3{X: 0, Y: 0, Z: 1}, // face +Z by default
	})

	w.PlaceFreeBall(0, 2) // free ball 2m ahead in +Z

	return &Room{
		ID:     "default",
		World:  w,
		inputs: make(map[string]physics.Input),
		subs:   make(map[*Subscriber]struct{}),
	}
}

// SetInput stores a player's latest input (last-write-wins; one pending input
// per player). Called from a transport read loop; never blocks the tick loop.
func (r *Room) SetInput(playerID string, in physics.Input) {
	r.inputMu.Lock()
	r.inputs[playerID] = in
	r.inputMu.Unlock()
}

// Subscribe registers a new client and returns its snapshot queue.
func (r *Room) Subscribe() *Subscriber {
	s := &Subscriber{C: make(chan []byte, sendBuffer)}
	r.subsMu.Lock()
	r.subs[s] = struct{}{}
	r.subsMu.Unlock()
	return s
}

// Unsubscribe removes a client and closes its queue (ending its writer).
func (r *Room) Unsubscribe(s *Subscriber) {
	r.subsMu.Lock()
	if _, ok := r.subs[s]; ok {
		delete(r.subs, s)
		close(s.C)
	}
	r.subsMu.Unlock()
}

// Run drives the fixed-rate tick loop until ctx is cancelled.
func (r *Room) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second / tickRate)
	defer ticker.Stop()
	dt := 1.0 / float64(tickRate)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.step(dt)
		}
	}
}

// step advances the simulation one tick and broadcasts the new snapshot.
func (r *Room) step(dt float64) {
	// Snapshot the latest inputs so the World step doesn't hold the input lock.
	r.inputMu.Lock()
	inputs := make(map[string]physics.Input, len(r.inputs))
	for id, in := range r.inputs {
		inputs[id] = in
	}
	r.inputMu.Unlock()

	r.World.Step(inputs, dt)
	r.tick++

	data, err := json.Marshal(r.snapshot())
	if err != nil {
		return
	}
	r.broadcast(data)
}

// snapshot builds the wire snapshot from the current world state.
func (r *Room) snapshot() proto.Snapshot {
	w := r.World
	players := make([]proto.PlayerState, 0, len(w.Players))
	for id, p := range w.Players {
		players = append(players, proto.PlayerState{
			ID:          id,
			X:           p.Pos.X,
			Z:           p.Pos.Z,
			FacingX:     p.Facing.X,
			FacingZ:     p.Facing.Z,
			Possessing:  w.Owner == id,
			AtFeet:      w.Owner == id && w.AtFeet(),
			ChargePower: p.ChargePower(),
			ChargeSpin:  p.ChargeSpin(),
			ChargeLift:  p.ChargeLift(),
		})
	}
	return proto.Snapshot{
		Tick:    r.tick,
		Owner:   w.Owner,
		Ball:    proto.BallState{X: w.Ball.Pos.X, Y: w.Ball.Pos.Y, Z: w.Ball.Pos.Z, Spin: w.Ball.Spin},
		Players: players,
		Touch:   w.TouchCount(),
		Kick:    w.KickCount(),
	}
}

// broadcast pushes a marshaled snapshot to every subscriber, dropping it for any
// client whose queue is full (naive backpressure; fine on localhost). The lock
// serializes against Unsubscribe so we never send on a closed channel.
func (r *Room) broadcast(data []byte) {
	r.subsMu.Lock()
	for s := range r.subs {
		select {
		case s.C <- data:
		default:
		}
	}
	r.subsMu.Unlock()
}
