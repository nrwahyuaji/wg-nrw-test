// Konfigurasi Vite WG-WebUI. Hasil build (dist/) disalin ke WG-Server/internal/web/dist lalu di-embed ke biner Go
// (`make web`). Dev server :5175 mem-proxy /api ke panel dev di netns wgdev; port itu perlu aturan UFW dari sysadmin.
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    host: '0.0.0.0',
    port: 5175,
    strictPort: true,
    proxy: { '/api': 'http://10.251.0.2:8091' },
  },
  build: { outDir: 'dist', emptyOutDir: true, sourcemap: false },
  test: { environment: 'node', include: ['src/**/*.test.ts'] },
})
