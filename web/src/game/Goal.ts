// A goal: white posts + crossbar with a slanted net behind them.
//
// Built entirely in code — no sprite/asset needed. The frame is three cylinders
// and the net is four panels (top, back, two sides) drawn as a grid of white
// LINE SEGMENTS. (An earlier version textured the panels with an alpha-tested
// grid, but the thin lines were averaged below the alphaTest cutoff by mipmap
// minification at any real camera distance, so the net vanished. Lines always
// rasterize at a ~1px minimum width, so they stay visible from far away.)
//
// Geometry is laid out in a LOCAL frame: the goal line is at local x=0 and the
// net trails toward local +X. The caller positions/rotates the returned group
// to place each goal at its end of the pitch.

import {
  BufferGeometry,
  CylinderGeometry,
  Float32BufferAttribute,
  Group,
  LineBasicMaterial,
  LineSegments,
  Mesh,
  MeshStandardMaterial,
} from 'three'

// Regulation-ish dimensions; the scene is roughly metres (a player is ~1.8 tall).
const GOAL_WIDTH = 7.32
const GOAL_HEIGHT = 2.44
const POST_RADIUS = 0.06
const NET_TOP_DEPTH = 0.9 // how far the net's top edge trails behind the line
const NET_BOTTOM_DEPTH = 2.0 // how far the net's base trails behind the line
const NET_CELL = 0.16 // net mesh size in world units (drives the grid tiling)

export interface Goal {
  group: Group
  dispose: () => void
}

export function createGoal(): Goal {
  const group = new Group()
  const halfW = GOAL_WIDTH / 2

  // Frame: two uprights and a crossbar. The post geometry is shared.
  const postGeometry = new CylinderGeometry(POST_RADIUS, POST_RADIUS, GOAL_HEIGHT, 12)
  const barGeometry = new CylinderGeometry(POST_RADIUS, POST_RADIUS, GOAL_WIDTH, 12)
  const frameMaterial = new MeshStandardMaterial({ color: 0xffffff, roughness: 0.4, metalness: 0.1 })

  const leftPost = new Mesh(postGeometry, frameMaterial)
  leftPost.position.set(0, GOAL_HEIGHT / 2, -halfW)
  const rightPost = new Mesh(postGeometry, frameMaterial)
  rightPost.position.set(0, GOAL_HEIGHT / 2, halfW)
  const crossbar = new Mesh(barGeometry, frameMaterial)
  crossbar.rotation.x = Math.PI / 2 // lay the cylinder along Z
  crossbar.position.set(0, GOAL_HEIGHT, 0)
  group.add(leftPost, rightPost, crossbar)

  // Net: a white wire grid. Slightly translucent so it reads as mesh, not a wall.
  const netGeometry = createNetGeometry()
  const netMaterial = new LineBasicMaterial({
    color: 0xffffff,
    transparent: true,
    opacity: 0.75,
  })
  const net = new LineSegments(netGeometry, netMaterial)
  group.add(net)

  return {
    group,
    dispose() {
      postGeometry.dispose()
      barGeometry.dispose()
      frameMaterial.dispose()
      netGeometry.dispose()
      netMaterial.dispose()
    },
  }
}

// Four panels forming the net box: a flat top trailing back from the crossbar, a
// back panel slanting down to the ground, and a side on each post. Each panel is
// filled with a grid of line segments spaced ~NET_CELL apart in both directions,
// so the holes read as a mesh.
function createNetGeometry(): BufferGeometry {
  const hw = GOAL_WIDTH / 2
  const h = GOAL_HEIGHT
  const top = NET_TOP_DEPTH
  const bot = NET_BOTTOM_DEPTH
  const slant = Math.hypot(bot - top, h)

  const positions: number[] = []

  // Fill the (bilinear) quad a→b→c→d with a line-segment grid. a→b is the "u"
  // edge (uLen long) and b→c the "v" edge (vLen long); cells are ~NET_CELL
  // square. Lines run both directions and are emitted as segment pairs for
  // LineSegments. The quad may be a trapezoid (the sides are) — bilinear
  // interpolation keeps the rows/columns evenly spaced across it.
  const panel = (
    ax: number, ay: number, az: number,
    bx: number, by: number, bz: number,
    cx: number, cy: number, cz: number,
    dx: number, dy: number, dz: number,
    uLen: number, vLen: number,
  ) => {
    const nu = Math.max(1, Math.round(uLen / NET_CELL))
    const nv = Math.max(1, Math.round(vLen / NET_CELL))
    const at = (i: number, j: number): [number, number, number] => {
      const u = i / nu
      const v = j / nv
      return [
        ax * (1 - u) * (1 - v) + bx * u * (1 - v) + cx * u * v + dx * (1 - u) * v,
        ay * (1 - u) * (1 - v) + by * u * (1 - v) + cy * u * v + dy * (1 - u) * v,
        az * (1 - u) * (1 - v) + bz * u * (1 - v) + cz * u * v + dz * (1 - u) * v,
      ]
    }
    // Lines of constant v (running along u).
    for (let j = 0; j <= nv; j++) {
      for (let i = 0; i < nu; i++) positions.push(...at(i, j), ...at(i + 1, j))
    }
    // Lines of constant u (running along v).
    for (let i = 0; i <= nu; i++) {
      for (let j = 0; j < nv; j++) positions.push(...at(i, j), ...at(i, j + 1))
    }
  }

  // Top: crossbar (x=0) back to the top-rear edge (x=top), at y=h.
  panel(0, h, -hw, 0, h, hw, top, h, hw, top, h, -hw, GOAL_WIDTH, top)
  // Back: top-rear edge down to the bottom-rear edge (x=bot, y=0).
  panel(top, h, -hw, top, h, hw, bot, 0, hw, bot, 0, -hw, GOAL_WIDTH, slant)
  // Left side (z=-hw) and right side (z=+hw): front-top → rear-top → rear-bottom → front-bottom.
  panel(0, h, -hw, top, h, -hw, bot, 0, -hw, 0, 0, -hw, top, h)
  panel(0, h, hw, top, h, hw, bot, 0, hw, 0, 0, hw, top, h)

  const geo = new BufferGeometry()
  geo.setAttribute('position', new Float32BufferAttribute(positions, 3))
  return geo
}
