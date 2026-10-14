import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { fileURLToPath } from 'node:url'

// The build goes into the Go package that embeds it (internal/webui).
export default defineConfig({
  plugins: [svelte()],
  resolve: { alias: { 'styled-system': fileURLToPath(new URL('./styled-system', import.meta.url)) } },
  build: {
    outDir: '../internal/webui/dist',
    emptyOutDir: true,
    target: 'es2022',
    chunkSizeWarningLimit: 800,
  },
  server: {
    // During development: `jw app --browser --port 7777`, then `npm run dev`.
    proxy: { '/api': 'http://127.0.0.1:7777' },
  },
})
