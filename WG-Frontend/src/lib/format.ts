// Format tampilan (DESIGN 11): waktu selalu WITA (Asia/Makassar), byte memakai satuan biner (KiB/MiB/GiB), dan
// waktu relatif berbahasa Indonesia. Fungsi murni supaya mudah diuji tanpa DOM.

const BYTE_UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB']

const decimal = new Intl.NumberFormat('id-ID', { maximumFractionDigits: 1 })

const witaFormatter = new Intl.DateTimeFormat('id-ID', {
  timeZone: 'Asia/Makassar',
  day: '2-digit',
  month: 'short',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hour12: false,
})

/** formatBytes: 1536 → "1,5 KiB" (satuan biner, satu desimal, koma desimal Indonesia). Negatif/NaN → "0 B". */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < BYTE_UNITS.length - 1) {
    value /= 1024
    unit++
  }
  return `${unit === 0 ? value : decimal.format(value)} ${BYTE_UNITS[unit]}`
}

/** formatGiB: GiB → string GB (1 GiB = 1024³ byte), satu desimal, koma desimal Indonesia. */
export function formatGiB(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const gib = bytes / 1024 / 1024 / 1024
  return decimal.format(gib) + ' GB'
}

/** formatWita: unix detik → "09 Okt 2026 10.30 WITA". null/undefined → "—". */
export function formatWita(unix: number | null | undefined): string {
  if (unix === null || unix === undefined) return '—'
  return `${witaFormatter.format(new Date(unix * 1000))} WITA`
}

/**
 * relativeTime: selisih unix detik terhadap `now` dalam kalimat pendek: "baru saja", "3 menit lalu",
 * "2 jam lalu", "5 hari lalu"; waktu di masa depan → "dalam 4 hari". null → "belum pernah".
 */
export function relativeTime(unix: number | null | undefined, now: number = Date.now() / 1000): string {
  if (unix === null || unix === undefined) return 'belum pernah'
  const diff = Math.round(now - unix)
  const future = diff < 0
  const seconds = Math.abs(diff)
  if (seconds < 45) return future ? 'sebentar lagi' : 'baru saja'
  // [batas atas detik, pembagi, satuan]: pilih satuan terbesar yang masih di bawah batasnya.
  const units: [number, number, string][] = [
    [3600, 60, 'menit'],
    [86400, 3600, 'jam'],
    [Infinity, 86400, 'hari'],
  ]
  const [, divisor, label] = units.find(([limit]) => seconds < limit) ?? units[units.length - 1]
  const amount = Math.max(1, Math.floor(seconds / divisor))
  return future ? `dalam ${amount} ${label}` : `${amount} ${label} lalu`
}

const witaHour = new Intl.DateTimeFormat('id-ID', { timeZone: 'Asia/Makassar', hour: '2-digit', minute: '2-digit', hour12: false })
const witaDay = new Intl.DateTimeFormat('id-ID', { timeZone: 'Asia/Makassar', day: '2-digit', month: 'short' })

/** chartLabel: label sumbu grafik trafik. range 24h → jam WITA "10.00", 7d/30d → tanggal WITA "09 Okt". */
export function chartLabel(unix: number, range: '24h' | '7d' | '30d'): string {
  return (range === '24h' ? witaHour : witaDay).format(new Date(unix * 1000))
}
