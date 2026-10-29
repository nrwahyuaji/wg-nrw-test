// Konfigurasi Playwright. Target panel diambil dari WGPANEL_E2E_URL (bawaan panel dev di netns wgdev); uji tidak
// menjalankan server sendiri. WGPANEL_E2E_CHROMIUM boleh menunjuk Chromium terpasang bila revisi bawaan Playwright
// belum diunduh (host dev: /root/.cache/ms-playwright/chromium-1234).
import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: 'e2e',
  globalSetup: './e2e/global-setup.ts',
  workers: 1, // satu panel dev bersama: uji tidak boleh saling mengubah data bersamaan
  timeout: 30_000,
  retries: 0,
  reporter: [['list']],
  use: {
    baseURL: process.env.WGPANEL_E2E_URL ?? 'http://10.251.0.2:8091',
    launchOptions: process.env.WGPANEL_E2E_CHROMIUM ? { executablePath: process.env.WGPANEL_E2E_CHROMIUM } : {},
    trace: 'off',
  },
})
