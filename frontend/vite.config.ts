import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

// https://vitejs.dev/config/
//
// Note: this file is the single source of truth for both the Vite build and the Vitest run.
// The compiled `vite.config.js` sitting next to it is a gitignored `vue-tsc -b` artifact and
// takes precedence during Vite's own config resolution, so the test script pins the config
// explicitly (`vitest run --config vite.config.ts`) rather than relying on resolution order.
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    strictPort: true,
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.spec.ts'],
    restoreMocks: true,
    clearMocks: true,
  },
})
