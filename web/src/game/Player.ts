// A procedurally-built footballer: a jointed humanoid made entirely from
// primitive meshes (no rig, no GLTF, no external asset) so it matches the rest
// of the scene, which is all built in code.
//
// The figure is a small hierarchy of pivot Groups — hips, knees, ANKLES,
// shoulders, elbows — so animating it is just rotating those pivots each frame.
// update() drives a sine-based gait whose stride length and cadence scale with
// the player's speed (idle → walk → run/sprint), dropping into a lower, hunched
// stance while dribbling. Dribble touches are NOT part of the gait cycle: the
// caller fires triggerTouch() when the server actually knocks the ball, and a
// one-shot envelope pokes that foot out (toe dipping on contact) — so the foot
// meets the ball exactly when it's pushed, including on a change of direction.
// The whole player faces local +Z; the caller positions and yaws the returned
// group from the server snapshot.
//
// Kit colours: the jersey/sleeves/socks share one material that the scene tints
// to signal possession (green = on the ball, blue = free), matching the old
// capsule's behaviour but over a much bigger area so the cue reads from the
// broadcast camera.

import {
  BoxGeometry,
  Group,
  Mesh,
  MeshStandardMaterial,
  SphereGeometry,
} from 'three'

// Overall metrics, in metres. Joint heights are measured from the ground (the
// group's origin sits on the pitch). The limbs hang DOWN from their pivots, so
// each pivot is placed at a joint and its meshes are offset by -length/2. The
// leg segments are sized so the boot sole lands on y=0 when the leg is straight:
// HIP_Y ≈ THIGH_LEN + SHIN_LEN + boot drop.
const HIP_Y = 0.9 // hip joint height
const SHOULDER_Y = 1.46 // shoulder joint height
const HIP_X = 0.12 // half the gap between the two hips
const SHOULDER_X = 0.26 // half the gap between the two shoulders
const HEAD_Y = 1.8

const THIGH_LEN = 0.52
const SHIN_LEN = 0.49
const UPPER_ARM_LEN = 0.34
const FOREARM_LEN = 0.3

// Kit / body colours. The jersey colour is supplied by the caller (and re-tinted
// for possession); everything else is fixed.
const SKIN = 0xe8b48f
const SHORTS = 0xf2f2f2
const BOOT = 0x18181c
const HAIR = 0x2b1d12

export interface Player {
  group: Group
  /**
   * Advance the gait by `dt` seconds.
   * - `speed`: planar speed in m/s — drives stride length + cadence.
   * - `possessing`: switches to the hunched dribbling stance.
   * Call every frame.
   */
  update: (dt: number, speed: number, possessing: boolean) => void
  /**
   * Play a one-shot dribble touch: a quick foot poke synced to the server
   * knocking the ball forward. `side` picks the foot ('L' = the +X-side leg,
   * 'R' = the −X-side leg) — choose whichever is nearer the ball. `reach` (~0–1.5)
   * scales how far the foot extends, so a longer knock draws a bigger poke.
   */
  triggerTouch: (side: 'L' | 'R', reach: number) => void
  /** Tint the kit (jersey, sleeves, socks) — used to flag possession. */
  setKitColor: (color: number) => void
  dispose: () => void
}

interface Limb {
  top: Group // hip / shoulder pivot
  joint: Group // knee / elbow pivot
  ankle: Group | null // ankle pivot (legs only)
}

export function createPlayer(jerseyColor: number): Player {
  const group = new Group()

  // `root` carries the whole body so we can bob/sway it without moving the group
  // origin (which the scene pins to the pitch from the snapshot).
  const root = new Group()
  group.add(root)

  // Shared materials. Collected (with geometries) for disposal at the end.
  const jerseyMat = new MeshStandardMaterial({ color: jerseyColor, roughness: 0.8 })
  const skinMat = new MeshStandardMaterial({ color: SKIN, roughness: 0.9 })
  const shortsMat = new MeshStandardMaterial({ color: SHORTS, roughness: 0.9 })
  const bootMat = new MeshStandardMaterial({ color: BOOT, roughness: 0.6 })
  const hairMat = new MeshStandardMaterial({ color: HAIR, roughness: 1 })

  const geometries: { dispose: () => void }[] = []
  // Make a box, remember its geometry for disposal, and return the mesh.
  const box = (w: number, h: number, d: number, mat: MeshStandardMaterial) => {
    const geo = new BoxGeometry(w, h, d)
    geometries.push(geo)
    return new Mesh(geo, mat)
  }

  // Torso, shorts, and a short neck. The torso is the jersey; the shorts sit at
  // the hips and stay put (parented to root) while the legs swing beneath them.
  const torso = box(0.46, 0.58, 0.26, jerseyMat)
  torso.position.y = 1.22
  root.add(torso)

  const shorts = box(0.52, 0.28, 0.3, shortsMat)
  shorts.position.y = 1.0
  root.add(shorts)

  const neck = box(0.12, 0.18, 0.12, skinMat)
  neck.position.y = 1.58
  root.add(neck)

  // Head + a little hair cap so the figure has a front.
  const headGeo = new SphereGeometry(0.135, 16, 12)
  geometries.push(headGeo)
  const head = new Mesh(headGeo, skinMat)
  head.position.y = HEAD_Y
  root.add(head)

  const hairGeo = new SphereGeometry(0.145, 16, 12)
  geometries.push(hairGeo)
  const hair = new Mesh(hairGeo, hairMat)
  hair.scale.set(1, 0.8, 1)
  hair.position.set(0, HEAD_Y + 0.04, -0.02)
  root.add(hair)

  // Build one limb hanging from a pivot at `(x, y, 0)`: an upper segment, a knee/
  // elbow pivot, a lower segment, and — for legs — an ankle pivot carrying a
  // boot. Returning each pivot lets the gait rotate hip, knee and ankle.
  const buildLimb = (
    x: number,
    y: number,
    upperLen: number,
    lowerLen: number,
    upperW: number,
    lowerW: number,
    upperMat: MeshStandardMaterial,
    lowerMat: MeshStandardMaterial,
    boot: boolean,
  ): Limb => {
    const top = new Group()
    top.position.set(x, y, 0)
    const upper = box(upperW, upperLen, upperW, upperMat)
    upper.position.y = -upperLen / 2
    top.add(upper)

    const joint = new Group()
    joint.position.y = -upperLen
    top.add(joint)
    const lower = box(lowerW, lowerLen, lowerW, lowerMat)
    lower.position.y = -lowerLen / 2
    joint.add(lower)

    let ankle: Group | null = null
    if (boot) {
      ankle = new Group()
      ankle.position.y = -lowerLen
      joint.add(ankle)
      const foot = box(lowerW + 0.04, 0.1, 0.34, bootMat)
      foot.position.set(0, -0.04, 0.09) // below + forward of the ankle (heel→toe)
      ankle.add(foot)
    }

    root.add(top)
    return { top, joint, ankle }
  }

  // Legs (skin thigh, jersey-coloured sock, boot) and arms (jersey sleeve, skin
  // forearm). +X is the player's left here; the pair is symmetric.
  const legL = buildLimb(HIP_X, HIP_Y, THIGH_LEN, SHIN_LEN, 0.17, 0.15, skinMat, jerseyMat, true)
  const legR = buildLimb(-HIP_X, HIP_Y, THIGH_LEN, SHIN_LEN, 0.17, 0.15, skinMat, jerseyMat, true)
  const armL = buildLimb(SHOULDER_X, SHOULDER_Y, UPPER_ARM_LEN, FOREARM_LEN, 0.14, 0.12, jerseyMat, skinMat, false)
  const armR = buildLimb(-SHOULDER_X, SHOULDER_Y, UPPER_ARM_LEN, FOREARM_LEN, 0.14, 0.12, jerseyMat, skinMat, false)

  // --- Gait state ---------------------------------------------------------
  let phase = 0 // stride phase, radians
  let gait = 0 // 0 = idle → 1 = full stride (eased toward movement)
  let stance = 0 // 0 = upright run → 1 = hunched dribble (eased)

  // Per-foot touch envelopes. triggerTouch() arms a timer; while it counts down,
  // a 0→1→0 sine envelope drives a one-shot poke of that foot, scaled by reach.
  const TOUCH_DUR = 0.22 // seconds a touch poke takes to play out
  let touchTimerL = 0
  let touchTimerR = 0
  let touchReachL = 1
  let touchReachR = 1

  const REF_SPEED = 7 // m/s treated as "full sprint" for scaling
  const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v))
  const lerp = (a: number, b: number, t: number) => a + (b - a) * t

  function update(dt: number, speed: number, possessing: boolean) {
    const speedNorm = Math.min(speed / REF_SPEED, 1)
    const moving = speed > 0.15

    // Ease the high-level states so transitions (start/stop, gain/lose ball)
    // don't pop. Rates are per-second, scaled by dt.
    gait += ((moving ? 1 : 0) - gait) * Math.min(1, dt * 10)
    stance += ((possessing && moving ? 1 : 0) - stance) * Math.min(1, dt * 6)

    // Cadence: legs turn over faster the quicker you move. Phase only advances
    // while moving, so the figure freezes mid-pose at rest rather than marching.
    const cadence = lerp(7, 13, speedNorm)
    phase += dt * cadence * gait

    const s = Math.sin(phase)
    const c = Math.cos(phase)

    // Stride and arm swing grow with speed; dribbling shortens the base stride
    // (small, controlled steps) and quietens the arms.
    const legAmp = lerp(0.35, 0.95, speedNorm) * gait * lerp(1, 0.6, stance)
    const armAmp = lerp(0.25, 0.7, speedNorm) * gait * lerp(1, 0.6, stance)

    // Dribble touches. Each foot's poke is a one-shot fired by triggerTouch()
    // when the server knocks the ball forward — NOT a periodic swing — so the
    // foot meets the ball exactly when it actually gets pushed (including on a
    // change of direction). The envelope ramps 0→1→0 over TOUCH_DUR.
    touchTimerL = Math.max(0, touchTimerL - dt)
    touchTimerR = Math.max(0, touchTimerR - dt)
    const envL = touchTimerL > 0 ? Math.sin(Math.PI * (1 - touchTimerL / TOUCH_DUR)) : 0
    const envR = touchTimerR > 0 ? Math.sin(Math.PI * (1 - touchTimerR / TOUCH_DUR)) : 0
    const pokeL = envL * touchReachL
    const pokeR = envR * touchReachR

    // Hips swing in antiphase, with the poke adding extra forward flexion.
    legL.top.rotation.x = s * legAmp + pokeL * 0.55
    legR.top.rotation.x = -s * legAmp + pokeR * 0.55

    // Knees only bend one way (forward), peaking on the recovery swing — a
    // rectified, phase-shifted cosine keeps them from snapping backwards. A
    // baseline crouch is added when dribbling; the poke straightens the touching
    // knee so the foot extends to meet the ball.
    const crouch = 0.4 * stance
    legL.joint.rotation.x = (Math.max(0, -c) * 0.9 + 0.1) * gait + crouch
    legR.joint.rotation.x = (Math.max(0, c) * 0.9 + 0.1) * gait + crouch
    legL.joint.rotation.x *= 1 - pokeL * 0.6
    legR.joint.rotation.x *= 1 - pokeR * 0.6

    // Ankles point the toe down/forward on contact (and flat otherwise) so the
    // touch reads as the boot striking down onto the ball.
    if (legL.ankle) legL.ankle.rotation.x = pokeL * 0.8
    if (legR.ankle) legR.ankle.rotation.x = pokeR * 0.8

    // Arms swing opposite their same-side leg; elbows hold a slight constant
    // bend, and the shoulders splay a little wider when dribbling for balance.
    const splay = 0.12 + 0.18 * stance
    armL.top.rotation.x = -s * armAmp
    armR.top.rotation.x = s * armAmp
    armL.top.rotation.z = -splay
    armR.top.rotation.z = splay
    armL.joint.rotation.x = 0.3 + 0.2 * gait
    armR.joint.rotation.x = 0.3 + 0.2 * gait

    // Lean forward into the run, more so when hunched over the ball; bob the body
    // twice per stride (|sin| peaks at each foot-plant) and add a small dribbling
    // weight-shift sway from side to side.
    root.rotation.x = lerp(0.05, 0.28, speedNorm) * gait + 0.2 * stance
    root.rotation.z = s * 0.05 * stance
    root.position.y = Math.abs(s) * 0.04 * gait
  }

  // Arm a one-shot poke on the chosen foot. reach is clamped to a sane span so a
  // wild ball offset can't fling the leg out. Re-arming mid-poke just restarts it.
  function triggerTouch(side: 'L' | 'R', reach: number) {
    const r = clamp(reach, 0.4, 1.5)
    if (side === 'L') {
      touchTimerL = TOUCH_DUR
      touchReachL = r
    } else {
      touchTimerR = TOUCH_DUR
      touchReachR = r
    }
  }

  function setKitColor(color: number) {
    jerseyMat.color.set(color)
  }

  return {
    group,
    update,
    triggerTouch,
    setKitColor,
    dispose() {
      for (const g of geometries) g.dispose()
      jerseyMat.dispose()
      skinMat.dispose()
      shortsMat.dispose()
      bootMat.dispose()
      hairMat.dispose()
    },
  }
}
