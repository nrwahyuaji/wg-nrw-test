// E2E Fase 6 (PLAN 6.3): login salah tampil pesan, login benar masuk dashboard, buat interface wg9 muncul di tabel.
// Kredensial dari env WGPANEL_E2E_USER / WGPANEL_E2E_PASSWORD (tidak pernah ditulis di repo). Uji yang hanya butuh
// sesi memakai storageState dari global-setup, supaya total login per menit tetap di bawah batas panel.
import { expect, test } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'

const user = process.env.WGPANEL_E2E_USER ?? 'admin'
const password = process.env.WGPANEL_E2E_PASSWORD ?? ''

test.beforeAll(() => {
  if (!password) throw new Error('set WGPANEL_E2E_PASSWORD')
})

test('login salah menampilkan pesan dari server', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('Username').fill(user)
  await page.getByLabel('Password').fill('password-salah-sekali')
  await page.getByRole('button', { name: 'Masuk' }).click()
  await expect(page.getByRole('alert')).toHaveText('username atau password salah')
  await expect(page).toHaveURL(/\/login$/)
})

test('tanpa sesi diarahkan ke /login', async ({ page }) => {
  await page.goto('/interfaces')
  await expect(page).toHaveURL(/\/login$/)
})

test('login benar masuk dashboard', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('Username').fill(user)
  await page.getByLabel('Password').fill(password)
  await page.getByRole('button', { name: 'Masuk' }).click()
  await expect(page).toHaveURL(/\/$/)
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible()
})

test.describe('dengan sesi', () => {
  test.use({ storageState: STORAGE_STATE })

test('menu admin lengkap, buat interface uji tampil di tabel', async ({ page }) => {
  // Nama, subnet, dan port unik per run: DELETE /interfaces baru ada di Fase 15, jadi sisa run sebelumnya tidak boleh
  // membuat uji ini gagal (409). Interface uji berawalan "wge" dibersihkan scripts/e2e-cleanup.sh.
  const suffix = Date.now() % 10000
  const name = `wge${suffix}`
  const octet = 100 + (suffix % 150)
  const port = String(52000 + suffix)
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible()
  await expect(page.getByRole('navigation', { name: 'Menu utama' }).getByRole('link', { name: 'Pengaturan' })).toBeVisible()

  await page.getByRole('link', { name: 'Interface' }).click()
  await page.getByRole('button', { name: 'Tambah interface' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Nama').fill(name)
  await dialog.getByLabel('Alamat server (CIDR)').fill(`10.${octet}.0.1/24`)
  await dialog.getByLabel('Port UDP').fill(port)
  await dialog.getByLabel('Endpoint publik').fill('http://salah')
  await dialog.getByRole('button', { name: 'Simpan' }).click()
  await expect(dialog.getByText('endpoint wajib: nama host atau IP publik, tanpa skema dan tanpa port')).toBeVisible()

  await dialog.getByLabel('Endpoint publik').fill('10.251.1.1')
  await dialog.getByRole('button', { name: 'Simpan' }).click()
  await expect(dialog).toBeHidden()
  const row = page.getByRole('row').filter({ hasText: name })
  await expect(row).toContainText(`10.${octet}.0.1/24`)
  await expect(row).toContainText(port)
  await expect(row).toContainText('Aktif')
})

test('halaman interface tidak menggulir ke samping di layar HP 360px', async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 800 })
  await page.goto('/interfaces')
  await expect(page.getByRole('row').nth(1)).toBeVisible()
  // Tabel boleh menggulir di dalam kartunya; dokumen tidak.
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(360)
})
})
