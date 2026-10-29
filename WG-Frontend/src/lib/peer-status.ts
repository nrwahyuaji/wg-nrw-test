// Status tampilan satu peer (F-3.6, F-5.1): gabungan kolom enabled/disabled_reason (DB) dan status live (/status).
// Murni supaya mudah diuji; komponen StatusBadge hanya memetakan hasilnya ke warna.
import type { Peer, PeerStatus } from '../api/client'

export type PeerDisplayStatus = { tone: 'online' | 'offline' | 'disabled'; label: string }

/** Label alasan nonaktif (PRD F-3.6). Alasan tak dikenal ditampilkan apa adanya. */
const REASON: Record<string, string> = { manual: 'manual', expired: 'kedaluwarsa', quota: 'kuota habis' }

/**
 * peerDisplayStatus: nonaktif selalu menang (peer tidak ada di kernel); selain itu online/offline dari /status.
 * Status live belum termuat → offline (bukan galat), karena polling pertama bisa belum selesai.
 */
export function peerDisplayStatus(peer: Pick<Peer, 'enabled' | 'disabled_reason'>, live?: PeerStatus): PeerDisplayStatus {
  if (!peer.enabled) {
    const reason = peer.disabled_reason ? REASON[peer.disabled_reason] ?? peer.disabled_reason : ''
    return { tone: 'disabled', label: reason ? `Nonaktif (${reason})` : 'Nonaktif' }
  }
  return live?.online ? { tone: 'online', label: 'Online' } : { tone: 'offline', label: 'Offline' }
}

/** indexStatus memetakan peer_id → status live agar penggabungan ke tabel O(1) per baris. */
export function indexStatus(rows: PeerStatus[] | undefined): Map<number, PeerStatus> {
  return new Map((rows ?? []).map((row) => [row.peer_id, row]))
}

/** splitList memecah isian "a, b ,c" menjadi daftar tanpa item kosong. */
export function splitList(raw: string): string[] {
  return raw.split(',').map((item) => item.trim()).filter(Boolean)
}
