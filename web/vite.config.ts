import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [svelte()],
  publicDir: 'static',
  build: {
    target: 'es2022',
    outDir: '../internal/webembed/dist',
    emptyOutDir: true,
  },
  // Crawl every source file at startup so the dev server pre-bundles all
  // dependencies (each per-icon lucide import, the lazily loaded pages' deps)
  // before the first page loads. Otherwise Vite discovers them mid-session
  // and force-reloads every open tab, which broke e2e runs in the middle of
  // an axe scan ("Execution context was destroyed").
  optimizeDeps: {
    entries: [
      'index.html',
      'src/**/*.svelte',
      'src/**/*.ts',
      '!src/**/*.test.ts',
      '!src/**/*.test.svelte',
      '!src/**/testing/**',
    ],
  },
  server: {
    host: '127.0.0.1',
    proxy: {
      '/api': { target: 'http://127.0.0.1:8080', changeOrigin: false },
      '/healthz': { target: 'http://127.0.0.1:8080', changeOrigin: false },
    },
  },
});
