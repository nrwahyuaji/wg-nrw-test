// E2E Fase 7.1 (PLAN): tambah peer → IP .2 (wg0 kosong), QR tampil (naturalWidth 512), unduhan .conf berisi
// [Interface], disable → badge "Nonaktif (manual)", hapus dengan ketik nama. Butuh wg0 di panel dev tanpa peer lain
// bernama uji-pw. Sesi dari global-setup.
import { readFile } from 'node:fs/promises'
import { expect, test } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'

test.use({ storageState: STORAGE_STATE })

test('peer: tambah, QR, unduh, nonaktifkan, hapus', async ({ page }) => {
  // Prasyarat PLAN 7.1 "muncul .2": wg0 belum punya peer. Sisa run lain (mis. e2e-handshake.sh --keep) membuat IP
  // pertama bukan .2, dan itu bukan bug panel — gagal di sini dengan pesan jelas, bukan di asersi IP.
  const existing = await (await page.request.get('/api/v1/peers')).json() as { data: { name: string }[] }
  expect(existing.data.map((row) => row.name), 'wg0 harus kosong: jalankan scripts/e2e-handshake.sh --clean').toEqual([])
  const name = `uji-pw-${Date.now() % 100000}`
  await page.goto('/peers')
  await page.getByRole('button', { name: 'Tambah peer' }).click()
  const form = page.getByRole('dialog')
  await form.getByLabel('Interface').selectOption({ label: 'wg0 (10.8.0.1/24)' })
  await form.getByLabel('Nama').fill(name)
  await form.getByRole('button', { name: 'Simpan' }).click()
  await expect(form).toBeHidden()

  const row = page.getByRole('row').filter({ hasText: name })
  await expect(row).toContainText('10.8.0.2/32')
  await expect(row).toContainText('Offline') // aktif, belum pernah handshake
  await expect(row).toContainText('belum pernah')

  // QR + unduh
  await row.getByRole('button', { name: `QR dan config ${name}` }).click()
  const qrDialog = page.getByRole('dialog')
  const qr = qrDialog.getByRole('img', { name: `Kode QR konfigurasi ${name}` })
  await expect(qr).toBeVisible()
  await expect.poll(() => qr.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBe(512)
  await expect(qrDialog.getByRole('note')).toHaveCount(0) // keypair dibuat server → tanpa peringatan
  const download = page.waitForEvent('download')
  await qrDialog.getByRole('link', { name: 'Unduh .conf' }).click()
  const file = await download
  expect(file.suggestedFilename()).toBe(`${name}.conf`)
  const content = await readFile(await file.path(), 'utf8')
  expect(content).toContain('[Interface]')
  expect(content).toContain('Address = 10.8.0.2/32')
  expect(content).toContain('PresharedKey = ')
  await qrDialog.getByRole('button', { name: 'Tutup' }).click() // tombol tutup untuk layar sentuh
  await expect(qrDialog).toBeHidden()
  // Private key tidak pernah tampil sebagai teks di halaman.
  const privateKey = content.match(/^PrivateKey = (.+)$/m)?.[1] ?? ''
  expect(privateKey.length).toBeGreaterThan(40)
  expect(await page.content()).not.toContain(privateKey)

  // Detail peer
  await row.getByRole('link', { name }).click()
  await expect(page.getByRole('heading', { name })).toBeVisible()
  await expect(page.getByText('ya (terenkripsi)')).toBeVisible()
  expect(await page.content()).not.toContain(privateKey)
  await page.getByRole('link', { name: 'Kembali ke daftar peer' }).click()

  // Nonaktifkan → badge
  await row.getByRole('button', { name: `Nonaktifkan ${name}` }).click()
  await expect(row).toContainText('Nonaktif (manual)')
  await row.getByRole('button', { name: `Aktifkan ${name}` }).click()
  await expect(row).toContainText('Offline')

  // Hapus dengan ketik nama: tombol baru aktif setelah nama tepat.
  await row.getByRole('button', { name: `Hapus ${name}` }).click()
  const confirm = page.getByRole('dialog')
  const remove = confirm.getByRole('button', { name: 'Hapus peer' })
  await expect(remove).toBeDisabled()
  await confirm.getByLabel(`Ketik "${name}" untuk konfirmasi`).fill(name.slice(0, -1))
  await expect(remove).toBeDisabled()
  await confirm.getByLabel(`Ketik "${name}" untuk konfirmasi`).fill(name)
  await remove.click()
  await expect(confirm).toBeHidden()
  await expect(page.getByRole('row').filter({ hasText: name })).toHaveCount(0)
})

test('peer dari public key tempelan: peringatan + placeholder private key', async ({ page }) => {
  const name = `uji-pk-${Date.now() % 100000}`
  // Public key contoh (base64 32 byte) — bukan kunci sungguhan.
  const publicKey = 'jOuvUYXfC6ZE6l7n4EjGq6zN0yvzbAnS2D1VQi6qT0c='
  await page.goto('/peers')
  await page.getByRole('button', { name: 'Tambah peer' }).click()
  const form = page.getByRole('dialog')
  await form.getByLabel('Nama').fill(name)
  await form.getByLabel('Tempel public key dari perangkat').check()
  await form.getByLabel('Public key', { exact: true }).fill(publicKey)
  await form.getByLabel('Pakai preshared key').uncheck()
  await form.getByRole('button', { name: 'Simpan' }).click()
  await expect(form).toBeHidden()
  const row = page.getByRole('row').filter({ hasText: name })
  await row.getByRole('button', { name: `QR dan config ${name}` }).click()
  await expect(page.getByRole('dialog').getByRole('note')).toContainText('isi baris PrivateKey di perangkat')
  await page.keyboard.press('Escape')

  // Bersihkan lewat API (konfirmasi hapus sudah diuji di atas).
  const id = (await row.getByRole('link', { name }).getAttribute('href'))?.split('/').pop()
  const response = await page.request.delete(`/api/v1/peers/${id}`, { headers: { 'X-Requested-With': 'wgpanel' } })
  expect(response.status()).toBe(204)
})

test('peer: galat validasi tampil per field', async ({ page }) => {
  await page.goto('/peers')
  await page.getByRole('button', { name: 'Tambah peer' }).click()
  const form = page.getByRole('dialog')
  await form.getByLabel('Nama').fill('uji-galat')
  await form.getByLabel('IP di tunnel').fill('10.99.0.5')
  await form.getByRole('button', { name: 'Simpan' }).click()
  await expect(form.getByText('IP di luar subnet interface')).toBeVisible()
  await expect(form).toBeVisible()
})
