// Raw Three.js scene for Phase 2.
//
// Composes the pitch (striped ground + grass, see Pitch.ts), two goals (see
// Goal.ts), a jointed procedural footballer (see Player.ts) and a ball sphere.
// State comes straight from server snapshots — positions are applied raw, with
// NO client-side prediction or interpolation (that's a later phase). The player's
// run/dribble gait is driven from snapshot-derived speed, and its foot pokes are
// fired off the server's per-snapshot dribble-touch counter. A follow camera
// tracks the ball with a FIXED orientation so WASD maps cleanly to world axes.

import {
  AmbientLight,
  Clock,
  Color,
  DirectionalLight,
  Mesh,
  MeshStandardMaterial,
  PerspectiveCamera,
  Scene,
  SphereGeometry,
  WebGLRenderer,
} from 'three'
import type { Snapshot } from '../net/protocol'
import { createChargeIndicator } from './ChargeIndicator'
import { createGoal } from './Goal'
import { createPitch, PITCH_WIDTH } from './Pitch'
import { createPlayer } from './Player'

export interface GameScene {
  applySnapshot: (snap: Snapshot) => void
  dispose: () => void
}

const BALL_RADIUS = 0.15

const COLOR_FREE = 0x3366ff // player tint when not possessing
const COLOR_OWN = 0x33dd66 // player tint when possessing

const INDICATOR_Y = 2.6 // height of the charge meter above a player (just over head height)

// FIFA-style side broadcast camera. It sits set back behind the near touchline,
// looking straight across the pitch, and follows the ball on BOTH ground axes so
// it stays framed: it pans on X (goal-to-goal, screen left/right) and trucks on Z
// (touchline-to-touchline) as the ball moves up/down the pitch. Height is fixed
// and it never zooms. Looking from +Z toward -Z makes world +X render to
// screen-right, so D = right. The camera keeps a constant CAM_DEPTH gap between
// itself and the point it looks at, so it always faces straight across.
const CAM_HEIGHT = 30 // fixed Y
const CAM_DEPTH = 40 // Z gap kept between the camera and its look point
const CAM_LOOK_Z = 0 // base Z of the look point (the Z truck shifts it)
const CAM_LERP = 0.08 // follow smoothing (shared by the X pan and Z truck)
const CAM_FOV = 40 // vertical FOV; lower = zoomed in (was 60)

// Depth follow. Like the X pan, the camera trucks along Z to follow the ball
// between the touchlines, keeping it centred. Clamped to ±CAM_Z_RANGE so it
// stops short of the sidelines instead of trucking out past the pitch.
const CAM_Z_RANGE = 30 // metres of Z follow each side of centre before it stops

const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v))

export function createScene(canvas: HTMLCanvasElement): GameScene {
  const renderer = new WebGLRenderer({ canvas, antialias: true })
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2))
  renderer.setSize(window.innerWidth, window.innerHeight)

  const scene = new Scene()
  scene.background = new Color(0x87ceeb)

  const camera = new PerspectiveCamera(CAM_FOV, window.innerWidth / window.innerHeight, 0.1, 1000)
  camera.position.set(0, CAM_HEIGHT, CAM_DEPTH)
  camera.lookAt(0, 0, CAM_LOOK_Z)

  const ambient = new AmbientLight(0xffffff, 0.7)
  scene.add(ambient)
  const sun = new DirectionalLight(0xffffff, 1.1)
  sun.position.set(20, 40, 10)
  scene.add(sun)

  // Pitch (striped ground + grass).
  const pitch = createPitch(renderer.capabilities.getMaxAnisotropy())
  scene.add(pitch.group)

  // Goals at each end of the long (X) axis. A goal is built facing local +X, so
  // the +X goal sits as-is and the -X goal is spun 180° to face inward.
  const goalRight = createGoal()
  goalRight.group.position.x = PITCH_WIDTH / 2
  scene.add(goalRight.group)

  const goalLeft = createGoal()
  goalLeft.group.position.x = -PITCH_WIDTH / 2
  goalLeft.group.rotation.y = Math.PI
  scene.add(goalLeft.group)

  // Player: a jointed procedural footballer (see Player.ts). It faces local +Z,
  // so the snapshot's facing yaw orients it the same way the old capsule's nose
  // did. The kit is tinted to flag possession.
  const player = createPlayer(COLOR_FREE)
  scene.add(player.group)

  // Kick-charge meter, floated above the player's head while charging.
  const chargeIndicator = createChargeIndicator()
  scene.add(chargeIndicator.group)

  // Ball.
  const ballGeometry = new SphereGeometry(BALL_RADIUS, 16, 12)
  const ballMaterial = new MeshStandardMaterial({ color: 0xffffff })
  const ball = new Mesh(ballGeometry, ballMaterial)
  scene.add(ball)

  // The camera eases toward these per-snapshot targets so it stays on the ball:
  // X pans it goal-to-goal; Z trucks it between the touchlines (clamped to
  // ±CAM_Z_RANGE). Height is fixed.
  let camTargetX = 0
  let camTargetZ = 0

  // Player gait inputs. Speed is estimated from how far the player moved between
  // snapshots (there's no client interpolation yet) and smoothed so the run
  // cycle doesn't stutter; `possessing` switches the player to its dribble stance.
  let playerSpeed = 0
  let possessing = false
  let lastTouch = 0 // last seen server dribble-touch count; a rise means a fresh touch
  let prevX = 0
  let prevZ = 0
  let prevSampleT = -1

  const clock = new Clock()
  let lastElapsed = 0
  renderer.setAnimationLoop(() => {
    const elapsed = clock.getElapsedTime()
    const dt = Math.min(elapsed - lastElapsed, 0.1) // clamp big tab-switch gaps
    lastElapsed = elapsed
    pitch.update(elapsed) // advance the wind
    player.update(dt, playerSpeed, possessing)
    // Smoothly pan on X and truck on Z. The camera and the point it looks at
    // slide together — keeping the constant CAM_DEPTH gap — so it always faces
    // straight across the pitch while tracking the ball on both ground axes.
    const cx = camera.position.x + (camTargetX - camera.position.x) * CAM_LERP
    const cz = camera.position.z + (CAM_DEPTH + camTargetZ - camera.position.z) * CAM_LERP
    camera.position.set(cx, CAM_HEIGHT, cz)
    camera.lookAt(cx, 0, CAM_LOOK_Z + cz - CAM_DEPTH)
    renderer.render(scene, camera)
  })

  const onResize = () => {
    const { innerWidth: w, innerHeight: h } = window
    camera.aspect = w / h
    camera.updateProjectionMatrix()
    renderer.setSize(w, h)
  }
  window.addEventListener('resize', onResize)

  function applySnapshot(snap: Snapshot) {
    const me = snap.players.find((p) => p.id === 'p1') ?? snap.players[0]
    if (me) {
      player.group.position.set(me.x, 0, me.z)
      // Yaw from facing vector; matches the server's atan2(facingX, facingZ).
      player.group.rotation.y = Math.atan2(me.facingX, me.facingZ)
      player.setKitColor(me.possessing ? COLOR_OWN : COLOR_FREE)
      possessing = me.possessing

      // Float the charge meter above the player's head and drive it from the
      // server's authoritative charge state (only shows while actually charging).
      chargeIndicator.group.position.set(me.x, INDICATOR_Y, me.z)
      chargeIndicator.update(me.chargePower, me.chargeSpin, me.chargeLift)

      // Estimate planar speed from the distance covered since the last snapshot,
      // smoothed, to drive the run cadence/stride.
      const t = clock.getElapsedTime()
      if (prevSampleT >= 0 && t > prevSampleT) {
        const inst = Math.hypot(me.x - prevX, me.z - prevZ) / (t - prevSampleT)
        playerSpeed += (inst - playerSpeed) * 0.4
      }
      prevX = me.x
      prevZ = me.z
      prevSampleT = t
    }
    ball.position.set(snap.ball.x, snap.ball.y, snap.ball.z)
    // A rise in the server's touch counter means the dribbler just knocked the
    // ball — play a one-shot foot poke. Pick the foot on the side the ball sits
    // (its local X relative to the player) and scale the reach by how far away it
    // is. Catching up across multiple ticks can bump the counter by >1; one poke
    // still reads fine.
    if (me && snap.touch > lastTouch) {
      const yaw = Math.atan2(me.facingX, me.facingZ)
      const dx = snap.ball.x - me.x
      const dz = snap.ball.z - me.z
      // World→local: rotate the offset by -yaw. localX runs +X to the player's
      // local left, localZ = +Z is straight ahead (= facing).
      const localX = Math.cos(yaw) * dx - Math.sin(yaw) * dz
      const dist = Math.hypot(dx, dz)
      player.triggerTouch(localX >= 0 ? 'L' : 'R', dist / 0.5)
    }
    lastTouch = snap.touch
    // The broadcast camera follows the ball on both ground axes: pan on X
    // (goal-to-goal) and truck on Z (touchline-to-touchline), the Z clamped to
    // ±CAM_Z_RANGE so it stops short of the sidelines.
    camTargetX = snap.ball.x
    camTargetZ = clamp(snap.ball.z, -CAM_Z_RANGE, CAM_Z_RANGE)
  }

  return {
    applySnapshot,
    dispose() {
      renderer.setAnimationLoop(null)
      window.removeEventListener('resize', onResize)
      pitch.dispose()
      goalRight.dispose()
      goalLeft.dispose()
      player.dispose()
      chargeIndicator.dispose()
      ballGeometry.dispose()
      ballMaterial.dispose()
      renderer.dispose()
    },
  }
}
