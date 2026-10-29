// usePeerActions: aksi tulis peer bersama untuk halaman daftar dan detail (enable/disable, hapus), lalu
// memuat ulang daftar + status supaya badge langsung berubah tanpa menunggu polling.
import { useQueryClient } from '@tanstack/react-query'
import { apiFetch, type Peer } from '../api/client'

export function usePeerActions() {
  const queryClient = useQueryClient()
  const refresh = () => Promise.all([
    queryClient.invalidateQueries({ queryKey: ['peers'] }),
    queryClient.invalidateQueries({ queryKey: ['status'] }),
  ])
  return {
    toggle: async (peer: Peer) => {
      await apiFetch<Peer>(`/peers/${peer.id}/${peer.enabled ? 'disable' : 'enable'}`, { method: 'POST' })
      await refresh()
    },
    remove: async (peer: Peer) => {
      await apiFetch(`/peers/${peer.id}`, { method: 'DELETE' })
      await refresh()
    },
  }
}
