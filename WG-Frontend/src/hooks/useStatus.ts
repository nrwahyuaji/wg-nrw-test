// useStatus: status live semua peer (GET /status), dipolling 5 dtk HANYA saat tab terlihat (DESIGN 11:
// refetchIntervalInBackground false), supaya tab yang ditinggal tidak membebani panel.
import { useQuery } from '@tanstack/react-query'
import { apiFetch, type PeerStatus } from '../api/client'

/** STATUS_POLL_MS = status.poll-interval-seconds di Plan/tailwind.theme.json. */
export const STATUS_POLL_MS = 5000

/** Opsi polling bersama: daftar peer juga dipolling, karena enable/disable bisa berubah di luar halaman ini
 *  (admin lain, API, scheduler kedaluwarsa/kuota Fase 9). Tanpa ini badge "Nonaktif" basi sampai halaman dimuat ulang. */
export const livePolling = { refetchInterval: STATUS_POLL_MS, refetchIntervalInBackground: false } as const

export function useStatus() {
  return useQuery({
    queryKey: ['status'],
    queryFn: () => apiFetch<PeerStatus[]>('/status'),
    refetchInterval: STATUS_POLL_MS,
    refetchIntervalInBackground: false,
  })
}
