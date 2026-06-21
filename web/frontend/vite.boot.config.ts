import { defineConfig } from 'vite'
import path from 'path'
import { resolve } from 'path'

export default defineConfig({
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    outDir: '../dist',
    emptyOutDir: false,
    lib: {
      entry: resolve(__dirname, 'src/appearance-boot.ts'),
      formats: ['iife'],
      name: 'HopStatBoot',
      fileName: () => 'appearance-boot.js',
    },
    rollupOptions: {
      output: {
        inlineDynamicImports: true,
      },
    },
  },
})
