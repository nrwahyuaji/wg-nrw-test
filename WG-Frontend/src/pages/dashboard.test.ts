// Setiap kode peringatan kesehatan di kontrak OpenAPI harus punya teks Indonesia di dashboard; kode baru di yaml
// tanpa teks akan tampil mentah (mis. "port_mismatch") dan uji ini menangkapnya.
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { HEALTH_WARNING } from './Dashboard'

const yaml = readFileSync(new URL('../../../docs/openapi.yaml', import.meta.url), 'utf8')
const codes = yaml.match(/enum: \[not_in_kernel[^\]]*\]/)?.[0].slice(7, -1).split(',').map((item) => item.trim()) ?? []

describe('peringatan kesehatan', () => {
  it('daftar kode terbaca dari openapi.yaml', () => {
    expect(codes).toContain('ip_forward_off')
  })
  it.each(codes)('%s punya teks', (code) => {
    expect(HEALTH_WARNING[code]).toBeTruthy()
  })
})
