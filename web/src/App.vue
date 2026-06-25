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

let scene: GameScene | null = null
let keyboard: Keyboard | null = null
let conn: Connection | null = null
let sendTimer: number | undefined

onMounted(() => {
  if (!canvasRef.value) return

  scene = createScene(canvasRef.value)
  keyboard = createKeyboard()
  conn = connect(WS_URL, {
    onSnapshot: (snap) => scene?.applySnapshot(snap),
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
    }
    conn!.send(input)
  }, 1000 / SEND_HZ)
})

onBeforeUnmount(() => {
  if (sendTimer !== undefined) clearInterval(sendTimer)
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
  </div>
</template>
