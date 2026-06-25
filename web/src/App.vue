<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { createScene, type GameScene } from './game/Scene'
import { createKeyboard, type Keyboard } from './input/keyboard'
import { connect, type Connection, type ConnStatus } from './net/connection'
import type { ClientInput } from './net/protocol'

const WS_URL = 'ws://localhost:8080/ws'
const SEND_HZ = 30

const canvasRef = ref<HTMLCanvasElement | null>(null)
const status = ref<ConnStatus>('connecting')
const score = ref({ home: 0, away: 0 })
const restartLabel = ref('') // the current restart call to flash ('' = hidden)

let scene: GameScene | null = null
let keyboard: Keyboard | null = null
let conn: Connection | null = null
let sendTimer: number | undefined
let bannerTimer: number | undefined

// How a restart is announced over the HUD, and how long the flash lingers (a goal
// dwells longer, matching the server's longer celebration pause before kickoff).
const RESTART_TEXT: Record<string, string> = {
  throwin: 'Throw-in',
  goalkick: 'Goal kick',
  corner: 'Corner',
  goal: 'GOAL!',
}
const RESTART_MS: Record<string, number> = { goal: 3000 }
const RESTART_MS_DEFAULT = 1600

let lastRestart = 0 // last seen server restart counter; a rise means a fresh call

onMounted(() => {
  if (!canvasRef.value) return

  scene = createScene(canvasRef.value)
  keyboard = createKeyboard()
  conn = connect(WS_URL, {
    onSnapshot: (snap) => {
      scene?.applySnapshot(snap)
      score.value = { home: snap.scoreHome, away: snap.scoreAway }
      // A rise in the restart counter means the ball just went out — flash the call
      // (it stays up through the server's dead-ball pause, then clears itself).
      if (snap.restart > lastRestart) {
        lastRestart = snap.restart
        restartLabel.value = RESTART_TEXT[snap.restartKind] ?? ''
        if (bannerTimer !== undefined) clearTimeout(bannerTimer)
        bannerTimer = window.setTimeout(() => {
          restartLabel.value = ''
        }, RESTART_MS[snap.restartKind] ?? RESTART_MS_DEFAULT)
      }
    },
    onStatus: (s) => (status.value = s),
  })

  // Steady-rate input send (simpler than send-on-change for now).
  let seq = 0
  sendTimer = window.setInterval(() => {
    const intent = keyboard!.getIntent()
    const input: ClientInput = {
      seq: seq++,
      moveX: intent.moveX,
      moveZ: intent.moveZ,
      sprint: intent.sprint,
      charge: intent.charge,
      lift: intent.lift,
      spin: intent.spin,
    }
    conn!.send(input)
  }, 1000 / SEND_HZ)
})

onBeforeUnmount(() => {
  if (sendTimer !== undefined) clearInterval(sendTimer)
  if (bannerTimer !== undefined) clearTimeout(bannerTimer)
  conn?.close()
  keyboard?.dispose()
  scene?.dispose()
})

const statusLabel: Record<ConnStatus, string> = {
  connecting: 'connecting…',
  open: 'connected',
  closed: 'disconnected',
}
</script>

<template>
  <canvas ref="canvasRef" class="block h-screen w-screen"></canvas>

  <!-- HUD overlay (Tailwind); pointer-events-none so it never eats input. -->
  <div class="pointer-events-none absolute inset-0 select-none p-4 font-mono text-sm text-white">
    <div class="inline-block rounded bg-black/40 px-3 py-2 leading-relaxed">
      <div class="font-semibold">footy · phase 2</div>
      <div>WASD to move · Shift to sprint</div>
      <div>Space = kick (hold for power) · ↑ loft ↓ driven ←→ curve</div>
      <div>
        server:
        <span
          :class="{
            'text-green-400': status === 'open',
            'text-yellow-300': status === 'connecting',
            'text-red-400': status === 'closed',
          }"
          >{{ statusLabel[status] }}</span
        >
      </div>
    </div>

    <!-- Scoreboard, top centre. -->
    <div class="absolute left-1/2 top-4 -translate-x-1/2 rounded bg-black/40 px-4 py-2 text-lg font-semibold tracking-wider">
      <span class="text-sky-300">HOME</span>
      <span class="mx-2">{{ score.home }} – {{ score.away }}</span>
      <span class="text-rose-300">AWAY</span>
    </div>

    <!-- Restart call, flashed centre-screen while the ball is dead. -->
    <div
      v-if="restartLabel"
      class="absolute left-1/2 top-1/3 -translate-x-1/2 rounded-lg bg-black/60 px-8 py-4 text-4xl font-extrabold uppercase tracking-widest drop-shadow-lg"
    >
      {{ restartLabel }}
    </div>
  </div>
</template>
