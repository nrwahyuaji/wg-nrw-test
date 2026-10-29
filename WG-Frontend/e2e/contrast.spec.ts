// Gerbang Fase 6 (PLAN): kontras AA. Untuk tema terang dan gelap, render halaman login + interface, lalu hitung rasio
// kontras setiap elemen teks yang terlihat terhadap latar efektifnya (WCAG 2.x). Gagal bila ada yang < 4.5.
import { expect, test, type Page } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'

/** worstContrast menghitung kontras semua elemen teks terlihat; mengembalikan yang di bawah batas. */
async function lowContrast(page: Page): Promise<{ text: string; ratio: number }[]> {
  return page.evaluate(() => {
    const parse = (value: string): [number, number, number, number] | null => {
      const match = value.match(/rgba?\(([^)]+)\)/)
      if (match) {
        const parts = match[1].split(/[ ,/]+/).filter(Boolean).map(Number)
        return [parts[0], parts[1], parts[2], parts[3] ?? 1]
      }
      const probe = document.createElement('canvas').getContext('2d')
      if (!probe) return null
      probe.fillStyle = value
      probe.fillRect(0, 0, 1, 1)
      const [r, g, b, a] = probe.getImageData(0, 0, 1, 1).data
      return [r, g, b, a / 255]
    }
    const luminance = ([r, g, b]: number[]) => {
      const channel = (c: number) => { const s = c / 255; return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4 }
      return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b)
    }
    const background = (element: Element | null): number[] => {
      for (let node = element; node; node = node.parentElement) {
        const color = parse(getComputedStyle(node).backgroundColor)
        if (color && color[3] > 0.5) return color.slice(0, 3)
      }
      return parse(getComputedStyle(document.body).backgroundColor)?.slice(0, 3) ?? [255, 255, 255]
    }
    const results: { text: string; ratio: number }[] = []
    for (const element of document.querySelectorAll('body *')) {
      const own = [...element.childNodes].some((node) => node.nodeType === Node.TEXT_NODE && node.textContent?.trim())
      if (!own) continue
      const style = getComputedStyle(element)
      const box = element.getBoundingClientRect()
      if (style.visibility === 'hidden' || style.display === 'none' || box.width === 0 || element.closest('.sr-only')) continue
      if ((element as HTMLElement).closest('[disabled]')) continue // WCAG 1.4.3 mengecualikan kontrol nonaktif
      const fg = parse(style.color)
      if (!fg) continue
      const [l1, l2] = [luminance(fg), luminance(background(element))].sort((a, b) => b - a)
      const ratio = (l1 + 0.05) / (l2 + 0.05)
      if (ratio < 4.5) results.push({ text: (element.textContent ?? '').trim().slice(0, 40), ratio: Math.round(ratio * 100) / 100 })
    }
    return results
  })
}

for (const theme of ['light', 'dark'] as const) {
  test(`kontras AA tema ${theme}`, async ({ browser, page }) => {
    await page.addInitScript((value) => localStorage.setItem('wgpanel-theme', value), theme)
    await page.goto('/login')
    await page.getByLabel('Password').fill('x') // munculkan teks isian
    expect(await lowContrast(page), 'halaman login').toEqual([])

    // Halaman di balik login memakai sesi dari global-setup (tanpa login ulang, lihat batas laju).
    const authed = await browser.newContext({ storageState: STORAGE_STATE })
    await authed.addInitScript((value) => localStorage.setItem('wgpanel-theme', value), theme)
    page = await authed.newPage()
    await page.goto('/interfaces')
    await expect(page.getByRole('heading', { name: 'Interface' })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.classList.contains('dark'))).toBe(theme === 'dark')
    expect(await lowContrast(page), 'halaman interface').toEqual([])

    await page.getByRole('button', { name: 'Tambah interface' }).click()
    await page.getByRole('dialog').getByRole('button', { name: 'Simpan' }).click() // munculkan galat per field
    await expect(page.getByRole('dialog').getByText('nama harus huruf kecil')).toBeVisible()
    expect(await lowContrast(page), 'dialog + galat').toEqual([])
    await page.keyboard.press('Escape')

    // Fase 7: dashboard, daftar peer (badge status + tombol aksi), dan form peer.
    await page.goto('/')
    await expect(page.getByText('Peer online')).toBeVisible()
    expect(await lowContrast(page), 'dashboard').toEqual([])
    await page.goto('/peers')
    await expect(page.getByRole('heading', { name: 'Peer' })).toBeVisible()
    expect(await lowContrast(page), 'halaman peer').toEqual([])
    await page.getByRole('button', { name: 'Tambah peer' }).click()
    await page.getByRole('dialog').getByLabel('IP di tunnel').fill('salah')
    await page.getByRole('dialog').getByRole('button', { name: 'Simpan' }).click()
    await expect(page.getByRole('dialog').getByText('alamat harus IPv4')).toBeVisible()
    expect(await lowContrast(page), 'form peer + galat').toEqual([])
    await authed.close()
  })
}
