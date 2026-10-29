// Uji format (PLAN 6.3): 1536 → "1,5 KiB", unix → jam WITA (UTC+8), waktu relatif.
import { describe, expect, it } from 'vitest'
import { chartLabel, formatBytes, formatWita, relativeTime } from './format'

describe('formatBytes', () => {
  it.each([
    [0, '0 B'],
    [-5, '0 B'],
    [Number.NaN, '0 B'],
    [512, '512 B'],
    [1536, '1,5 KiB'],
    [1048576, '1 MiB'],
    [5 * 1024 ** 3 + 300 * 1024 ** 2, '5,3 GiB'],
  ])('%s → %s', (input, want) => {
    expect(formatBytes(input)).toBe(want)
  })
})

describe('formatWita', () => {
  it('memakai zona Asia/Makassar (UTC+8)', () => {
    // 2026-10-09 02:30:00 UTC = 10.30 WITA
    expect(formatWita(Date.UTC(2026, 9, 9, 2, 30) / 1000)).toMatch(/09 Okt 2026.*10[.:]30 WITA$/)
  })
  it('null → tanda pisah', () => {
    expect(formatWita(null)).toBe('—')
  })
})

describe('relativeTime', () => {
  const now = 1_800_000_000
  it.each([
    [null, 'belum pernah'],
    [now - 10, 'baru saja'],
    [now - 50, '1 menit lalu'],
    [now - 120, '2 menit lalu'],
    [now - 3 * 3600, '3 jam lalu'],
    [now - 5 * 86400, '5 hari lalu'],
    [now + 4 * 86400, 'dalam 4 hari'],
  ])('%s → %s', (input, want) => {
    expect(relativeTime(input, now)).toBe(want)
  })
})

describe('chartLabel', () => {
  it('jam dan tanggal mengikuti WITA', () => {
    // 2026-10-08 16:00 UTC = 09 Okt 00.00 WITA (awal hari WITA)
    const midnight = Date.UTC(2026, 9, 8, 16) / 1000
    expect(chartLabel(midnight, '24h')).toMatch(/^00[.:]00$/)
    expect(chartLabel(midnight, '7d')).toBe('09 Okt')
    expect(chartLabel(midnight - 1, '30d')).toBe('08 Okt')
  })
})
