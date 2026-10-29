// Halaman Peer (F-3, F-4, F-5.1): tabel peer + status live, tambah/ubah, QR, enable/disable, hapus dengan
// konfirmasi ketik nama (DESIGN 11). Operator hanya melihat (tanpa aksi).
import { useQuery } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { ApiError, apiFetch, type Interface, type Peer } from '../api/client'
import { PeerForm } from '../components/PeerForm'
import { PeerTable, type PeerAction } from '../components/PeerTable'
import { QrDialog } from '../components/QrDialog'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { ConfirmDialog } from '../components/ui/confirm-dialog'
import { useMe } from '../hooks/useMe'
import { usePeerActions } from '../hooks/usePeerActions'
import { livePolling, useStatus } from '../hooks/useStatus'
import { indexStatus } from '../lib/peer-status'

export default function Peers() {
  const me = useMe()
  const isAdmin = me.data?.role === 'admin'
  const peers = useQuery({ queryKey: ['peers'], queryFn: () => apiFetch<Peer[]>('/peers'), ...livePolling })
  const interfaces = useQuery({ queryKey: ['interfaces'], queryFn: () => apiFetch<Interface[]>('/interfaces') })
  const status = useStatus()
  const actions = usePeerActions()
  const [dialog, setDialog] = useState<{ action: PeerAction | 'new'; peer: Peer | null } | null>(null)

  // onAction: QR/ubah/hapus membuka dialog; enable/disable langsung (bisa dibalik dengan satu klik lagi).
  const onAction = async (action: PeerAction, peer: Peer) => {
    if (action !== 'toggle') { setDialog({ action, peer }); return }
    try {
      await actions.toggle(peer)
      toast.success(`${peer.name} ${peer.enabled ? 'dinonaktifkan' : 'diaktifkan'}`)
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : 'gagal')
    }
  }

  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold">Peer</h1>
        {isAdmin ? (
          <Button onClick={() => setDialog({ action: 'new', peer: null })} disabled={!interfaces.data?.length}>
            <Plus size={18} aria-hidden /> Tambah peer
          </Button>
        ) : null}
      </div>
      {interfaces.data?.length === 0 ? <p className="text-muted-foreground">Buat interface dulu sebelum menambah peer.</p> : null}
      <Card>
        {peers.isPending || interfaces.isPending ? <p className="text-muted-foreground">Memuat…</p>
          : peers.isError ? <p className="text-destructive">{peers.error.message}</p>
          : <PeerTable peers={peers.data} interfaces={interfaces.data ?? []} status={indexStatus(status.data)}
              isAdmin={isAdmin} onAction={onAction} />}
      </Card>
      {dialog?.action === 'new' || dialog?.action === 'edit' ? (
        <PeerForm current={dialog.peer} interfaces={interfaces.data ?? []} onClose={() => setDialog(null)} />
      ) : null}
      {dialog?.action === 'qr' && dialog.peer ? <QrDialog peer={dialog.peer} onClose={() => setDialog(null)} /> : null}
      {dialog?.action === 'delete' && dialog.peer ? (
        <ConfirmDialog open onOpenChange={(open) => { if (!open) setDialog(null) }} title={`Hapus ${dialog.peer.name}?`}
          description="Peer langsung terputus dan config lamanya tidak bisa dipakai lagi. Tidak bisa dibatalkan."
          confirmLabel="Hapus peer" confirmText={dialog.peer.name}
          onConfirm={async () => { await actions.remove(dialog.peer as Peer); toast.success('peer dihapus') }} />
      ) : null}
    </div>
  )
}
