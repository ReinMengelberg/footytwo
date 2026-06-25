// TypeScript half of the wire contract in proto/messages.md. Kept in sync by
// hand with server/internal/proto/messages.go.

export interface ClientInput {
  seq: number
  moveX: number // -1..1 desired direction on X
  moveZ: number // -1..1 desired direction on Z
  sprint: boolean
  charge: boolean // hold Space to build kick power; release commits
  lift: number // while charging: +1 loft (up) / -1 driven (down) / 0
  spin: number // while charging: -1 left / +1 right / 0 (curve)
}

export interface BallState {
  x: number
  y: number // height (a lofted ball arcs above the ground)
  z: number
  spin: number // current side-spin, for an optional visual cue
}

export interface PlayerState {
  id: string
  x: number
  z: number
  facingX: number
  facingZ: number
  possessing: boolean // owns the ball (sticky)
  atFeet: boolean // ball at this player's feet this tick (pulses on contact)
  chargePower: number // current kick wind-up, 0..1 (0 = not charging)
  chargeSpin: number // dialled-in curve, -1..1 (sign = direction)
  chargeLift: number // dialled-in loft, -1..1 (+loft / -driven)
}

export interface Snapshot {
  tick: number
  owner: string
  ball: BallState
  players: PlayerState[]
  touch: number // monotonic dribble-touch count; diff it between snapshots to animate touches
  kick: number // monotonic kick count; diff it between snapshots to animate/sfx a kick
}
