// Login SEKALI untuk semua uji yang butuh sesi, lalu simpan cookie ke e2e/.auth/admin.json (diabaikan git).
// Alasan: panel membatasi login 5/menit per IP (DESIGN 7.3); login di setiap uji membuat gerbang gagal acak (429).
import { request, type FullConfig } from '@playwright/test'

export const STORAGE_STATE = 'e2e/.auth/admin.json'

export default async function globalSetup(config: FullConfig) {
  const baseURL = config.projects[0].use.baseURL
  const password = process.env.WGPANEL_E2E_PASSWORD
  if (!password) throw new Error('set WGPANEL_E2E_PASSWORD')
  const context = await request.newContext({ baseURL })
  const response = await context.post('/api/v1/auth/login', {
    headers: { 'X-Requested-With': 'wgpanel' },
    data: { username: process.env.WGPANEL_E2E_USER ?? 'admin', password },
  })
  if (response.status() !== 204) throw new Error(`login globalSetup: HTTP ${response.status()}`)
  await context.storageState({ path: STORAGE_STATE })
  await context.dispose()
}
