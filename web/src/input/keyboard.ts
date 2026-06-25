// Keyboard input: tracks held WASD + Shift and turns them into a raw movement
// intent. Axes are summed (not normalized) — the server normalizes so diagonals
// aren't faster. Mapping is chosen to match the FIFA-style side camera (which
// looks across the pitch from +Z): W = up the screen = into the pitch (-Z),
// S = +Z toward the camera, D = +X = screen-right, A = -X = screen-left.
// Shift = sprint.

export interface MoveIntent {
  moveX: number // -1..1
  moveZ: number // -1..1
  sprint: boolean
}

export interface Keyboard {
  getIntent: () => MoveIntent
  dispose: () => void
}

export function createKeyboard(): Keyboard {
  const held = new Set<string>()

  const onKeyDown = (e: KeyboardEvent) => held.add(e.code)
  const onKeyUp = (e: KeyboardEvent) => held.delete(e.code)
  // Releasing focus shouldn't leave keys stuck "down".
  const onBlur = () => held.clear()

  window.addEventListener('keydown', onKeyDown)
  window.addEventListener('keyup', onKeyUp)
  window.addEventListener('blur', onBlur)

  return {
    getIntent() {
      let moveX = 0
      let moveZ = 0
      if (held.has('KeyW')) moveZ -= 1 // up the screen / into the pitch
      if (held.has('KeyS')) moveZ += 1 // down the screen / toward the camera
      if (held.has('KeyD')) moveX += 1 // screen-right
      if (held.has('KeyA')) moveX -= 1 // screen-left
      const sprint = held.has('ShiftLeft') || held.has('ShiftRight')
      return { moveX, moveZ, sprint }
    },
    dispose() {
      window.removeEventListener('keydown', onKeyDown)
      window.removeEventListener('keyup', onKeyUp)
      window.removeEventListener('blur', onBlur)
    },
  }
}
