// Package physics holds the hand-rolled simulation: vectors, the player/ball
// model, the stat→physics mapping, and the per-tick world Step (movement,
// possession, dribbling). No third-party physics engine is used anywhere.
//
// It must not import the game/room layer — game imports physics, not the
// reverse. This phase wires only Pace and Dribbling into motion; the other
// stats are defined but unused.
package physics
