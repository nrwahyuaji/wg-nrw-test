// Uji status tampilan peer: nonaktif + alasan, online/offline dari status live, penggabungan per peer_id.
import { describe, expect, it } from 'vitest'
import type { PeerStatus } from '../api/client'
import { indexStatus, peerDisplayStatus, splitList } from './peer-status'

const live = (peer_id: number, online: boolean): PeerStatus =>
  ({ peer_id, interface_id: 1, online, last_handshake: online ? 1 : null, endpoint: '', rx: 0, tx: 0 })

describe('peerDisplayStatus', () => {
  it('nonaktif menang atas status live', () => {
    expect(peerDisplayStatus({ enabled: false, disabled_reason: 'manual' }, live(1, true))).toEqual({ tone: 'disabled', label: 'Nonaktif (manual)' })
    expect(peerDisplayStatus({ enabled: false, disabled_reason: 'quota' })).toEqual({ tone: 'disabled', label: 'Nonaktif (kuota habis)' })
    expect(peerDisplayStatus({ enabled: false, disabled_reason: 'expired' }).label).toBe('Nonaktif (kedaluwarsa)')
    expect(peerDisplayStatus({ enabled: false, disabled_reason: '' }).label).toBe('Nonaktif')
  })
  it('aktif: online/offline dari /status, belum termuat = offline', () => {
    expect(peerDisplayStatus({ enabled: true, disabled_reason: '' }, live(1, true))).toEqual({ tone: 'online', label: 'Online' })
    expect(peerDisplayStatus({ enabled: true, disabled_reason: '' }, live(1, false)).label).toBe('Offline')
    expect(peerDisplayStatus({ enabled: true, disabled_reason: '' }).label).toBe('Offline')
  })
})

describe('indexStatus / splitList', () => {
  it('memetakan per peer_id', () => {
    const map = indexStatus([live(3, true), live(7, false)])
    expect(map.get(3)?.online).toBe(true)
    expect(map.get(9)).toBeUndefined()
    expect(indexStatus(undefined).size).toBe(0)
  })
  it('memecah daftar dipisah koma', () => {
    expect(splitList(' 10.8.0.0/24, ,192.168.1.0/24 ')).toEqual(['10.8.0.0/24', '192.168.1.0/24'])
  })
})
