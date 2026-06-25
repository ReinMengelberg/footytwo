import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

// Client-only SPA (no SSR). Raw Three.js renders inside a Vue component.
// Tailwind v4 is wired via its Vite plugin — no config file needed.
export default defineConfig({
  plugins: [vue(), tailwindcss()],
  server: {
    port: 5173,
  },
})
