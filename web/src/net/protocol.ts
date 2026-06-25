// TypeScript half of the wire contract in proto/messages.md. Kept in sync by
// hand with server/internal/proto/messages.go.

export interface ClientInput {
  seq: number
  moveX: number // -1..1 desired direction on X
  moveZ: number // -1..1 desired direction on Z
  sprint: boolean
}

export interface BallState {
  x: number
  y: number
  z: number
}

export interface PlayerState {
  id: string
  x: number
  z: number
  facingX: number
  facingZ: number
  possessing: boolean
}

export interface Snapshot {
  tick: number
  owner: string
  ball: BallState
  players: PlayerState[]
  touch: number // monotonic dribble-touch count; diff it between snapshots to animate touches
}
