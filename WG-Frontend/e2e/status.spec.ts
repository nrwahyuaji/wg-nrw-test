// E2E Fase 7.2 (PLAN): status live + dashboard. Prasyarat: `sudo scripts/e2e-handshake.sh --keep` (peer uji-e2e hidup
// dengan ping latar). Dijalankan terpisah dari suite bawaan karena butuh tunnel aktif dan root (ping dari netns wgcli):
//   WGPANEL_E2E_LIVE=1 npx playwright test e2e/status.spec.ts
import { execFileSync } from 'node:child_process'
import { expect, test } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'

/** pingTunnel mengirim lalu lintas dari "perangkat client" (netns wgcli) sampai tunnel menjawab. Setelah peer di-enable,
 *  client masih memakai sesi yang sudah dibuang server; WireGuard baru handshake ulang setelah KEEPALIVE_TIMEOUT (10 dtk)
 *  + REKEY_TIMEOUT (5 dtk) tanpa balasan. Terukur 16,1 dtk (2 ronde), jadi batas tunggu 25 dtk. */
function pingTunnel() {
  execFileSync('ip', ['netns', 'exec', 'wgcli', 'ping', '-c', '1', '-w', '25', '10.8.0.1'], { stdio: 'ignore' })
}

test.use({ storageState: STORAGE_STATE })
test.skip(!process.env.WGPANEL_E2E_LIVE, 'butuh tunnel hidup: jalankan e2e-handshake.sh --keep lalu set WGPANEL_E2E_LIVE=1')

test('badge peer berubah Online tanpa reload (polling 5 dtk)', async ({ page }) => {
  test.setTimeout(60_000) // termasuk ±16 dtk tunnel tersambung ulang
  // Mulai dari keadaan offline: nonaktifkan peer lewat API, buka halaman, lalu aktifkan lagi di luar halaman.
  const peers = await (await page.request.get('/api/v1/peers')).json() as { data: { id: number; name: string }[] }
  const peer = peers.data.find((row) => row.name === 'uji-e2e')
  if (!peer) throw new Error('peer uji-e2e tidak ada: jalankan scripts/e2e-handshake.sh --keep')
  const headers = { 'X-Requested-With': 'wgpanel' }
  expect((await page.request.post(`/api/v1/peers/${peer.id}/disable`, { headers })).status()).toBe(200)

  await page.goto('/peers')
  const row = page.getByRole('row').filter({ hasText: 'uji-e2e' })
  await expect(row).toContainText('Nonaktif (manual)')
  const navigations: string[] = []
  page.on('framenavigated', (frame) => { if (frame === page.mainFrame()) navigations.push(frame.url()) })

  // Diaktifkan dari LUAR halaman (API langsung) lalu client mengirim lalu lintas: hanya polling yang bisa membuat
  // badge berubah. Batas 10 dtk dihitung SETELAH tunnel tersambung lagi (= 2 interval polling, PLAN 7.2).
  expect((await page.request.post(`/api/v1/peers/${peer.id}/enable`, { headers })).status()).toBe(200)
  pingTunnel()
  await expect(row).toContainText('Online', { timeout: 10_000 })
  expect(navigations).toEqual([]) // tanpa reload
  await expect(row).not.toContainText('belum pernah')
})

test('dashboard menghitung peer online', async ({ page }) => {
  pingTunnel()
  await page.goto('/')
  await expect(page.getByText('Peer online').locator('..')).toContainText(/[1-9]/, { timeout: 10_000 })
  await expect(page.getByRole('row').filter({ hasText: 'wg0' })).toContainText('Berjalan')
})

test('tab tersembunyi tidak meminta /status', async ({ page }) => {
  await page.goto('/peers')
  await page.waitForResponse((response) => response.url().endsWith('/api/v1/status'))
  const requests: string[] = []
  page.on('request', (request) => { if (request.url().endsWith('/api/v1/status')) requests.push(request.url()) })

  // Simulasikan tab disembunyikan: TanStack Query membaca document.visibilityState lewat event visibilitychange.
  await page.evaluate(() => {
    Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'hidden' })
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => true })
    document.dispatchEvent(new Event('visibilitychange'))
  })
  await page.waitForTimeout(12_000) // > 2 interval polling
  expect(requests).toEqual([])

  // Tampil lagi → polling lanjut.
  await page.evaluate(() => {
    Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'visible' })
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => false })
    document.dispatchEvent(new Event('visibilitychange'))
  })
  await expect.poll(() => requests.length, { timeout: 8_000 }).toBeGreaterThan(0)
})
