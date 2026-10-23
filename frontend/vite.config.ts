import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import {execFileSync} from 'node:child_process'
import { fileURLToPath, URL } from 'node:url'

// https://vitejs.dev/config/
//
// Note: this file is the single source of truth for both the Vite build and the Vitest run.
// The compiled `vite.config.js` sitting next to it is a gitignored `vue-tsc -b` artifact and
// takes precedence during Vite's own config resolution, so the test script pins the config
// explicitly (`vitest run --config vite.config.ts`) rather than relying on resolution order.
export default defineConfig({
  plugins: [vue()],
  define: {'__CST_VERSION__': JSON.stringify('build-'+execFileSync('git',['rev-parse','--short=8','HEAD'],{encoding:'utf8'}).trim())},
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    strictPort: true,
    proxy: { '/api': process.env.SPEEDTEST_API_URL || 'http://127.0.0.1:8999' },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks: {
          'vendor-vue': ['vue', 'pinia'],
        },
      },
    },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.spec.ts'],
    testTimeout: 10000,
    restoreMocks: true,
    clearMocks: true,
  },
})
