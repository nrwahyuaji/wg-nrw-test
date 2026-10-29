// E2E Fase 8.2 (PLAN): grafik trafik ter-render (svg path ada) di dashboard dan detail peer, dan tombol rentang
// mengganti data. Butuh wg0 di panel dev. Peer uji dibuat lewat API lalu dihapus. Sesi dari global-setup.
import { expect, test } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'

test.use({ storageState: STORAGE_STATE })

const headers = { 'X-Requested-With': 'wgpanel' }

test('grafik trafik ter-render di dashboard dan detail peer', async ({ page }) => {
  const interfaces = await (await page.request.get('/api/v1/interfaces')).json() as { data: { id: number; name: string }[] }
  const wg0 = interfaces.data.find((row) => row.name === 'wg0')
  if (!wg0) throw new Error('wg0 tidak ada di panel dev')

  await page.goto('/')
  const dashboardChart = page.getByTestId('traffic-chart').first()
  await expect(dashboardChart.locator('svg path.recharts-area-curve').first()).toBeAttached()
  await expect(page.getByText('Trafik hari ini', { exact: true })).toBeVisible()

  const created = await page.request.post('/api/v1/peers', { headers, data: { interface_id: wg0.id, name: `uji-grafik-${Date.now() % 100000}` } })
  expect(created.status()).toBe(201)
  const peer = (await created.json() as { data: { id: number } }).data
  try {
    await page.goto(`/peers/${peer.id}`)
    const chart = page.getByTestId('traffic-chart')
    await expect(chart.locator('svg path.recharts-area-curve').first()).toBeAttached()
    const sevenDays = page.waitForResponse((response) => response.url().endsWith(`/peers/${peer.id}/traffic?range=7d`))
    await page.getByRole('button', { name: '7 hari' }).click()
    expect((await sevenDays).status()).toBe(200)
    await expect(page.getByRole('button', { name: '7 hari' })).toHaveAttribute('aria-pressed', 'true')
    await expect(chart.locator('svg path.recharts-area-curve').first()).toBeAttached()
  } finally {
    expect((await page.request.delete(`/api/v1/peers/${peer.id}`, { headers })).status()).toBe(204)
  }
})

test('dashboard dengan grafik tidak menggulir ke samping di HP 360px', async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 800 })
  await page.goto('/')
  await expect(page.getByTestId('traffic-chart').first().locator('svg')).toBeAttached()
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(360)
})
