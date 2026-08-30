/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

const apiTarget = process.env.HOPSTAT_API_TARGET || 'http://localhost:8080'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    include: ['src/**/*.test.{ts,tsx}'],
    coverage: {
      provider: 'v8',
      // The logic layer only. Components and pages are covered by the browser suite in
      // e2e/, which exercises them as rendered pages rather than in isolation.
      include: ['src/lib/**', 'src/hooks/**'],
      exclude: ['**/*.test.{ts,tsx}'],
      reporter: ['text-summary'],
      // A ratchet, not a target: set just under the measured baseline so coverage cannot
      // slide, without demanding low-value tests to reach a round number.
      thresholds: {
        statements: 62,
        branches: 50,
        functions: 56,
        lines: 63,
      },
    },
  },
  build: {
    outDir: '../dist',
    emptyOutDir: true,
    manifest: true,
    sourcemap: false,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': { target: apiTarget, changeOrigin: true },
      '/health': { target: apiTarget, changeOrigin: true },
      '^/logo\\.(png|jpg|svg|webp)$': { target: apiTarget, changeOrigin: true },
    },
  },
})
