// FIFA pitch markings, drawn as thin flat ribbons laid just above the turf.
//
// Lines are real-world sized: 0.12 m wide (the regulation maximum) and all the
// areas/arcs use their fixed FIFA dimensions, so they're correct on any legal
// pitch regardless of the overall size passed in. Built as a single unlit white
// mesh in the XZ plane (y baked to MARK_Y) — one geometry, one draw call.
//
// The markings are also described as a list of primitives (segments, rings,
// discs) so we can expose isMarked(x, z): the pitch uses it to clear grass off
// the lines, otherwise the blades would bury them from the broadcast angle.
//
// Axes match the scene: X is the goal-to-goal length, Z is the touchline width,
// goals sit at x = ±pitchWidth/2.

import { BufferGeometry, DoubleSide, Float32BufferAttribute, Mesh, MeshBasicMaterial } from 'three'

const LINE_W = 0.12 // line width (regulation max 12 cm)
const MARK_Y = 0.03 // lifted just off the ground to avoid z-fighting
const CLEAR_MARGIN = 0.08 // extra grass-free border around each line so it reads

// Fixed FIFA measurements (metres). These never scale with the pitch.
const GOAL_WIDTH = 7.32
const PEN_AREA_DEPTH = 16.5
const PEN_AREA_HALF_W = GOAL_WIDTH / 2 + 16.5 // 20.16: 16.5 out from each post
const GOAL_AREA_DEPTH = 5.5
const GOAL_AREA_HALF_W = GOAL_WIDTH / 2 + 5.5 // 9.16
const PEN_SPOT_DIST = 11 // penalty spot, from the goal line
const CENTER_R = 9.15 // centre circle / penalty arc radius
const CORNER_R = 1.0
const SPOT_R = 0.11 // painted spot dots

const TWO_PI = Math.PI * 2

type Prim =
  | { k: 'seg'; ax: number; az: number; bx: number; bz: number; w: number }
  | { k: 'ring'; cx: number; cz: number; r: number; a0: number; a1: number; w: number }
  | { k: 'disc'; cx: number; cz: number; r: number }

export interface PitchMarkings {
  mesh: Mesh
  /** True if (x, z) lies on (or just beside) a marking — used to clear grass. */
  isMarked: (x: number, z: number) => boolean
  dispose: () => void
}

export function createPitchMarkings(pitchWidth: number, pitchLength: number): PitchMarkings {
  const halfX = pitchWidth / 2 // goal-to-goal half-length
  const halfZ = pitchLength / 2 // touchline half-width

  // --- Describe every marking as a primitive -------------------------------
  const prims: Prim[] = []
  const line = (ax: number, az: number, bx: number, bz: number, w = LINE_W) =>
    prims.push({ k: 'seg', ax, az, bx, bz, w })
  const arc = (cx: number, cz: number, r: number, a0: number, a1: number, w = LINE_W) =>
    prims.push({ k: 'ring', cx, cz, r, a0, a1, w })
  const circle = (cx: number, cz: number, r: number, w = LINE_W) => arc(cx, cz, r, 0, TWO_PI, w)
  const disc = (cx: number, cz: number, r: number) => prims.push({ k: 'disc', cx, cz, r })

  // Boundary: touchlines (long, along X) and goal lines (short, along Z).
  line(-halfX, halfZ, halfX, halfZ)
  line(-halfX, -halfZ, halfX, -halfZ)
  line(halfX, -halfZ, halfX, halfZ)
  line(-halfX, -halfZ, -halfX, halfZ)

  // Halfway line + centre circle + centre spot.
  line(0, -halfZ, 0, halfZ)
  circle(0, 0, CENTER_R)
  disc(0, 0, SPOT_R)

  // Per end (sign = -1 left goal, +1 right goal).
  const phi = Math.acos((PEN_AREA_DEPTH - PEN_SPOT_DIST) / CENTER_R) // half-angle of the visible 'D'
  for (const sign of [-1, 1]) {
    const goalX = sign * halfX
    const penFrontX = sign * (halfX - PEN_AREA_DEPTH)
    const goalFrontX = sign * (halfX - GOAL_AREA_DEPTH)
    const spotX = sign * (halfX - PEN_SPOT_DIST)

    // Penalty area (three lines; the goal line closes the box).
    line(goalX, PEN_AREA_HALF_W, penFrontX, PEN_AREA_HALF_W)
    line(goalX, -PEN_AREA_HALF_W, penFrontX, -PEN_AREA_HALF_W)
    line(penFrontX, -PEN_AREA_HALF_W, penFrontX, PEN_AREA_HALF_W)

    // Goal area (6-yard box).
    line(goalX, GOAL_AREA_HALF_W, goalFrontX, GOAL_AREA_HALF_W)
    line(goalX, -GOAL_AREA_HALF_W, goalFrontX, -GOAL_AREA_HALF_W)
    line(goalFrontX, -GOAL_AREA_HALF_W, goalFrontX, GOAL_AREA_HALF_W)

    // Penalty spot + the 'D' (only the part of the arc outside the box).
    disc(spotX, 0, SPOT_R)
    const base = sign > 0 ? Math.PI : 0 // arc bulges toward the pitch centre
    arc(spotX, 0, CENTER_R, base - phi, base + phi)
  }

  // Corner arcs: quarter circles bulging into the pitch from each corner.
  for (const sx of [-1, 1]) {
    for (const sz of [-1, 1]) {
      // Start angle so the quarter sweeps toward the interior (-sx, -sz).
      let a0: number
      if (sx < 0 && sz < 0) a0 = 0
      else if (sx > 0 && sz < 0) a0 = Math.PI / 2
      else if (sx > 0 && sz > 0) a0 = Math.PI
      else a0 = -Math.PI / 2
      arc(sx * halfX, sz * halfZ, CORNER_R, a0, a0 + Math.PI / 2)
    }
  }

  // --- Build the mesh from those primitives ---------------------------------
  const positions: number[] = []
  const indices: number[] = []
  const quad = (
    ax: number, az: number, bx: number, bz: number,
    cx: number, cz: number, dx: number, dz: number,
  ) => {
    const base = positions.length / 3
    positions.push(ax, 0, az, bx, 0, bz, cx, 0, cz, dx, 0, dz)
    indices.push(base, base + 1, base + 2, base, base + 2, base + 3)
  }

  for (const p of prims) {
    if (p.k === 'seg') {
      const dx = p.bx - p.ax
      const dz = p.bz - p.az
      const len = Math.hypot(dx, dz) || 1
      const ox = (-dz / len) * (p.w / 2)
      const oz = (dx / len) * (p.w / 2)
      quad(p.ax + ox, p.az + oz, p.bx + ox, p.bz + oz, p.bx - ox, p.bz - oz, p.ax - ox, p.az - oz)
    } else if (p.k === 'ring') {
      const ri = p.r - p.w / 2
      const ro = p.r + p.w / 2
      const steps = Math.max(2, Math.ceil((Math.abs(p.a1 - p.a0) / TWO_PI) * 96))
      for (let i = 0; i < steps; i++) {
        const t0 = p.a0 + ((p.a1 - p.a0) * i) / steps
        const t1 = p.a0 + ((p.a1 - p.a0) * (i + 1)) / steps
        const c0 = Math.cos(t0)
        const s0 = Math.sin(t0)
        const c1 = Math.cos(t1)
        const s1 = Math.sin(t1)
        quad(p.cx + ri * c0, p.cz + ri * s0, p.cx + ro * c0, p.cz + ro * s0, p.cx + ro * c1, p.cz + ro * s1, p.cx + ri * c1, p.cz + ri * s1)
      }
    } else {
      const steps = 24
      const center = positions.length / 3
      positions.push(p.cx, 0, p.cz)
      for (let i = 0; i <= steps; i++) {
        const t = (i / steps) * TWO_PI
        positions.push(p.cx + p.r * Math.cos(t), 0, p.cz + p.r * Math.sin(t))
      }
      for (let i = 0; i < steps; i++) indices.push(center, center + 1 + i, center + 2 + i)
    }
  }

  const geometry = new BufferGeometry()
  geometry.setAttribute('position', new Float32BufferAttribute(positions, 3))
  geometry.setIndex(indices)
  const material = new MeshBasicMaterial({ color: 0xffffff, side: DoubleSide })
  const mesh = new Mesh(geometry, material)
  mesh.position.y = MARK_Y

  // --- Grass-clearing predicate over the same primitives --------------------
  const isMarked = (x: number, z: number): boolean => {
    for (const p of prims) {
      if (p.k === 'seg') {
        if (distToSegment(x, z, p.ax, p.az, p.bx, p.bz) <= p.w / 2 + CLEAR_MARGIN) return true
      } else if (p.k === 'ring') {
        const d = Math.hypot(x - p.cx, z - p.cz)
        if (Math.abs(d - p.r) <= p.w / 2 + CLEAR_MARGIN) {
          const span = p.a1 - p.a0
          if (span >= TWO_PI - 1e-6) return true
          const da = (((Math.atan2(z - p.cz, x - p.cx) - p.a0) % TWO_PI) + TWO_PI) % TWO_PI
          if (da <= span) return true
        }
      } else if (Math.hypot(x - p.cx, z - p.cz) <= p.r + CLEAR_MARGIN) {
        return true
      }
    }
    return false
  }

  return {
    mesh,
    isMarked,
    dispose() {
      geometry.dispose()
      material.dispose()
    },
  }
}

function distToSegment(px: number, pz: number, ax: number, az: number, bx: number, bz: number): number {
  const dx = bx - ax
  const dz = bz - az
  const len2 = dx * dx + dz * dz
  const t = len2 === 0 ? 0 : Math.max(0, Math.min(1, ((px - ax) * dx + (pz - az) * dz) / len2))
  return Math.hypot(px - (ax + t * dx), pz - (az + t * dz))
}
