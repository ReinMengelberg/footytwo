// A floating kick-charge meter shown above the player while charging a shot/pass.
// It reads the server's authoritative charge state (so it only appears when the
// charge is really building) and shows three things at a glance:
//   - power: a bar that fills left→right and shifts green (soft) → red (hard).
//   - curve: the bar tilts toward the dialled-in side-spin direction.
//   - loft:  the bar rides up for a lofted ball, down for a driven one.
// It's a billboard facing +Z, which is where the fixed broadcast camera looks
// from, so it always reads head-on without per-frame re-orientation.

import { Color, Group, Mesh, MeshBasicMaterial, PlaneGeometry } from 'three'

export interface ChargeIndicator {
  group: Group
  // power 0..1; spin -1..1 (sign = curve direction); lift -1..1 (+loft / -driven).
  update: (power: number, spin: number, lift: number) => void
  dispose: () => void
}

const BAR_W = 1.6
const BAR_H = 0.22
const PAD = 0.06
const TILT = 0.35 // radians of bar tilt at full spin
const LOFT_RISE = 0.2 // metres the bar rides up/down at full loft/drive

export function createChargeIndicator(): ChargeIndicator {
  const group = new Group() // positioned at the player by the scene
  group.visible = false

  const bar = new Group() // tilts (curve) and rides up/down (loft) within the group
  group.add(bar)

  const backGeo = new PlaneGeometry(BAR_W + PAD * 2, BAR_H + PAD * 2)
  const backMat = new MeshBasicMaterial({ color: 0x111111, transparent: true, opacity: 0.55 })
  bar.add(new Mesh(backGeo, backMat))

  const fillGeo = new PlaneGeometry(BAR_W, BAR_H)
  const fillMat = new MeshBasicMaterial({ color: 0x33dd66 })
  const fill = new Mesh(fillGeo, fillMat)
  fill.position.z = 0.001 // sit just in front of the backboard to avoid z-fighting
  bar.add(fill)

  const fillColor = new Color()

  return {
    group,
    update(power, spin, lift) {
      if (power <= 0.001) {
        group.visible = false
        return
      }
      group.visible = true

      // Power: grow the fill from the left edge; hue green (low) → red (high).
      const p = Math.min(Math.max(power, 0), 1)
      fill.scale.x = p
      fill.position.x = (-BAR_W / 2) * (1 - p)
      fillColor.setHSL(0.33 * (1 - p), 0.85, 0.5)
      fillMat.color.copy(fillColor)

      // Curve tilts the bar; loft rides it up (+) or down (-).
      bar.rotation.z = -spin * TILT
      bar.position.y = lift * LOFT_RISE
    },
    dispose() {
      backGeo.dispose()
      backMat.dispose()
      fillGeo.dispose()
      fillMat.dispose()
    },
  }
}
