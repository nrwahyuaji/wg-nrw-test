// Uji logika murni halaman: penyaringan menu per role dan konversi form interface → body API.
import { describe, expect, it } from 'vitest'
import { menuFor } from '../components/Layout'
import { toRequest } from '../pages/Interfaces'

describe('menuFor', () => {
  it('admin melihat semua menu', () => {
    expect(menuFor('admin').map((item) => item.label)).toEqual(['Dashboard', 'Interface', 'Peer', 'User', 'Audit', 'Pengaturan', 'Akun'])
  })
  it('operator tanpa User/Audit/Pengaturan', () => {
    expect(menuFor('operator').map((item) => item.label)).toEqual(['Dashboard', 'Interface', 'Peer', 'Akun'])
  })
  it('user dan role tak dikenal hanya Akun', () => {
    expect(menuFor('user').map((item) => item.label)).toEqual(['Akun'])
    expect(menuFor(undefined).map((item) => item.label)).toEqual([])
  })
})

describe('toRequest', () => {
  it('memecah daftar dipisah koma dan mengubah angka', () => {
    expect(toRequest({ name: ' wg9 ', address: '10.9.0.1/24', listen_port: '51829', mtu: '', endpoint_host: ' vpn ',
      client_dns: '1.1.1.1, 9.9.9.9,', client_allowed_ips: '', client_keepalive: '25' })).toEqual({
      name: 'wg9', address: '10.9.0.1/24', listen_port: 51829, mtu: 0, endpoint_host: 'vpn',
      client_dns: ['1.1.1.1', '9.9.9.9'], client_allowed_ips: [], client_keepalive: 25 })
  })
  it('angka rusak → 0 supaya validasi server menolak', () => {
    expect(toRequest({ name: 'x', address: '', listen_port: 'abc', mtu: '0', endpoint_host: '', client_dns: '',
      client_allowed_ips: '', client_keepalive: '' }).listen_port).toBe(0)
  })
})
