// Package game is the match/room layer: it wraps a physics.World with identity
// (room ID) and, in later phases, the fixed 60Hz tick loop, a room registry
// keyed by ID, and the WebSocket fan-out. It imports physics, never the reverse.
package game
